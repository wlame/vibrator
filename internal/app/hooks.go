package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wlame/vibrator/internal/config"
	"github.com/wlame/vibrator/internal/hooktools"
)

// runHookReadiness checks the host's Claude Code settings.json for hooks that
// shell out to a tool the resolved image won't install (e.g. node-based hooks
// under the minimal profile, which has no node feature). Each such hook fails
// on every event with a noisy "<tool>: not found".
//
// For each gap it either:
//
//   - interactive launch: prompts to install the backing feature (added to
//     pin.With so the next build bakes it) or to leave the hooks disabled —
//     the container guard in templates/scripts/entrypoint.sh skips them and
//     the choice is remembered ([hooks].acknowledged_missing) so we don't nag;
//   - non-interactive launch (CI, pipes): prints a one-line warning and
//     continues — the container guard handles it silently.
//
// Returns the (possibly updated) pin and whether it changed (so the caller can
// re-persist). It never blocks the launch: any error reading host state is
// treated as "nothing to check". Only claude-code uses ~/.claude hooks; other
// harnesses are a no-op here.
func runHookReadiness(pin config.Pin, opts Options) (config.Pin, bool) {
	if pin.Harness != "claude-code" {
		return pin, false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return pin, false
	}
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		return pin, false // no host settings → no hooks to check
	}
	commands := hooktools.Commands(data)
	if len(commands) == 0 {
		return pin, false
	}

	// Use the same resolution the build will use, so we reason about exactly
	// the features that get baked. Resolution errors surface (with a better
	// message) later in buildSpecs — don't duplicate them here.
	_, enabled, err := resolveExtensionsAndFeatures(pin, opts)
	if err != nil {
		return pin, false
	}

	gaps := hooktools.Scan(commands, enabled)
	if len(gaps) == 0 {
		return pin, false
	}

	acked := map[string]bool{}
	if pin.Hooks != nil {
		for _, f := range pin.Hooks.AcknowledgedMissing {
			acked[f] = true
		}
	}

	interactive := isStdinTTY(opts.Stdin)
	dirty := false

	for _, g := range gaps {
		if acked[g.Feature] {
			continue // already decided for this workspace
		}

		fmt.Fprintf(opts.Stderr, "\n  ⚠  [hooks] %d hook(s) call %s, installed by `%s` — not in this image\n",
			len(g.Commands), backtickList(g.Tools), g.Feature)
		fmt.Fprintf(opts.Stderr, "       e.g. %s\n", truncateRunes(g.Commands[0], 72))

		if !interactive {
			fmt.Fprintf(opts.Stderr,
				"       they'll be skipped in the container — add --with=%s to install instead\n", g.Feature)
			continue
		}

		fmt.Fprintf(opts.Stderr,
			"       The container will skip them. Install `%s` and rebuild? [y/N] ", g.Feature)
		var ans string
		fmt.Fscanln(opts.Stdin, &ans)

		if strings.EqualFold(strings.TrimSpace(ans), "y") {
			if !sliceHas(pin.With, g.Feature) {
				pin.With = append(append([]string{}, pin.With...), g.Feature)
			}
			// Drop any stale "acknowledged-as-skipped" marker for this feature.
			if pin.Hooks != nil {
				pin.Hooks.AcknowledgedMissing = sliceWithout(pin.Hooks.AcknowledgedMissing, g.Feature)
				if len(pin.Hooks.AcknowledgedMissing) == 0 {
					pin.Hooks = nil
				}
			}
			dirty = true
			fmt.Fprintf(opts.Stderr, "       ✓ added `%s` — it'll be installed on the next build\n", g.Feature)
		} else {
			if pin.Hooks == nil {
				pin.Hooks = &config.HookPrefs{}
			}
			if !sliceHas(pin.Hooks.AcknowledgedMissing, g.Feature) {
				pin.Hooks.AcknowledgedMissing = append(pin.Hooks.AcknowledgedMissing, g.Feature)
			}
			dirty = true
			fmt.Fprintln(opts.Stderr, "       ↷ keeping them disabled (won't ask again)")
		}
	}
	fmt.Fprintln(opts.Stderr)
	return pin, dirty
}

// backtickList renders ["node","npx"] as "`node`, `npx`".
func backtickList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = "`" + s + "`"
	}
	return strings.Join(q, ", ")
}

// truncateRunes shortens s to at most n runes, appending an ellipsis when cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func sliceHas(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func sliceWithout(ss []string, drop string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}
