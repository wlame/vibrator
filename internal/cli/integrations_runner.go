package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/integration"
)

// runIntegration is the generic flow for any registered Integration.
// It handles:
//
//   - Header (name, summary, docs URL)
//   - Status detection (which runtime is active, or if reachable via probe
//     without any local runtime — the "external" case)
//   - Management menu when running: show URL, tail logs, stop, restart
//   - Start flow when stopped: pick runtime, start, await probe
//   - Workspace bootstrap when WorkspaceDriver is set and probe succeeds
//
// Integrations with bespoke setup (claude-mem's DSN flow) can call
// their own setup code first and then delegate to runIntegration for
// the rest. Pure declarative integrations (Serena, future TOML
// imports) call this directly with no preamble.
func runIntegration(cmd *cobra.Command, integ *integration.Integration) error {
	out := cmd.OutOrStdout()
	c := newColors(!isTerminal(out))
	ctx := cmdContext(cmd)

	renderIntegrationHeader(out, integ, c)

	// Step 1 — detect state. activeRT is the runtime currently running,
	// or nil if none are running. reachable is true when the probe
	// succeeds (covers the "user manages externally" case).
	activeRT, _ := detectActiveRuntime(ctx, integ)
	probe := safeProbe(ctx, integ)
	reachable := probe != nil && probe.Check(ctx) == nil

	if activeRT != nil || reachable {
		if err := manageIntegration(cmd, integ, activeRT, probe, c); err != nil {
			return err
		}
	} else {
		if err := startIntegration(cmd, integ, c); err != nil {
			return err
		}
		// Re-evaluate reachability after start. (activeRT isn't read again
		// past this point, so we don't recompute it.)
		probe = safeProbe(ctx, integ)
		reachable = probe != nil && probe.Check(ctx) == nil
	}

	// Step 2 — workspace bootstrap, when applicable.
	if integ.Workspace != nil && reachable {
		return runWorkspaceBootstrap(cmd, integ, c)
	}
	return nil
}

// renderIntegrationHeader prints the standard banner shown by every
// integration flow. Kept consistent across integrations so the user
// learns one visual pattern.
func renderIntegrationHeader(out io.Writer, integ *integration.Integration, c colors) {
	fmt.Fprintf(out, "\n%s%s%s — %s\n", c.bold, integ.Name, c.reset, integ.Summary)
	fmt.Fprintln(out, strings.Repeat("─", 60))
	if integ.DocsURL != "" {
		fmt.Fprintf(out, "  %sDocs:%s   %s\n", c.dim, c.reset, integ.DocsURL)
	}
	if integ.AdminConfig != nil && integ.AdminConfig.Path != "" {
		fmt.Fprintf(out, "  %sConfig:%s %s\n", c.dim, c.reset, integ.AdminConfig.Path)
	}
	fmt.Fprintln(out)
}

// detectActiveRuntime queries each registered HostRuntime's Status and
// returns the first one reporting Running=true. Returns (nil, status)
// when no runtime is active (status is zero-value).
func detectActiveRuntime(ctx context.Context, integ *integration.Integration) (integration.HostRuntime, integration.RuntimeStatus) {
	for _, rt := range integ.Runtimes {
		s, err := rt.Status(ctx)
		if err != nil {
			continue
		}
		if s.Running {
			return rt, s
		}
	}
	return nil, integration.RuntimeStatus{}
}

// safeProbe calls ProbeFn and returns the Probe, or nil on any error
// or a nil probe response. The wider runner treats nil-probe as
// "skip reachability check" — same semantics as the `list` command.
func safeProbe(ctx context.Context, integ *integration.Integration) integration.Probe {
	if integ.ProbeFn == nil {
		return nil
	}
	p, err := integ.ProbeFn(ctx)
	if err != nil {
		return nil
	}
	return p
}

// cmdContext extracts a usable Context from a cobra.Command. cobra sets
// ctx when invoked via cmd.ExecuteContext, but falls through to nil for
// other entry points. Default to Background to keep the runner usable
// from tests and from the bare-default-RunE path.
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// ── manage flow ─────────────────────────────────────────────────────────

// manageIntegration loops on a "what would you like to do?" menu when
// the integration is running. activeRT is the runtime backing the
// running instance, or nil when probe-reachable but no local runtime
// is responsible (the "externally managed" path). probe MAY be nil if
// the integration has no ProbeFn.
func manageIntegration(
	cmd *cobra.Command,
	integ *integration.Integration,
	activeRT integration.HostRuntime,
	probe integration.Probe,
	c colors,
) error {
	out := cmd.OutOrStdout()
	ctx := cmdContext(cmd)

	if activeRT != nil {
		s, _ := activeRT.Status(ctx)
		fmt.Fprintf(out, "  %s✓%s Running via %s", c.green, c.reset, activeRT.Kind())
		switch {
		case s.PID != 0:
			fmt.Fprintf(out, " (PID %d)", s.PID)
		case s.Container != "":
			fmt.Fprintf(out, " (container %s)", s.Container)
		}
		fmt.Fprintln(out)
		if s.Detail != "" {
			fmt.Fprintf(out, "    %s%s%s\n", c.dim, s.Detail, c.reset)
		}
	} else {
		fmt.Fprintf(out, "  %s✓%s Reachable (externally managed)\n", c.green, c.reset)
	}
	fmt.Fprintln(out)

	for {
		action, err := pickManageAction(integ, activeRT)
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				return nil
			}
			return err
		}

		switch action {
		case "url":
			showWiringURL(out, integ, probe, c)
		case "tail":
			tailRuntimeLogs(ctx, out, activeRT, c)
		case "stop":
			return stopActiveRuntime(ctx, out, activeRT, c)
		case "restart":
			if err := restartActiveRuntime(ctx, out, integ, activeRT, c); err != nil {
				return err
			}
		case "done":
			return nil
		}
	}
}

// pickManageAction renders the management menu and returns the chosen
// action ID. Action options vary based on whether a local runtime owns
// the instance (logs/stop/restart only make sense then).
func pickManageAction(integ *integration.Integration, activeRT integration.HostRuntime) (string, error) {
	var action string
	opts := []huh.Option[string]{
		huh.NewOption("Show container URL", "url"),
	}
	if activeRT != nil {
		opts = append(opts,
			huh.NewOption("Tail logs", "tail"),
			huh.NewOption("Stop", "stop"),
			huh.NewOption("Restart", "restart"),
		)
	}
	opts = append(opts, huh.NewOption("Done", "done"))

	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Manage " + integ.Name).
			Options(opts...).
			Value(&action),
	)).WithTheme(huh.ThemeCharm())
	err := form.Run()
	return action, err
}

// showWiringURL prints the URL containers should use to reach the
// integration. Prefers the first MCP-wiring HTTP URL (that's the
// container-side address); falls back to the probe's Describe (which
// for HTTPProbe is the probe URL — host-side, less useful but better
// than nothing).
func showWiringURL(out io.Writer, integ *integration.Integration, probe integration.Probe, c colors) {
	url := firstMCPURL(integ)
	if url == "" && probe != nil {
		url = probe.Describe()
	}
	if url == "" {
		fmt.Fprintf(out, "\n  %sNo URL declared for this integration.%s\n\n", c.dim, c.reset)
		return
	}
	fmt.Fprintf(out, "\n  Container URL: %s\n\n", url)
}

func firstMCPURL(integ *integration.Integration) string {
	for _, w := range integ.Wiring {
		if w.MCP != nil && w.MCP.HTTP != nil && w.MCP.HTTP.URL != "" {
			return w.MCP.HTTP.URL
		}
	}
	return ""
}

func tailRuntimeLogs(ctx context.Context, out io.Writer, rt integration.HostRuntime, c colors) {
	if rt == nil {
		fmt.Fprintf(out, "\n  %sLogs unavailable for externally-managed instance.%s\n\n", c.dim, c.reset)
		return
	}
	log, err := rt.Logs(ctx, 8192)
	if err != nil {
		fmt.Fprintf(out, "\n  %slog error: %v%s\n\n", c.red, err, c.reset)
		return
	}
	if strings.TrimSpace(log) == "" {
		fmt.Fprintf(out, "\n  %s(log is empty)%s\n\n", c.dim, c.reset)
		return
	}
	fmt.Fprintf(out, "\n%s\n\n", log)
}

func stopActiveRuntime(ctx context.Context, out io.Writer, rt integration.HostRuntime, c colors) error {
	if rt == nil {
		fmt.Fprintf(out, "  %sExternal instance — nothing for vibrator to stop.%s\n\n", c.dim, c.reset)
		return nil
	}
	fmt.Fprintf(out, "  Stopping (%s) …\n", rt.Kind())
	if err := rt.Stop(ctx); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	fmt.Fprintf(out, "  %s✓%s Stopped.\n\n", c.green, c.reset)
	return nil
}

func restartActiveRuntime(
	ctx context.Context,
	out io.Writer,
	integ *integration.Integration,
	rt integration.HostRuntime,
	c colors,
) error {
	if rt == nil {
		fmt.Fprintf(out, "  %sExternal instance — restart by hand.%s\n\n", c.dim, c.reset)
		return nil
	}
	fmt.Fprintf(out, "  Restarting (%s) …\n", rt.Kind())
	if err := rt.Stop(ctx); err != nil {
		return fmt.Errorf("restart-stop: %w", err)
	}
	if err := rt.Start(ctx); err != nil {
		return fmt.Errorf("restart-start: %w", err)
	}
	awaitProbe(ctx, out, integ, c)
	return nil
}

// ── start flow ──────────────────────────────────────────────────────────

// startIntegration presents a "how would you like to start this?"
// picker over the integration's Runtimes, starts the chosen one, then
// polls the probe until reachable (or timeout).
func startIntegration(cmd *cobra.Command, integ *integration.Integration, c colors) error {
	out := cmd.OutOrStdout()
	ctx := cmdContext(cmd)

	fmt.Fprintf(out, "  %s✗%s Not running.\n\n", c.red, c.reset)

	if len(integ.Runtimes) == 0 {
		fmt.Fprintf(out, "  This integration has no declared runtimes — start it externally.\n\n")
		return nil
	}

	rt, err := pickRuntime(integ)
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
		return err
	}
	if rt == nil {
		return nil // user picked nothing (shouldn't happen, defensive)
	}

	// External-runtime path: print instructions, don't try to start.
	if ext, ok := rt.(*integration.ExternalRuntime); ok {
		if ext.Instructions != "" {
			fmt.Fprintf(out, "\n  %sInstructions:%s\n\n%s\n\n", c.bold, c.reset, indent(ext.Instructions, "    "))
		}
		// Poll briefly in case the user wires it up while we wait — the
		// generic flow continues to bootstrap if probe succeeds.
		return nil
	}

	fmt.Fprintf(out, "\n  Starting (%s) …\n", rt.Kind())
	if err := rt.Start(ctx); err != nil {
		fmt.Fprintf(out, "  %s✗%s start failed: %v\n\n", c.red, c.reset, err)
		return nil
	}
	fmt.Fprintf(out, "  %s✓%s Started.\n", c.green, c.reset)

	awaitProbe(ctx, out, integ, c)
	return nil
}

// pickRuntime renders the runtime-mode picker. When only one runtime
// is registered, skips the form and returns it directly.
func pickRuntime(integ *integration.Integration) (integration.HostRuntime, error) {
	if len(integ.Runtimes) == 1 {
		return integ.Runtimes[0], nil
	}
	opts := make([]huh.Option[string], 0, len(integ.Runtimes))
	for _, rt := range integ.Runtimes {
		opts = append(opts, huh.NewOption(rt.Label(), rt.Kind()))
	}
	var kind string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("How would you like to start " + integ.Name + "?").
			Options(opts...).
			Value(&kind),
	)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		return nil, err
	}
	for _, rt := range integ.Runtimes {
		if rt.Kind() == kind {
			return rt, nil
		}
	}
	return nil, nil
}

// awaitProbe polls the integration's probe for up to 90 s, printing a
// single-line status. Used after Start and Restart. Non-fatal — if the
// probe never succeeds we print an error but let the user continue.
func awaitProbe(ctx context.Context, out io.Writer, integ *integration.Integration, c colors) {
	probe := safeProbe(ctx, integ)
	if probe == nil {
		fmt.Fprintf(out, "  %s(no probe declared — assuming ready)%s\n\n", c.dim, c.reset)
		return
	}
	fmt.Fprintf(out, "  Waiting for reachability …")
	err := probe.Wait(ctx, 90*time.Second)
	if err != nil {
		fmt.Fprintf(out, "\r  %s✗%s Not reachable within 90 s.                          \n",
			c.red, c.reset)
		fmt.Fprintf(out, "    %slast probe error: %v%s\n\n", c.dim, err, c.reset)
		return
	}
	fmt.Fprintf(out, "\r  %s✓%s Reachable at %s                          \n",
		c.green, c.reset, probe.Describe())
	if url := firstMCPURL(integ); url != "" {
		fmt.Fprintf(out, "    Containers connect via: %s\n", url)
		fmt.Fprintf(out, "    claude-exec.sh switches transport on the next shell entry.\n")
	}
	fmt.Fprintln(out)
}

// indent prefixes each non-empty line of s with prefix. Used to render
// ExternalRuntime instruction blocks.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}

// ── workspace bootstrap ─────────────────────────────────────────────────

// runWorkspaceBootstrap runs the integration's WorkspaceDriver against
// the current workspace, persisting the result into the workspace .vb
// file under [prereqs.<id>]. Asks for confirmation when a cached key
// already exists.
func runWorkspaceBootstrap(cmd *cobra.Command, integ *integration.Integration, c colors) error {
	out := cmd.OutOrStdout()
	ctx := cmdContext(cmd)

	wsPath, projectName, pinPath, err := resolveWorkspace()
	if err != nil {
		fmt.Fprintf(out, "\n  %sNo workspace found.%s Run from a project directory to bootstrap a key.\n\n",
			c.dim, c.reset)
		return nil
	}

	fmt.Fprintf(out, "\n  Workspace:  %s\n", wsPath)
	fmt.Fprintf(out, "  Project:    %s\n", projectName)

	prereqID := integ.Workspace.PrereqID()
	cached, hasCached := loadCachedPrereq(pinPath, prereqID)
	if hasCached {
		fmt.Fprintf(out, "  Cached:     %s✓%s present", c.green, c.reset)
		if k := cached["api_key"]; k != "" {
			fmt.Fprintf(out, " (%s…)", k[:min(12, len(k))])
		}
		fmt.Fprintln(out)

		ok, err := confirm("A credential is already cached. Rotate it?",
			"Revokes the old credential and mints a fresh one.")
		if err != nil || !ok {
			return err
		}
		result, err := integ.Workspace.Rotate(ctx, integration.Workspace{
			Path: wsPath, ProjectName: projectName, Hostname: hostname(),
		}, cached)
		if err != nil {
			return fmt.Errorf("rotate: %w", err)
		}
		return persistAndReport(out, pinPath, prereqID, result, c)
	}

	ok, err := confirm("Bootstrap a credential for this workspace?",
		"Generates project-scoped credentials and stores them in .vb (gitignored).")
	if err != nil || !ok {
		fmt.Fprintf(out, "\n  Skipped — re-run later to mint a credential.\n\n")
		return err
	}

	result, err := integ.Workspace.Bootstrap(ctx, integration.Workspace{
		Path: wsPath, ProjectName: projectName, Hostname: hostname(),
	})
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	return persistAndReport(out, pinPath, prereqID, result, c)
}

// persistAndReport writes the bootstrap result into the .vb pin file
// and prints a per-key summary. .vb is added to .gitignore on first
// touch — we never store credentials in version control.
func persistAndReport(out io.Writer, pinPath, prereqID string, result map[string]string, c colors) error {
	if err := persistPrereqResult(pinPath, prereqID, result); err != nil {
		return fmt.Errorf("persist to %s: %w", pinPath, err)
	}
	changed, gErr := config.AppendToGitignore(filepath.Dir(pinPath))
	if gErr != nil {
		fmt.Fprintf(out, "  %swarning: could not update .gitignore: %v%s\n", c.yellow, gErr, c.reset)
	}

	for k, v := range result {
		display := v
		if k == "api_key" && len(v) > 12 {
			display = fmt.Sprintf("%s… (length %d)", v[:12], len(v))
		}
		fmt.Fprintf(out, "  %s✓%s %s = %s\n", c.green, c.reset, k, display)
	}
	fmt.Fprintf(out, "  %s✓%s cached in %s [prereqs.%s]\n", c.green, c.reset, pinPath, prereqID)
	if changed {
		fmt.Fprintf(out, "  %s✓%s .vb added to .gitignore\n", c.green, c.reset)
	}
	fmt.Fprintf(out, "\n  Next: %svibrate --rebuild%s to pick up the credential.\n\n", c.bold, c.reset)
	return nil
}

// confirm renders a single-step yes/no form. Returns (false, nil) on
// user-abort so the caller can treat it as "user wants to skip".
func confirm(title, desc string) (bool, error) {
	var v bool
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Description(desc).Value(&v),
	)).WithTheme(huh.ThemeCharm())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, err
	}
	return v, nil
}

// hostname returns os.Hostname() or "unknown" on error. Used to build
// the actor identifier some workspace drivers stamp into credentials.
func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}
