// Package app is the orchestrator that powers the top-level `vibrate`
// command. It glues all the pieces together — flag resolution, .vb
// loading, the wizard, prereq probes, local LLM provider startup,
// build, run, and exec.
//
// # Decision tree
//
//	.vb exists? → resolve spec → image exists? → container exists?
//	                                               ├─ running → docker exec
//	                                               ├─ stopped → docker start + exec
//	                                               ├─ image only → docker run
//	                                               └─ none → build → run
//	.vb missing → wizard → save .vb → build → run
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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/docker"
	"github.com/wlame/vibrator/internal/dockerfile"
	"github.com/wlame/vibrator/internal/extensions"
	"github.com/wlame/vibrator/internal/feature"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/hostprobe"
	"github.com/wlame/vibrator/internal/mount"
	"github.com/wlame/vibrator/internal/profile"
	"github.com/wlame/vibrator/internal/wizard"
	"github.com/wlame/vibrator/internal/workspace"
)

// LaunchTarget selects which command the orchestrator exec's inside
// the container. The default (LaunchHarness, also represented by the
// zero value "") preserves the new "bare vibrate launches the agent"
// UX; LaunchShell keeps the escape-hatch behaviour previously offered
// by bare vibrate.
type LaunchTarget string

const (
	// LaunchHarness exec's the harness's own CLI (claude / codex /
	// opencode / pi). The default for `vibrate` and `vibrate run`.
	LaunchHarness LaunchTarget = "harness"

	// LaunchShell exec's the user's shell instead. Triggered by
	// `vibrate shell`; preserves the legacy "drop me in a shell"
	// behaviour for debugging, installing extras, one-off commands.
	LaunchShell LaunchTarget = "shell"
)

// dindLabelKey is the container label that records whether a container
// was created with the host docker socket mounted (--dind). It lives in
// app (set in launch.go, read in resolveAndLaunch) because it's an
// orchestration concern, not a docker-client detail.
const dindLabelKey = "vibrator.dind"

// identityLabelKey records the [identity] override a container was created
// with — stored as a fingerprint (not the alias itself) so resolveAndLaunch
// can detect a change and recreate the container. Like dindLabelKey, it's a
// runtime concern: identity flows in via env vars + the entrypoint, neither
// of which can be retrofitted onto a live container.
const identityLabelKey = "vibrator.identity"

// mountsLabelKey records a fingerprint of the extra --mount set a container
// was created with, so resolveAndLaunch can recreate it when the set
// changes (bind mounts can't be added to a live container).
const mountsLabelKey = "vibrator.mounts"

// identityFingerprint returns a short, stable hash of the pin's identity
// override (name + email), or "" when no override is set. Hashing keeps the
// alias out of docker labels while still letting us detect a change.
func identityFingerprint(pin config.Pin) string {
	if pin.Identity == nil || (pin.Identity.Name == "" && pin.Identity.Email == "") {
		return ""
	}
	sum := sha256.Sum256([]byte(pin.Identity.Name + "\x00" + pin.Identity.Email))
	return hex.EncodeToString(sum[:8])
}

// mountsFingerprint resolves the pin's extra mounts and returns their
// fingerprint, matching what runContainer stamps on the container. A bad
// --mount aborts here (fail-fast) before any reuse decision.
func mountsFingerprint(pin config.Pin, wsDir string) (string, error) {
	rs, err := mount.ResolveAll(pin.Mounts, wsDir)
	if err != nil {
		return "", err
	}
	return mount.Fingerprint(rs), nil
}

// effectiveLaunchTarget normalizes the zero-value to LaunchHarness so
// downstream consumers can branch on a known value without a special
// case for the empty string.
func (lt LaunchTarget) effective() LaunchTarget {
	if lt == "" {
		return LaunchHarness
	}
	return lt
}

// Options bundles the flag state passed from the CLI layer. Mirrors
// `vibrate build`'s flags plus a few orchestrator-only knobs.
type Options struct {
	// CLI flag overrides — non-empty wins over .vb values.
	Harness      string
	Profile      string
	Shell        string
	With         []string
	No           []string
	ExtensionIDs []string
	// Mounts are extra host folders to bind into the container at the same
	// absolute path, as raw "PATH[:ro|:rw]" entries (read-only default).
	Mounts   []string
	Username string
	HostUID  int
	HostGID  int

	// NoWizard, when true, skips the interactive wizard entirely. Falls
	// back to flags/defaults — useful for scripted invocations and CI.
	NoWizard bool

	// NoSave, when true, doesn't write the wizard's result to .vb.
	// Useful for "try this combo once" invocations.
	NoSave bool

	// Rebuild forces a fresh `docker build` even when a matching image
	// exists. Same as `--no-cache` at the docker level.
	Rebuild bool

	// LaunchTarget selects what `vibrate` exec's inside the container.
	// The default ("" or LaunchHarness) runs the harness's own CLI
	// (claude / codex / opencode / pi) directly — that's the bare
	// `vibrate` UX. LaunchShell drops into the user's shell instead —
	// what `vibrate shell` does, useful for debugging, installing
	// extras, or running one-off commands.
	LaunchTarget LaunchTarget

	// LoginMode, when true, runs `claude auth login` inside the container
	// before launching the harness. The container is started detached
	// (sleep infinity) so the login exec can intercept the auth URL and
	// open the host browser automatically. Auth state is written back to
	// the host's ~/.claude.json so subsequent launches are pre-authenticated.
	// Skipped silently when the host config already has oauthAccount.
	LoginMode bool

	// DinD enables Docker-in-Docker: the host's docker socket is
	// bind-mounted into the container and the container user is added
	// to the docker group so they can `docker` against the host daemon
	// without sudo. Opt-in because mounting the socket grants the
	// container ~root-equivalent on the host (it can run any docker
	// command, including --privileged ones).
	DinD bool

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

	// Explicit --username reaches the Dockerfile generator; reject bad
	// values here for a clean CLI error before any docker work. (The
	// generator re-checks as defense in depth.)
	if err := dockerfile.ValidateUsername(opts.Username); err != nil {
		return err
	}

	// 5. Persist the pin (after wizard, with consent baked in by Options.NoSave).
	if saveAfterWizard {
		if err := persistPin(pinPath, &pin, opts.Stderr); err != nil {
			return fmt.Errorf("save .vb: %w", err)
		}
	}

	// 5b. Hook-tool readiness: warn when host Claude hooks need a tool this
	//     image won't bake (e.g. node hooks under the minimal profile), and
	//     optionally install the feature or remember the user's choice. Runs
	//     before buildSpecs so an accepted "install" feeds feature resolution.
	if updated, dirty := runHookReadiness(pin, opts); dirty {
		pin = updated
		if !opts.NoSave {
			if err := persistPin(pinPath, &pin, opts.Stderr); err != nil {
				return fmt.Errorf("save .vb after hook check: %w", err)
			}
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

	// 9. Run integration readiness checks. Warns when an integration the
	//    workspace uses is not fully configured; offers inline bootstrap for
	//    fixable gaps. Never aborts the launch — a dormant integration is
	//    better than blocking the user from reaching their container.
	var pinDirty bool
	pin, pinDirty, err = runIntegrationReadiness(ctx, pin, wsDir, opts)
	if err != nil {
		return err
	}
	if pinDirty {
		if err := persistPin(pinPath, &pin, opts.Stderr); err != nil {
			return fmt.Errorf("save .vb after inline bootstrap: %w", err)
		}
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

	return resolveAndLaunch(ctx, dockerCli, dfSpec, wsSpec, pin, wsDir, imageTag, containerName, opts)
}

// Indirection seams for resolveAndLaunch. They default to the real
// implementations in launch.go but can be swapped in tests so the decision
// logic (which branch runs, whether --rebuild tears the container down)
// can be exercised without performing a real docker build, run, or exec.
var (
	buildImageFn        = buildImage
	runContainerFn      = runContainer
	execIntoContainerFn = execIntoContainer
	waitForEntrypointFn = waitForEntrypoint
	runLoginStepFn      = runLoginStep
)

// resolveAndLaunch decides how to bring the workspace up given the current
// state of its container and image, then launches it. The four outcomes:
//
//   - --rebuild set: tear down any existing container, rebuild the image
//     from scratch (--no-cache), run fresh. Checked first so a running or
//     stopped container can't short-circuit the rebuild.
//   - container running: exec into it.
//   - container stopped (exited/created/dead): start it, then exec.
//   - no container: build the image if missing, then run.
//
// It takes the docker.Client as a parameter (rather than constructing one)
// so the decision logic can be unit-tested with a mock client.
func resolveAndLaunch(ctx context.Context, dc docker.Client,
	dfSpec dockerfile.Spec, wsSpec workspace.Spec, pin config.Pin,
	wsDir, imageTag, containerName string, opts Options,
) error {
	status, err := dc.ContainerStatus(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	// --rebuild forces a from-scratch image build and a fresh container.
	// This must be handled before the reuse switch below: otherwise an
	// already running/stopped container short-circuits to exec/start and
	// the flag is silently ignored. Tear down the existing container (if
	// any), rebuild the image with --no-cache, then run fresh.
	if opts.Rebuild {
		if status != "" {
			fmt.Fprintf(opts.Stderr, "→ --rebuild: removing existing container %s (%s)\n", containerName, status)
			if err := dc.Remove(ctx, docker.RemoveContainer, containerName, true); err != nil {
				return fmt.Errorf("remove container for rebuild: %w", err)
			}
		}
		if err := buildImageFn(ctx, dc, dfSpec, imageTag, opts); err != nil {
			return err
		}
		if !opts.LoginMode {
			return runContainerFn(ctx, dc, imageTag, containerName, wsDir, wsSpec, pin, dfSpec.Extensions, opts)
		}
		if err := runContainerFn(ctx, dc, imageTag, containerName, wsDir, wsSpec, pin, dfSpec.Extensions, opts); err != nil {
			return err
		}
		if err := waitForEntrypointFn(ctx, dc, containerName); err != nil {
			fmt.Fprintf(opts.Stderr, "⚠  entrypoint readiness check timed out: %v\n", err)
		}
		if err := runLoginStepFn(ctx, dc, containerName, defaultUsername(opts), opts); err != nil {
			fmt.Fprintf(opts.Stderr, "⚠  login step failed: %v\n", err)
		}
		return execIntoContainerFn(ctx, dc, containerName, wsDir, pin, opts)
	}

	// Runtime-state recreate: some settings are baked into a container at
	// creation time and CANNOT be changed on a live container — the --dind
	// socket mount and the [identity] override (env vars + entrypoint
	// rewrite). When the request differs from how the existing container was
	// created, recreate it from the EXISTING image (no rebuild; image content
	// is identical). This is what makes `vibrate --dind` (or a freshly-set
	// alias) take effect on a prior container without a from-scratch build —
	// and, for identity, ensures a privacy alias can't silently fail to apply
	// because an old container leaking the real email got reused.
	if status != "" {
		var reason string

		if haveDinD, err := containerHasDinD(ctx, dc, containerName); err != nil {
			return fmt.Errorf("inspect container dind state: %w", err)
		} else if haveDinD != opts.DinD {
			reason = fmt.Sprintf("--dind state changed (was %v, now %v)", haveDinD, opts.DinD)
		}

		if reason == "" {
			if haveID, err := dc.ContainerLabel(ctx, containerName, identityLabelKey); err != nil {
				return fmt.Errorf("inspect container identity state: %w", err)
			} else if haveID != identityFingerprint(pin) {
				reason = "identity ([identity] in .vb) changed"
			}
		}

		if reason == "" {
			wantFP, err := mountsFingerprint(pin, wsDir)
			if err != nil {
				return err // bad --mount: fail before touching the container
			}
			if haveFP, err := dc.ContainerLabel(ctx, containerName, mountsLabelKey); err != nil {
				return fmt.Errorf("inspect container mounts state: %w", err)
			} else if haveFP != wantFP {
				reason = "--mount set changed"
			}
		}

		if reason != "" {
			fmt.Fprintf(opts.Stderr,
				"→ %s — recreating container %s from the existing image (no rebuild)\n",
				reason, containerName)
			if err := dc.Remove(ctx, docker.RemoveContainer, containerName, true); err != nil {
				return fmt.Errorf("remove container for runtime-state change: %w", err)
			}
			status = "" // fall through to the build-if-missing + run path
		}
	}

	switch status {
	case "running":
		fmt.Fprintln(opts.Stderr, "→ Container already running — exec'ing in")
		if opts.LoginMode {
			if err := runLoginStepFn(ctx, dc, containerName, defaultUsername(opts), opts); err != nil {
				fmt.Fprintf(opts.Stderr, "⚠  login step failed: %v\n", err)
			}
		}
		return execIntoContainerFn(ctx, dc, containerName, wsDir, pin, opts)

	case "exited", "created", "dead":
		fmt.Fprintf(opts.Stderr, "→ Container %s (%s) — starting + exec\n", containerName, status)
		if err := dc.Start(ctx, containerName); err != nil {
			return fmt.Errorf("docker start: %w", err)
		}
		if opts.LoginMode {
			if err := runLoginStepFn(ctx, dc, containerName, defaultUsername(opts), opts); err != nil {
				fmt.Fprintf(opts.Stderr, "⚠  login step failed: %v\n", err)
			}
		}
		return execIntoContainerFn(ctx, dc, containerName, wsDir, pin, opts)

	case "":
		// Container doesn't exist. Build image if needed, then run.
		exists, err := dc.ImageExists(ctx, imageTag)
		if err != nil {
			return err
		}
		if !exists {
			if err := buildImageFn(ctx, dc, dfSpec, imageTag, opts); err != nil {
				return err
			}
		} else {
			fmt.Fprintf(opts.Stderr, "→ Image %s present — skipping build\n", imageTag)
		}
		// LoginMode: start detached, do login, then exec into the harness.
		// Normal mode: docker run -it blocks for the entire session.
		if !opts.LoginMode {
			return runContainerFn(ctx, dc, imageTag, containerName, wsDir, wsSpec, pin, dfSpec.Extensions, opts)
		}
		if err := runContainerFn(ctx, dc, imageTag, containerName, wsDir, wsSpec, pin, dfSpec.Extensions, opts); err != nil {
			return err
		}
		if err := waitForEntrypointFn(ctx, dc, containerName); err != nil {
			fmt.Fprintf(opts.Stderr, "⚠  entrypoint readiness check timed out: %v\n", err)
		}
		if err := runLoginStepFn(ctx, dc, containerName, defaultUsername(opts), opts); err != nil {
			fmt.Fprintf(opts.Stderr, "⚠  login step failed: %v\n", err)
		}
		return execIntoContainerFn(ctx, dc, containerName, wsDir, pin, opts)

	default:
		return fmt.Errorf("unexpected container status %q for %s", status, containerName)
	}
}

// containerHasDinD reports whether an existing container was created with
// the host docker socket mounted, by reading the vibrator.dind label.
// A missing label (older container, or label absent) reads as false.
func containerHasDinD(ctx context.Context, dc docker.Client, containerName string) (bool, error) {
	v, err := dc.ContainerLabel(ctx, containerName, dindLabelKey)
	if err != nil {
		return false, err
	}
	return v == "true", nil
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
	if len(opts.ExtensionIDs) > 0 {
		pin.Extensions = append([]string{}, opts.ExtensionIDs...)
	}
	if len(opts.Mounts) > 0 {
		pin.Mounts = append([]string{}, opts.Mounts...)
	}
}

// needsWizard reports whether any required field is missing after
// merging flags + pin. If harness is set we consider the pin
// "sufficient" — profile/shell have defaults the resolver can fill in.
func needsWizard(pin config.Pin) bool {
	return pin.Harness == ""
}

// runWizard probes the host, loads the extensions, and runs the wizard.
// Returns the wizard.Result so the caller can check Cancelled.
func runWizard(ctx context.Context, initial config.Pin, wsDir string) (wizard.Result, error) {
	home, _ := os.UserHomeDir()
	detected, _ := hostprobe.ProbeAll(home)

	entries, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		return wizard.Result{}, fmt.Errorf("load extensions: %w", err)
	}

	return wizard.Run(ctx, wizard.Input{
		Initial:      initial,
		WorkspaceDir: wsDir,
		HostDetected: detected,
		Extensions:   entries,
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
	// The shell value from .vb reaches `docker exec` argv ("/bin/"+shell)
	// even on container-reuse paths that never run the Dockerfile
	// generator's own validation — so reject bad values here, up front.
	if pin.Shell != "" && !dockerfile.SupportedShell(pin.Shell) {
		return fmt.Errorf("unsupported shell %q in .vb (valid: bash, zsh, fish)", pin.Shell)
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
	if dockerfile.ValidateUsername(out) != nil {
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

// resolveExtensionsAndFeatures loads+validates the pin's extension entries and
// computes the final enabled feature list (profile + harness-required +
// extension deps + optional docker-cli, with `with`/`no` deltas applied).
//
// Shared by buildSpecs and runHookReadiness so both see an identical feature
// set — the hook check must reason about exactly the features the build will
// bake, or it would prompt about a tool that's actually present (or miss one).
func resolveExtensionsAndFeatures(pin config.Pin, opts Options) ([]*extensions.Entry, []string, error) {
	h, ok := harness.ByID(pin.Harness)
	if !ok {
		return nil, nil, fmt.Errorf("unknown harness %q", pin.Harness)
	}

	profileID := pin.Profile
	if profileID == "" {
		profileID = profile.IDFull
	}
	p, ok := profile.ByID(profileID)
	if !ok {
		return nil, nil, fmt.Errorf("unknown profile %q", profileID)
	}

	// Extensions entries: validate that every requested ID exists. Loaded
	// before feature resolution so each entry's `deps.features` can be
	// folded into the feature `initial` set — without this step the
	// `deps:` declarations in extensions files are documentation-only and
	// the install snippets blow up at build time (e.g., `npm: not found`
	// when a node-dependent MCP is selected under a non-node profile).
	var catEntries []*extensions.Entry
	if len(pin.Extensions) > 0 {
		all, err := extensions.LoadAll(vibrator.ExtensionsFS)
		if err != nil {
			return nil, nil, fmt.Errorf("load extensions: %w", err)
		}
		for _, id := range pin.Extensions {
			key := h.ID() + "/" + id
			entry, ok := all[key]
			if !ok {
				return nil, nil, fmt.Errorf(
					"extensions entry %q not found for harness %q", id, h.ID())
			}
			catEntries = append(catEntries, entry)
		}
	}

	// Resolve features: profile + harness-required + extensions deps, then
	// with/no deltas. Extensions deps land in `initial` (same tier as
	// harness requirements) so `--no` can still strip them if the user
	// really insists — matching the existing precedence pattern.
	initial := append([]string{}, p.Features...)
	initial = append(initial, h.RequiredFeatures()...)
	for _, e := range catEntries {
		initial = append(initial, e.Deps.Features...)
	}
	// NB: --dind deliberately does NOT add any feature here. The docker CLI
	// is baked into the base image for every variant, so the image content
	// is identical with or without --dind. That keeps toggling --dind a
	// run-time-only decision (socket mount + container recreate), never an
	// image rebuild. See resolveAndLaunch for the container-side handling.
	resolved, err := feature.Resolve(initial, pin.With, pin.No)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve features: %w", err)
	}
	return catEntries, resolved.Enabled, nil
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
	shell := pin.Shell
	if shell == "" {
		shell = "zsh"
	}

	catEntries, enabled, err := resolveExtensionsAndFeatures(pin, opts)
	if err != nil {
		return dockerfile.Spec{}, workspace.Spec{}, err
	}

	feats := make([]feature.Feature, 0, len(enabled))
	for _, id := range enabled {
		fe, _ := feature.ByID(id)
		feats = append(feats, fe)
	}

	dfSpec := dockerfile.Spec{
		Harness:         h,
		Profile:         profileID,
		Shell:           shell,
		Features:        feats,
		Extensions:      catEntries,
		Username:        defaultUsername(opts),
		HostUID:         defaultUID(opts),
		HostGID:         defaultGID(opts),
		VibratorVersion: opts.VibratorVersion,
	}

	wsSpec := workspace.Spec{
		Harness:    h.ID(),
		Profile:    profileID,
		Shell:      shell,
		Features:   enabled,
		Extensions: pin.Extensions,
		Username:   defaultUsername(opts),
	}
	return dfSpec, wsSpec, nil
}
