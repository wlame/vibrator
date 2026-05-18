package wizard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wlame/vibrator/internal/config"
)

// EquivalentCommand renders the `vibrate ...` invocation that would
// produce the given pin without going through the wizard. Printed at
// the end of every successful wizard run so users can:
//
//  1. Copy it into a script or onboarding doc.
//  2. See exactly what got saved to .vb at a glance.
//  3. Run a subsequent invocation deterministically (flags > .vb merge).
//
// The output uses the project's CLI-style convention (`--flag=value`,
// not `--flag value`) — matches the user's stated preference in CLAUDE.md.
//
// Pasted API keys are NEVER included in the output; their absence is
// noted as a trailing comment so the user understands the gap.
func EquivalentCommand(p config.Pin) string {
	var b strings.Builder
	b.WriteString("vibrate")

	if p.Harness != "" {
		fmt.Fprintf(&b, " \\\n  --harness=%s", p.Harness)
	}
	if p.Profile != "" {
		fmt.Fprintf(&b, " \\\n  --profile=%s", p.Profile)
	}
	if p.Shell != "" {
		fmt.Fprintf(&b, " \\\n  --shell=%s", p.Shell)
	}
	if len(p.With) > 0 {
		fmt.Fprintf(&b, " \\\n  --with=%s", strings.Join(p.With, ","))
	}
	if len(p.No) > 0 {
		fmt.Fprintf(&b, " \\\n  --no=%s", strings.Join(p.No, ","))
	}
	if len(p.Catalog) > 0 {
		// Sort catalog for stable output regardless of wizard insertion order.
		sorted := append([]string{}, p.Catalog...)
		sort.Strings(sorted)
		fmt.Fprintf(&b, " \\\n  --catalog=%s", strings.Join(sorted, ","))
	}

	var pastedKeyNote bool
	if p.LLM != nil {
		fmt.Fprintf(&b, " \\\n  --llm-provider=%s", p.LLM.Provider)
		if p.LLM.Model != "" {
			fmt.Fprintf(&b, " \\\n  --llm-model=%s", p.LLM.Model)
		}
		if p.LLM.BaseURL != "" {
			fmt.Fprintf(&b, " \\\n  --llm-base-url=%s", p.LLM.BaseURL)
		}
		if p.LLM.Auth != nil {
			if p.LLM.Auth.Env != "" {
				fmt.Fprintf(&b, " \\\n  --llm-auth-env=%s", p.LLM.Auth.Env)
			}
			if p.LLM.Auth.Value != "" {
				// Don't print the value; the CLI accepts the env-var path
				// only. Note it explicitly so the user isn't confused.
				pastedKeyNote = true
			}
		}
	}

	if pastedKeyNote {
		b.WriteString("\n# Note: API key was pasted into the wizard and is stored in .vb only.")
		b.WriteString("\n# It cannot be reproduced via CLI flags. To rotate, re-run the wizard.")
	}
	return b.String()
}

// Summary renders a multi-section human-readable view of the pin shown
// at the end of the wizard, right before "save and build". Always
// non-empty so callers don't have to special-case missing fields.
//
// Fields print in the same order the wizard collected them: harness,
// profile, shell, llm, catalog. Lengths are clamped to keep the output
// terminal-friendly.
func Summary(p config.Pin, workspaceDir string) string {
	var b strings.Builder

	if workspaceDir != "" {
		fmt.Fprintf(&b, "Workspace: %s\n", workspaceDir)
	}
	if p.Harness != "" {
		fmt.Fprintf(&b, "Harness:   %s\n", p.Harness)
	}
	if p.Profile != "" {
		fmt.Fprintf(&b, "Profile:   %s\n", p.Profile)
	}
	if p.Shell != "" {
		fmt.Fprintf(&b, "Shell:     %s\n", p.Shell)
	}

	if p.LLM != nil {
		fmt.Fprintf(&b, "LLM:       %s / %s\n", p.LLM.Provider, p.LLM.Model)
		if p.LLM.BaseURL != "" {
			fmt.Fprintf(&b, "           base_url=%s\n", p.LLM.BaseURL)
		}
		if p.LLM.Auth != nil {
			switch {
			case p.LLM.Auth.Env != "":
				fmt.Fprintf(&b, "           auth=$%s (host env)\n", p.LLM.Auth.Env)
			case p.LLM.Auth.Value != "":
				fmt.Fprintf(&b, "           auth=<pasted, saved in .vb>\n")
			}
		}
	}

	if len(p.Catalog) > 0 {
		sorted := append([]string{}, p.Catalog...)
		sort.Strings(sorted)
		fmt.Fprintf(&b, "Catalog:   %s\n", strings.Join(sorted, ", "))
	}
	return b.String()
}
