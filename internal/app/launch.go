package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/wlame/vibrator/internal/catalog"
	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/localprovider"
	"github.com/wlame/vibrator/internal/prereq"
	"github.com/wlame/vibrator/internal/workspace"
)

// buildImage generates the Dockerfile fresh and shells out to
// `docker build`. The Dockerfile is piped via stdin (-f -) so we never
// touch disk — same path as `vibrate build`.
func buildImage(ctx context.Context, dc docker.Client,
	dfSpec dockerfile.Spec, imageTag string, opts Options,
) error {
	out, err := dockerfile.Generate(dfSpec)
	if err != nil {
		return fmt.Errorf("generate dockerfile: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.Stderr, "→ Building image %s (no-cache=%v) ...\n", imageTag, opts.Rebuild)

	return dc.Build(ctx, docker.BuildSpec{
		DockerfileBytes: out,
		ContextDir:      cwd,
		Tag:             imageTag,
		NoCache:         opts.Rebuild,
		BuildArgs: map[string]string{
			"USERNAME": dfSpec.Username,
			"HOST_UID": fmt.Sprintf("%d", dfSpec.HostUID),
			"HOST_GID": fmt.Sprintf("%d", dfSpec.HostGID),
		},
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
	})
}

// runContainer translates a workspace + pin into a `docker run`
// invocation, mounts the workspace at the same absolute path, forwards
// auth + LLM env vars, and execs.
//
// `docker run` is INTERACTIVE here (-it) because the user is dropping
// into a shell session. When they exit, docker returns and we return
// normally.
func runContainer(ctx context.Context, dc docker.Client,
	imageTag, containerName, wsDir string,
	wsSpec workspace.Spec, pin config.Pin, opts Options,
) error {
	wsHash := workspace.Fingerprint(wsSpec)

	envVars, err := buildContainerEnv(pin)
	if err != nil {
		return err
	}

	labels := map[string]string{
		"vibrator.managed":   "true",
		"vibrator.harness":   pin.Harness,
		"vibrator.workspace": wsHash,
		"vibrator.path":      wsDir,
	}

	fmt.Fprintf(opts.Stderr, "→ Creating container %s ...\n", containerName)

	return dc.Run(ctx, docker.RunSpec{
		Image:         imageTag,
		ContainerName: containerName,
		Interactive:   true,
		Volumes: []docker.Volume{
			{Host: wsDir, Container: wsDir},
		},
		Env:    envVars,
		Labels: labels,
		// host network keeps host.docker.internal cheap and lets
		// in-container tools reach host services without --add-host.
		// We use bridge instead of host to keep Linux/macOS behavior
		// uniform; --add-host below patches in host.docker.internal.
		AddHosts: []string{"host.docker.internal:host-gateway"},
		Stdin:    opts.Stdin,
		Stdout:   opts.Stdout,
		Stderr:   opts.Stderr,
	})
}

// execIntoContainer runs an interactive shell inside an already-running
// (or just-started) container.
func execIntoContainer(ctx context.Context, dc docker.Client,
	containerName string, pin config.Pin, opts Options,
) error {
	shell := pin.Shell
	if shell == "" {
		shell = "zsh"
	}
	return dc.Exec(ctx, docker.ExecSpec{
		Container:   containerName,
		Interactive: true,
		Cmd:         []string{"/bin/" + shell},
		Stdin:       opts.Stdin,
		Stdout:      opts.Stdout,
		Stderr:      opts.Stderr,
	})
}

// buildContainerEnv produces the full set of env vars forwarded into
// the container at `docker run` time. Order of precedence:
//
//  1. Harness AuthEnvVars (host env values passed through)
//  2. Harness LLMEnvVars (computed from pin.LLM)
//  3. pin.Env overrides (literal or $NAME indirection from host)
//
// Later entries with the same name win.
func buildContainerEnv(pin config.Pin) ([]docker.EnvVar, error) {
	h, ok := harness.ByID(pin.Harness)
	if !ok {
		return nil, fmt.Errorf("unknown harness %q", pin.Harness)
	}

	// Materialize into an ordered map so we can deduplicate by name
	// while preserving the precedence rule (later wins).
	final := map[string]string{}

	// 1. Auth env vars — forward host values verbatim.
	for _, name := range h.AuthEnvVars() {
		if v := os.Getenv(name); v != "" {
			final[name] = v
		}
	}

	// 2. LLM-derived env vars from pin.LLM.
	if pin.LLM != nil {
		apiKey, err := resolveAPIKey(pin.LLM)
		if err != nil {
			return nil, fmt.Errorf("resolve LLM api key: %w", err)
		}
		for k, v := range h.LLMEnvVars(pin.LLM.Provider, pin.LLM.Model, pin.LLM.BaseURL, apiKey) {
			final[k] = v
		}
	}

	// 3. pin.Env overrides. Values of the form "$NAME" are resolved
	//    against the host's environment; literal values pass through.
	for k, v := range pin.Env {
		if strings.HasPrefix(v, "$") {
			final[k] = os.Getenv(strings.TrimPrefix(v, "$"))
		} else {
			final[k] = v
		}
	}

	// Convert to sorted []docker.EnvVar for stable output (matters in
	// tests and when debugging exact `docker run` arg lines).
	names := make([]string, 0, len(final))
	for n := range final {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]docker.EnvVar, 0, len(final))
	for _, n := range names {
		out = append(out, docker.EnvVar{Name: n, Value: final[n]})
	}
	return out, nil
}

// resolveAPIKey extracts the credential the LLM provider expects.
// Precedence:
//
//  1. pin.LLM.Auth.Value — pasted-into-wizard literal.
//  2. $pin.LLM.Auth.Env — host environment variable name.
//  3. "" — only valid for local providers (ollama, lmstudio).
//
// Returns ("", nil) for local providers. Returns an error when a cloud
// provider has neither path populated.
func resolveAPIKey(spec *config.LLMSpec) (string, error) {
	switch spec.Provider {
	case "ollama", "lmstudio":
		return "", nil
	}
	if spec.Auth == nil {
		return "", fmt.Errorf("provider %q requires credentials but [llm.auth] is missing", spec.Provider)
	}
	if spec.Auth.Value != "" {
		return spec.Auth.Value, nil
	}
	if spec.Auth.Env != "" {
		v := os.Getenv(spec.Auth.Env)
		if v == "" {
			return "", fmt.Errorf("env var $%s is unset on the host", spec.Auth.Env)
		}
		return v, nil
	}
	return "", fmt.Errorf("provider %q has no credential configured", spec.Provider)
}

// runLaunchPrereqs probes every prereq referenced by the pin's catalog
// entries. Failure here is fatal — entering a container with broken
// host wiring just wastes the user's time. The error message
// references the catalog's setup-doc anchor so the user knows where
// to look.
//
// This is the wizard's "soft warn" promoted to "hard fail" for launch.
func runLaunchPrereqs(ctx context.Context, pin config.Pin, stderr io.Writer) error {
	if len(pin.Catalog) == 0 {
		return nil
	}
	entries, err := catalog.LoadAll(vibrator.CatalogFS)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	// Walk pin.Catalog and collect distinct prereq IDs referenced.
	prereqIDs := map[string]bool{}
	for _, id := range pin.Catalog {
		key := pin.Harness + "/" + id
		entry, ok := entries[key]
		if !ok || entry.Prereq == "" {
			continue
		}
		prereqIDs[entry.Prereq] = true
	}
	if len(prereqIDs) == 0 {
		return nil
	}

	// For each unique prereq id, probe.
	for id := range prereqIDs {
		// claude-mem is the only built-in prereq for now. New ones can
		// drop into this switch as they're added.
		var p *prereq.Prereq
		switch id {
		case prereq.ClaudeMemPrereqID:
			cfg, err := prereq.LoadClaudeMemAdminConfig()
			if err != nil {
				return fmt.Errorf("claude-mem admin config not found (%s) — see catalog/claude-code/claude-mem.md#host-setup", prereq.ClaudeMemAdminConfigPath())
			}
			p = prereq.ClaudeMemPrereq(cfg, nil)
		default:
			fmt.Fprintf(stderr, "  (skipping unknown prereq %q — no probe registered)\n", id)
			continue
		}

		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		r := p.Verifier.Verify(probeCtx)
		cancel()

		if !r.OK {
			return fmt.Errorf(
				"prereq %q FAILED at launch: %s\nhint: %s\nsee: %s",
				id, r.Message, r.Hint, p.SetupDoc)
		}
		fmt.Fprintf(stderr, "  ✓ prereq %s: %s\n", id, r.Message)
	}
	return nil
}

// ensureLLMProviderRunning launches the host-side local provider if the
// pin specifies one (Ollama / LM Studio). For cloud providers this is
// a no-op.
//
// The function returns an error if the local provider can't be
// reached AND can't be auto-started — abort the launch rather than
// running a container that will immediately fail.
func ensureLLMProviderRunning(ctx context.Context, pin config.Pin, stderr io.Writer) error {
	if pin.LLM == nil {
		return nil
	}
	p, ok := localprovider.ByID(pin.LLM.Provider)
	if !ok {
		// Not a local provider — nothing to start.
		return nil
	}
	url := pin.LLM.BaseURL
	if url == "" {
		url = p.DefaultURL()
	}
	fmt.Fprintf(stderr, "→ Ensuring %s is running at %s ...\n", p.Name(), url)

	startCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := p.EnsureRunning(startCtx, url, pin.LLM.Model); err != nil {
		return fmt.Errorf("local provider %s not reachable: %w", p.Name(), err)
	}
	fmt.Fprintf(stderr, "  ✓ %s reachable\n", p.Name())
	return nil
}
