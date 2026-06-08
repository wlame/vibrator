package hooktools

import (
	"reflect"
	"testing"

	"github.com/wlame/vibrator/internal/feature"
)

func TestCommands(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{
			name: "extracts commands across events in sorted order",
			json: `{
				"hooks": {
					"PreToolUse": [
						{"matcher": "Write|Edit", "hooks": [{"type": "command", "command": "node fmt.js"}]}
					],
					"PostToolUse": [
						{"matcher": "Write", "hooks": [
							{"type": "command", "command": "python lint.py"},
							{"type": "command", "command": "gofmt -w ."}
						]}
					]
				}
			}`,
			// PostToolUse sorts before PreToolUse.
			want: []string{"python lint.py", "gofmt -w .", "node fmt.js"},
		},
		{
			name: "no hooks key",
			json: `{"model": "x"}`,
			want: nil,
		},
		{
			name: "empty commands skipped",
			json: `{"hooks":{"Stop":[{"hooks":[{"command":""},{"command":"  "}]}]}}`,
			want: nil,
		},
		{
			name: "malformed json returns nil",
			json: `{not json`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Commands([]byte(tt.json))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Commands() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScan(t *testing.T) {
	tests := []struct {
		name     string
		commands []string
		enabled  []string
		want     []Gap
	}{
		{
			name:     "node missing",
			commands: []string{"node /home/u/.claude/hooks/fmt.js", "npx prettier --write"},
			enabled:  nil,
			want: []Gap{{
				Feature:  "node",
				Tools:    []string{"node", "npx"},
				Commands: []string{"node /home/u/.claude/hooks/fmt.js", "npx prettier --write"},
			}},
		},
		{
			name:     "node present → no gap",
			commands: []string{"node fmt.js"},
			enabled:  []string{"node"},
			want:     []Gap{},
		},
		{
			name:     "multiple missing features sorted",
			commands: []string{"python a.py", "node b.js"},
			enabled:  nil,
			want: []Gap{
				{Feature: "node", Tools: []string{"node"}, Commands: []string{"node b.js"}},
				{Feature: "python", Tools: []string{"python"}, Commands: []string{"python a.py"}},
			},
		},
		{
			name:     "uv and python3 both map to python feature",
			commands: []string{"uv run x", "python3 y.py"},
			enabled:  nil,
			want: []Gap{{
				Feature:  "python",
				Tools:    []string{"python3", "uv"},
				Commands: []string{"uv run x", "python3 y.py"},
			}},
		},
		{
			name:     "word boundary: nodemon does not match node",
			commands: []string{"nodemon watch"},
			enabled:  nil,
			want:     []Gap{},
		},
		{
			name:     "absolute path still matches",
			commands: []string{"/usr/local/bin/node fmt.js"},
			enabled:  nil,
			want: []Gap{{
				Feature:  "node",
				Tools:    []string{"node"},
				Commands: []string{"/usr/local/bin/node fmt.js"},
			}},
		},
		{
			name:     "base-toolkit tools are not tracked",
			commands: []string{"jq . settings.json", "rg TODO", "git status"},
			enabled:  nil,
			want:     []Gap{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scan(tt.commands, tt.enabled)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Scan() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestToolFeatureValuesAreKnownFeatures guards against the tool→feature map
// referencing a feature ID that doesn't exist in internal/feature.
func TestToolFeatureValuesAreKnownFeatures(t *testing.T) {
	for tool, feat := range toolFeature {
		if !feature.IsKnown(feat) {
			t.Errorf("tool %q maps to unknown feature %q", tool, feat)
		}
	}
}
