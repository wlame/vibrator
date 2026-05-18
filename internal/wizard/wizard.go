// Package wizard runs the interactive setup flow that fills in
// the pieces of a workspace pin (.vb) the user hasn't supplied via CLI
// flags.
//
// # Adaptive gating
//
// The wizard never asks for a value the caller already provided. Each
// step is shown only when its corresponding Pin field is unset. Users
// who pass `--harness=claude-code --profile=full --shell=zsh` see only
// the catalog selection step; users who pass nothing see the full
// flow.
//
// # Form library
//
// We use charmbracelet/huh (v1.0.0). huh forms are composed of Groups;
// each Group is one screen, conditionally hidden via WithHideFunc.
// Within a Group, multiple inputs share a screen.
//
// # Testing strategy
//
// huh forms are interactive TUI surfaces; programmatic interaction
// requires a PTY emulator we don't want to depend on. We split the
// wizard into:
//
//   - Pure helpers (summary.go, this file's gating logic) — unit-tested
//     against fixtures.
//   - Form composition (llm.go, catalog.go) — smoke-tested via the binary.
package wizard

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/huh"

	"github.com/wlame/vibrator/internal/catalog"
	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/harness"
	"github.com/wlame/vibrator/internal/hostprobe"
)

// Input bundles everything the wizard needs to run. All fields are
// optional except Initial; Run handles nil gracefully.
type Input struct {
	// Initial is the partially-populated pin (typically: CLI flags
	// folded with any existing .vb). The wizard fills only the gaps.
	Initial config.Pin

	// WorkspaceDir is the absolute path to the project root, used for
	// the "Workspace:" line in the summary. May be empty.
	WorkspaceDir string

	// HostDetected is the result of hostprobe.ProbeAll, used to pre-check
	// catalog entries that the user already has on the host.
	HostDetected map[string]hostprobe.Detected

	// CatalogEntries is the loaded catalog (typically from
	// catalog.LoadAll). The wizard uses it for the selection step.
	CatalogEntries map[string]*catalog.Entry
}

// Result is what Run returns when the wizard completes successfully.
type Result struct {
	// Pin is the fully-populated pin, ready to save to .vb.
	Pin config.Pin

	// Cancelled is true when the user hit Esc / Ctrl-C during the
	// wizard. Callers should treat this as "user backed out" and
	// neither save .vb nor proceed with build.
	Cancelled bool
}

// Steps tracks which wizard steps need to be shown for a given input.
// Exposed for unit-testing the gating decisions independently from the
// huh form layer.
type Steps struct {
	Harness bool
	Profile bool
	Shell   bool
	LLM     bool
	Catalog bool
}

// PlanSteps computes which steps the wizard would show given an input.
// Pure function; no huh, no I/O. The wizard layer consumes this to
// decide which Groups to include.
//
// The LLM step depends on the chosen harness — it's planned only when
// (a) harness is known and (b) the harness supports LLM provider
// selection. If harness is itself a wizard step, LLM planning is
// deferred until after harness selection (handled at runtime).
func PlanSteps(in Input) Steps {
	var s Steps

	s.Harness = in.Initial.Harness == ""
	s.Profile = in.Initial.Profile == ""
	s.Shell = in.Initial.Shell == ""
	s.Catalog = true // always show catalog step — empty selection is valid

	if !s.Harness {
		// Harness is known; we can decide LLM planning now.
		if h, ok := harness.ByID(in.Initial.Harness); ok && h.SupportsLLMProvider() && in.Initial.LLM == nil {
			s.LLM = true
		}
	} else {
		// Harness not yet chosen — LLM step will be decided after
		// harness selection. We mark s.LLM = true here; the runtime
		// form-builder re-checks SupportsLLMProvider once the user
		// picks a harness, hiding the LLM Group if necessary.
		s.LLM = in.Initial.LLM == nil
	}
	return s
}

// Run launches the wizard for the supplied input. Returns Result.Pin
// containing all fields populated (Initial + wizard answers).
//
// Returns Result{Cancelled: true} when the user aborts via Esc/Ctrl-C;
// any other error is a genuine failure (TTY missing, form configuration
// bug, etc.).
//
// The signature is built around huh.NewForm, which requires us to
// pre-allocate the value bindings; values flow back into Pin only after
// the form runs to completion.
func Run(ctx context.Context, in Input) (Result, error) {
	steps := PlanSteps(in)

	// Working copies of the pin's fields, used as huh value bindings.
	// Pre-populate from Initial so the form opens on the right default
	// (e.g., a step the user already partly filled).
	pin := in.Initial

	// Build the form groups based on which steps are needed. Each
	// builder may return scratch bindings that we commit into `pin`
	// after the form completes.
	var groups []*huh.Group
	groups = append(groups, buildCoreGroups(&pin, steps)...)

	var llmBinds *llmBindings
	if steps.LLM {
		llmGroups, b := buildLLMGroups(&pin)
		groups = append(groups, llmGroups...)
		llmBinds = b
	}

	var catalogBinds *kindBindings
	if steps.Catalog && len(in.CatalogEntries) > 0 {
		catGroups, b := buildCatalogGroups(&pin, in.CatalogEntries, in.HostDetected)
		groups = append(groups, catGroups...)
		catalogBinds = b
	}

	if len(groups) == 0 {
		// Everything was pre-supplied; nothing to ask.
		return Result{Pin: pin}, nil
	}

	form := huh.NewForm(groups...).WithTheme(huh.ThemeCharm())
	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return Result{Pin: pin, Cancelled: true}, nil
		}
		return Result{}, fmt.Errorf("wizard form: %w", err)
	}

	// Post-form commit: fold scratch bindings into `pin`. Done outside
	// huh because huh.Group has no validate hook in v1.0.0.
	if llmBinds != nil {
		commitLLM(&pin, llmBinds)
		// If the wizard ran the LLM step but the chosen harness ended up
		// being one that doesn't actually support an LLM provider step
		// (e.g., user switched to claude-code mid-flow), drop the LLM
		// section so .vb stays clean.
		if !harnessSupportsLLM(pin.Harness) {
			pin.LLM = nil
		}
	}
	if catalogBinds != nil {
		pin.Catalog = catalogBinds.flatten()
	}

	return Result{Pin: pin}, nil
}

// buildCoreGroups assembles the harness/profile/shell groups based on
// the Steps plan. Each Group is hidden when the corresponding field is
// already set in Initial.
//
// We could write these as one big function, but splitting out
// per-concern keeps each builder focused on one thing.
func buildCoreGroups(pin *config.Pin, steps Steps) []*huh.Group {
	var groups []*huh.Group

	if steps.Harness {
		// Build harness options from the registry so the wizard stays in
		// sync with whatever harnesses are linked into this build.
		opts := make([]huh.Option[string], 0, len(harness.Registry))
		for _, h := range harness.Registry {
			opts = append(opts, huh.NewOption(h.Name()+" ("+h.ID()+")", h.ID()))
		}
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().
				Title("Agent harness").
				Description("Which AI coding harness will run in the container?").
				Options(opts...).
				Value(&pin.Harness),
		))
	}

	if steps.Profile {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().
				Title("Base profile").
				Description("Bundle of always-on features for the image.").
				Options(
					huh.NewOption("minimal — bare CLI, ~600 MB", "minimal"),
					huh.NewOption("backend — Python + Go + Postgres tools", "backend"),
					huh.NewOption("frontend — Node + Playwright + browser bins", "frontend"),
					huh.NewOption("full — everything (default)", "full"),
				).
				Value(&pin.Profile),
		))
	}

	if steps.Shell {
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().
				Title("Default shell").
				Description("Shell the container's user lands in by default.").
				Options(
					huh.NewOption("zsh (recommended)", "zsh"),
					huh.NewOption("bash", "bash"),
					huh.NewOption("fish", "fish"),
				).
				Value(&pin.Shell),
		))
	}

	return groups
}
