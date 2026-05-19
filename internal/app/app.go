// Package app is the orchestrator that powers the top-level `vibrate`
// command. It glues all the pieces together — flag resolution, .vb
// loading, the wizard, prereq probes, local LLM provider startup,
// build, run, and exec.
//
// # Decision tree
//
//   .vb exists? → resolve spec → image exists? → container exists?
//                                                  ├─ running → docker exec
//                                                  ├─ stopped → docker start + exec
//                                                  ├─ image only → docker run
//                                                  └─ none → build → run
//   .vb missing → wizard → save .vb → build → run
//
// Each step is a small function in this package; Run wires them in
// order. The cobra subcommand layer (internal/cli) imports app and
// calls Run with parsed flags.
//
// # Layering
//
// app is the topmost internal package. It imports everything below
// (config, docker, dockerfile, feature, harness, hostprobe, localprovider,
// prereq, profile, runtime, wizard, workspace). Nothing internal imports
// app.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/hostprobe"
	"github.com/wlame/vibrator/internal/profile"
	"github.com/wlame/vibrator/internal/wizard"
	"github.com/wlame/vibrator/internal/workspace"
)

// Options bundles the flag state passed from the CLI layer. Mirrors
// `vibrate build`'s flags plus a few orchestrator-only knobs.
type Options struct {
	// CLI flag overrides — non-empty wins over .vb values.
	Harness    string
	Profile    string
	Shell      string
	With       []string
	No         []string
	CatalogIDs []string
	Username   string
	HostUID    int
	HostGID    int

	// NoWizard, when true, skips the interactive wizard entirely. Falls
	// back to flags/defaults — useful for scripted invocations and CI.
	NoWizard bool

	// NoSave, when true, doesn't write the wizard's result to .vb.
	// Useful for "try this combo once" invocations.
	NoSave bool

	// Rebuild forces a fresh `docker build` even when a matching image
	// exists. Same as `--no-cache` at the docker level.
	Rebuild bool

	// VibratorVersion is the version string baked into the generated
	// Dockerfile header. Passed through from the CLI layer.
	VibratorVersion string

	// Output streams. nil = use OS defaults.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
}

// Run executes the full vibrate decision tree. Returns nil on
// successful container entry (the exec/run call inherits stdio so the
// user "lands" inside the container; when they exit, control returns
// here with whatever exit code they produced).
//
// Most errors are wrapped with `fmt.Errorf(... "%w", err)` so the CLI
// layer's error printer surfaces useful diagnostics.
func Run(ctx context.Context, opts Options) error {
	// Default streams.
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Stdin == nil {
		opts.Stdin = os.Stdin
	}

	// 1. Resolve workspace + existing pin (if any).
	wsDir, pin, pinPath, err := loadWorkspaceAndPin(opts)
	if err != nil {
		return err
	}

	// 2. Apply CLI flag overrides on top of pin values.
	applyFlagOverrides(&pin, opts)

	// 3. Run the wizard for anything still unset.
	saveAfterWizard := false
	if needsWizard(pin) && !opts.NoWizard {
		fmt.Fprintf(opts.Stderr, "→ Setup wizard\n")
		result, err := runWizard(ctx, pin, wsDir)
		if err != nil {
			return err
		}
		if result.Cancelled {
			fmt.Fprintln(opts.Stderr, "Wizard cancelled — aborting.")
			return nil
		}
		pin = result.Pin
		saveAfterWizard = !opts.NoSave

		// Show the user what they picked + the equivalent CLI command,
		// so they can copy it into a script next time. We print to
		// stderr (not stdout) so this doesn't pollute scripted callers
		// piping vibrate's stdout somewhere.
		fmt.Fprintln(opts.Stderr)
		fmt.Fprintln(opts.Stderr, wizard.Summary(pin, wsDir))
		fmt.Fprintln(opts.Stderr, "Equivalent command (skip the wizard next time):")
		fmt.Fprintln(opts.Stderr, wizard.EquivalentCommand(pin))
		fmt.Fprintln(opts.Stderr)
	}

	// 4. Validate the resolved pin before committing.
	if err := validatePin(pin); err != nil {
		return err
	}

	// 5. Persist the pin (after wizard, with consent baked in by Options.NoSave).
	if saveAfterWizard {
		if err := persistPin(pinPath, &pin, opts.Stderr); err != nil {
			return fmt.Errorf("save .vb: %w", err)
		}
	}

	// 6. Resolve dockerfile + workspace specs.
	dfSpec, wsSpec, err := buildSpecs(pin, opts)
	if err != nil {
		return err
	}

	// 7. Compute image tag + container name.
	fp := workspace.Fingerprint(wsSpec)
	imageTag := workspace.ImageName(wsSpec, fp)
	containerName := workspace.ContainerName(wsDir, fp)

	// 8. Print the launch plan so the user sees what's about to happen.
	printLaunchPlan(opts.Stderr, wsDir, imageTag, containerName, &pin)

	// 9. Run launch-time prereq checks. These hard-fail on missing
	//    requirements (the wizard's "warn-but-allow" doesn't apply here).
	if err := runLaunchPrereqs(ctx, pin, opts.Stderr); err != nil {
		return err
	}

	// 10. Ensure local LLM provider is reachable, starting it if needed.
	if err := ensureLLMProviderRunning(ctx, pin, opts.Stderr); err != nil {
		return err
	}

	// 11. Decision: container exists → reuse / image exists → run /
	//     neither → build then run.
	dockerCli, err := docker.NewCLIClient()
	if err != nil {
		return err
	}

	status, err := dockerCli.ContainerStatus(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	switch status {
	case "running":
		fmt.Fprintln(opts.Stderr, "→ Container already running — exec'ing in")
		return execIntoContainer(ctx, dockerCli, containerName, wsDir, pin, opts)

	case "exited", "created", "dead":
		fmt.Fprintf(opts.Stderr, "→ Container %s (%s) — starting + exec\n", containerName, status)
		if err := dockerCli.Start(ctx, containerName); err != nil {
			return fmt.Errorf("docker start: %w", err)
		}
		return execIntoContainer(ctx, dockerCli, containerName, wsDir, pin, opts)

	case "":
		// Container doesn't exist. Build image if needed, then run.
		exists, err := dockerCli.ImageExists(ctx, imageTag)
		if err != nil {
			return err
		}
		if !exists || opts.Rebuild {
			if err := buildImage(ctx, dockerCli, dfSpec, imageTag, opts); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(opts.Stderr, "→ Image %s present — skipping build\n", imageTag)
		}
		return runContainer(ctx, dockerCli, imageTag, containerName, wsDir, wsSpec, pin, opts)

	default:
		return fmt.Errorf("unexpected container status %q for %s", status, containerName)
	}
}

// loadWorkspaceAndPin resolves the workspace root and reads any
// existing .vb. Returns (workspaceDir, pin, pinPath, error). When no
// .vb exists, pin is the zero value and pinPath is $PWD/.vb.
func loadWorkspaceAndPin(opts Options) (string, config.Pin, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", config.Pin{}, "", err
	}

	var pin config.Pin
	var pinPath string

	existing, err := config.FindPin(cwd)
	switch {
	case err == nil:
		loaded, err := config.Load(existing)
		if err != nil {
			return "", config.Pin{}, "", fmt.Errorf("load %s: %w", existing, err)
		}
		pin = *loaded
		pinPath = existing
		// Workspace root is the directory the pin lives in.
		fmt.Fprintf(opts.Stderr, "→ Loaded pin: %s\n", existing)
		return filepath.Dir(existing), pin, pinPath, nil
	case errors.Is(err, os.ErrNotExist):
		// No pin — workspace is $PWD; pin will be written at $PWD/.vb if saved.
		return cwd, pin, filepath.Join(cwd, config.PinFileName), nil
	default:
		return "", config.Pin{}, "", err
	}
}

// applyFlagOverrides folds non-empty CLI flag values onto the pin.
// Flags always win — the pin is the saved baseline, flags are the
// per-invocation overlay.
func applyFlagOverrides(pin *config.Pin, opts Options) {
	if opts.Harness != "" {
		pin.Harness = opts.Harness
	}
	if opts.Profile != "" {
		pin.Profile = opts.Profile
	}
	if opts.Shell != "" {
		pin.Shell = opts.Shell
	}
	if len(opts.With) > 0 {
		pin.With = append([]string{}, opts.With...)
	}
	if len(opts.No) > 0 {
		pin.No = append([]string{}, opts.No...)
	}
	if len(opts.CatalogIDs) > 0 {
		pin.Catalog = append([]string{}, opts.CatalogIDs...)
	}
}

// needsWizard reports whether any required field is missing after
// merging flags + pin. If harness is set we consider the pin
// "sufficient" — profile/shell have defaults the resolver can fill in.
func needsWizard(pin config.Pin) bool {
	return pin.Harness == ""
}

// runWizard probes the host, loads the catalog, and runs the wizard.
// Returns the wizard.Result so the caller can check Cancelled.
func runWizard(ctx context.Context, initial config.Pin, wsDir string) (wizard.Result, error) {
	home, _ := os.UserHomeDir()
	detected, _ := hostprobe.ProbeAll(home)

	entries, err := catalog.LoadAll(vibrator.CatalogFS)
	if err != nil {
		return wizard.Result{}, fmt.Errorf("load catalog: %w", err)
	}

	return wizard.Run(ctx, wizard.Input{
		Initial:        initial,
		WorkspaceDir:   wsDir,
		HostDetected:   detected,
		CatalogEntries: entries,
	})
}

// validatePin checks the pin has the minimum data needed to proceed.
// Defaults are applied here so downstream code can assume non-empty
// values.
func validatePin(pin config.Pin) error {
	if pin.Harness == "" {
		return fmt.Errorf("no harness set (pass --harness=... or run without --no-wizard)")
	}
	if _, ok := harness.ByID(pin.Harness); !ok {
		return fmt.Errorf("unknown harness %q (valid: %s)",
			pin.Harness, strings.Join(harness.IDs(), ", "))
	}
	return nil
}

// persistPin saves the pin and (idempotently) ensures `.vb` is
// gitignored. Prints a confirmation line to stderr.
func persistPin(pinPath string, pin *config.Pin, stderr io.Writer) error {
	if pin.Profile == "" {
		pin.Profile = profile.IDFull
	}
	if pin.Shell == "" {
		pin.Shell = "zsh"
	}
	if err := config.Save(pinPath, pin); err != nil {
		return err
	}
	wsDir := filepath.Dir(pinPath)
	if added, err := config.AppendToGitignore(wsDir); err == nil && added {
		fmt.Fprintf(stderr, "→ Added .vb to %s/.gitignore\n", wsDir)
	}
	fmt.Fprintf(stderr, "→ Saved pin: %s\n", pinPath)
	return nil
}

// printLaunchPlan emits a short stderr banner showing the workspace,
// image tag, container name, and LLM choice so the user sees the
// resolved state before any work happens.
func printLaunchPlan(stderr io.Writer, wsDir, imageTag, containerName string, pin *config.Pin) {
	fmt.Fprintf(stderr, "→ Workspace:  %s\n", wsDir)
	fmt.Fprintf(stderr, "  Image:      %s\n", imageTag)
	fmt.Fprintf(stderr, "  Container:  %s\n", containerName)
	if pin.LLM != nil {
		fmt.Fprintf(stderr, "  LLM:        %s / %s\n", pin.LLM.Provider, pin.LLM.Model)
	}
}

// fallbackUsername is the username used when the host user can't be
// detected or isn't safe for Linux useradd (e.g., running as root).
const fallbackUsername = "vibrate"

// validUsername matches the Linux useradd convention: lowercase letters,
// digits, underscores, and dashes only, starting with a letter or
// underscore. macOS allows mixed case and a few odd chars; we sanitize
// them out (HostUsername docs the exact rules).
var validUsername = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// HostUsername returns the host user's name, sanitized for use as a
// Linux container username. Sanitization rules:
//
//   - Lowercase (Linux is case-sensitive, but lowercase is convention
//     and avoids surprising file-ownership-by-name mismatches).
//   - Anything outside [a-z0-9_-] is replaced with `_`.
//   - If the first char isn't a letter or `_`, prepend `_`.
//   - Truncated to 32 chars (Linux useradd's NAME_REGEX default).
//   - Falls back to "vibrate" if detection fails OR the sanitized
//     result is empty OR the host user is root (UID 0 — useradd at 0
//     would clash with the container's existing root).
//
// Exported so the CLI layer can compute the same default value at
// flag-parse time (for `--help` output) that the orchestrator uses at
// runtime — single source of truth, no drift.
func HostUsername() string {
	u, err := user.Current()
	if err != nil || u == nil {
		return fallbackUsername
	}
	if u.Uid == "0" {
		return fallbackUsername
	}
	cleaned := sanitizeUsername(u.Username)
	if cleaned == "" {
		return fallbackUsername
	}
	return cleaned
}

// sanitizeUsername applies the rules documented on HostUsername.
// Exposed as a separate function so it can be unit-tested without
// touching the real OS user database.
func sanitizeUsername(raw string) string {
	if raw == "" {
		return ""
	}
	// Lowercase + replace invalid chars with `_`.
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range strings.ToLower(raw) {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	// Must start with a letter or `_` — if it doesn't, prepend `_`.
	if out != "" {
		first := out[0]
		if !((first >= 'a' && first <= 'z') || first == '_') {
			out = "_" + out
		}
	}
	// Truncate to 32 chars (useradd NAME_REGEX default).
	if len(out) > 32 {
		out = out[:32]
	}
	// Final validation — if we somehow still don't match, give up.
	if !validUsername.MatchString(out) {
		return ""
	}
	return out
}

// defaultUsername resolves the username baked into the container.
// Honors `--username` if explicitly set, otherwise derives from the
// host user via HostUsername. Falls back to "vibrate" only when
// HostUsername can't derive a safe value.
func defaultUsername(opts Options) string {
	if opts.Username != "" {
		return opts.Username
	}
	return HostUsername()
}

// defaultUID resolves the host UID to bake in.
func defaultUID(opts Options) int {
	if opts.HostUID != 0 {
		return opts.HostUID
	}
	return os.Getuid()
}

// defaultGID resolves the host GID to bake in.
func defaultGID(opts Options) int {
	if opts.HostGID != 0 {
		return opts.HostGID
	}
	return os.Getgid()
}

// buildSpecs materializes the dockerfile + workspace specs the rest of
// the orchestrator works with. Most of the logic mirrors what
// internal/cli/build.go's resolveSpec does — same precedence rules
// (flags > pin > defaults).
func buildSpecs(pin config.Pin, opts Options) (dockerfile.Spec, workspace.Spec, error) {
	h, ok := harness.ByID(pin.Harness)
	if !ok {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("unknown harness %q", pin.Harness)
	}

	profileID := pin.Profile
	if profileID == "" {
		profileID = profile.IDFull
	}
	p, ok := profile.ByID(profileID)
	if !ok {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("unknown profile %q", profileID)
	}

	shell := pin.Shell
	if shell == "" {
		shell = "zsh"
	}

	// Catalog entries: validate that every requested ID exists. Loaded
	// before feature resolution so each entry's `deps.features` can be
	// folded into the feature `initial` set — without this step the
	// `deps:` declarations in catalog files are documentation-only and
	// the install snippets blow up at build time (e.g., `npm: not found`
	// when a node-dependent MCP is selected under a non-node profile).
	var catEntries []*catalog.Entry
	if len(pin.Catalog) > 0 {
		all, err := catalog.LoadAll(vibrator.CatalogFS)
		if err != nil {
			return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("load catalog: %w", err)
		}
		for _, id := range pin.Catalog {
			key := h.ID() + "/" + id
			entry, ok := all[key]
			if !ok {
				return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf(
					"catalog entry %q not found for harness %q", id, h.ID())
			}
			catEntries = append(catEntries, entry)
		}
	}

	// Resolve features: profile + harness-required + catalog deps, then
	// with/no deltas. Catalog deps land in `initial` (same tier as
	// harness requirements) so `--no` can still strip them if the user
	// really insists — matching the existing precedence pattern.
	initial := append([]string{}, p.Features...)
	initial = append(initial, h.RequiredFeatures()...)
	for _, e := range catEntries {
		initial = append(initial, e.Deps.Features...)
	}
	resolved, err := feature.Resolve(initial, pin.With, pin.No)
	if err != nil {
		return dockerfile.Spec{}, workspace.Spec{}, fmt.Errorf("resolve features: %w", err)
	}

	feats := make([]feature.Feature, 0, len(resolved.Enabled))
	for _, id := range resolved.Enabled {
		fe, _ := feature.ByID(id)
		feats = append(feats, fe)
	}

	dfSpec := dockerfile.Spec{
		Harness:         h,
		Profile:         profileID,
		Shell:           shell,
		Features:        feats,
		CatalogEntries:  catEntries,
		Username:        defaultUsername(opts),
		HostUID:         defaultUID(opts),
		HostGID:         defaultGID(opts),
		VibratorVersion: opts.VibratorVersion,
	}

	wsSpec := workspace.Spec{
		Harness:  h.ID(),
		Profile:  profileID,
		Shell:    shell,
		Features: resolved.Enabled,
		Catalog:  pin.Catalog,
	}
	return dfSpec, wsSpec, nil
}

// hostnameOrDefault returns os.Hostname() with a fallback to
// "unknown" so probe-derived identifiers (e.g., claude-mem actor_id)
// don't carry empty strings.
func hostnameOrDefault() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

// platformLabel surfaces "linux/amd64" or similar for `vibrate
// variants list`'s `Built on` column. Best-effort.
func platformLabel() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
