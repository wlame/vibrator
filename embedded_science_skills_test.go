package vibrator_test

// Contract tests for the Google DeepMind "Science Skills" extension family.
//
// science-skills ships as one entry per harness (claude-code, codex, opencode),
// each copying the same pinned upstream bundle into that harness's skill dir.
// These tests pin the contract so an accidental edit — a drifted SHA, a wrong
// kind, a missing python dep — fails here rather than at docker-build time.

import (
	"regexp"
	"strings"
	"testing"

	vibrator "github.com/wlame/vibrator"
	"github.com/wlame/vibrator/internal/extensions"
)

var sciRefLine = regexp.MustCompile(`(?m)^\s*SCI_REF=([0-9a-f]{40})\s*$`)

// sciSkillDir maps each harness to the skill dir its science-skills entry must
// copy into. Mirrors the paths ECC's harness-aware installer uses.
var sciSkillDir = map[string]string{
	"claude-code": "$HOME/.claude/skills",
	"codex":       "$HOME/.codex/skills",
	"opencode":    "$HOME/.opencode/skills",
}

func scienceSkillsEntries(t *testing.T) []*extensions.Entry {
	t.Helper()
	all, err := extensions.LoadAll(vibrator.ExtensionsFS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	var out []*extensions.Entry
	for _, e := range all {
		if e.ID == "science-skills" {
			out = append(out, e)
		}
	}
	return out
}

// TestScienceSkills_PresentForSupportedHarnesses asserts the bundle exists for
// each harness that consumes SKILL.md skills (pi is intentionally excluded).
func TestScienceSkills_PresentForSupportedHarnesses(t *testing.T) {
	got := map[string]bool{}
	for _, e := range scienceSkillsEntries(t) {
		got[e.Harness] = true
	}
	for h := range sciSkillDir {
		if !got[h] {
			t.Errorf("missing science-skills entry for harness %q", h)
		}
	}
}

// TestScienceSkills_InstallContract checks each entry is a skill that copies the
// bundle into the harness-correct skill dir and declares the python dep (which
// provides uv, the scripts' runtime).
func TestScienceSkills_InstallContract(t *testing.T) {
	for _, e := range scienceSkillsEntries(t) {
		if e.Kind != extensions.KindSkill {
			t.Errorf("%s: kind = %q, want skill", e.Key(), e.Kind)
		}

		dir, ok := sciSkillDir[e.Harness]
		if !ok {
			t.Errorf("%s: unexpected harness for science-skills", e.Key())
			continue
		}
		if !strings.Contains(e.Install, dir) {
			t.Errorf("%s: install does not target skill dir %q", e.Key(), dir)
		}

		var hasPython bool
		for _, f := range e.Deps.Features {
			if f == "python" {
				hasPython = true
			}
		}
		if !hasPython {
			t.Errorf("%s: missing python feature dep (scripts run on uv/python)", e.Key())
		}
	}
}

// TestScienceSkills_PinnedRefIsUniform guards bumping: every entry must pin the
// same upstream commit, so a bump is a single uniform edit.
func TestScienceSkills_PinnedRefIsUniform(t *testing.T) {
	refs := map[string][]string{}
	for _, e := range scienceSkillsEntries(t) {
		m := sciRefLine.FindStringSubmatch(e.Install)
		if m == nil {
			t.Errorf("%s: install has no `SCI_REF=<40-hex-sha>` line", e.Key())
			continue
		}
		refs[m[1]] = append(refs[m[1]], e.Key())
	}
	if len(refs) > 1 {
		t.Errorf("science-skills entries pin %d different commits (must be uniform): %v", len(refs), refs)
	}
}
