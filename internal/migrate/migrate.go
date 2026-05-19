// Package migrate converts legacy bash-era `.vb.env` files (key=value
// dotenv) into the new Go-era `.vb` (TOML) format.
//
// The bash version of vibrator used a shell-sourced dotenv file at the
// workspace root with values like:
//
//	PROFILE=full
//	NO=aider
//	HARNESS=claude-code
//	CLAUDE_MEM_SERVER_BETA_API_KEY=cmem_...
//
// The Go version uses a TOML file with a richer structure (subtables
// for [prereqs.*] and [llm.*]). This package handles the one-shot
// conversion users run when migrating their existing workspaces.
//
// The migration is deliberately conservative:
//
//   - Unknown keys are preserved under the new pin's [env] section
//     (so user-set OPENAI_API_KEY etc. carries over).
//   - Quoted values are unquoted but otherwise pass through verbatim.
//   - Comments and blank lines are dropped — TOML doesn't preserve
//     them anyway, and the new pin's auto-generated header is more
//     informative.
package migrate

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/wlame/vibrator/internal/config"
)

// ParseDotenv reads `r` as a shell-sourced dotenv stream and returns
// the key/value pairs in the order they appeared. Blank lines and
// comments are skipped. Lines without `=` are skipped (treated as a
// soft parse error — we don't want to abort migration over one
// malformed line).
//
// Quoting: surrounding single OR double quotes are stripped if both
// sides match. Embedded escapes (\n, \t) are NOT expanded — this is
// shell-sourcing semantics, not C escapes; bash itself doesn't expand
// these inside single quotes either.
func ParseDotenv(r io.Reader) ([]KV, error) {
	var out []KV
	scanner := bufio.NewScanner(r)
	// Bump the buffer in case a workspace has an unusually long line
	// (e.g., a base64-encoded blob in an env value).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional `export ` prefix; bash users sometimes write it.
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		v = unquote(v)
		out = append(out, KV{Key: k, Value: v})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dotenv: %w", err)
	}
	return out, nil
}

// KV is one parsed key/value pair, preserving its source order.
type KV struct {
	Key, Value string
}

// unquote strips matching single OR double quotes from v.
func unquote(v string) string {
	if len(v) < 2 {
		return v
	}
	first, last := v[0], v[len(v)-1]
	if first == last && (first == '"' || first == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

// ToPin maps a parsed dotenv stream to a config.Pin. The mapping is
// conservative — every legacy key has either a structured destination
// (scalar/list/prereqs) or a fallback ([env]) so nothing is silently
// dropped.
//
// Mapping:
//
//	HARNESS                                  → pin.Harness
//	PROFILE                                  → pin.Profile
//	SHELL                                    → pin.Shell
//	WITH                                     → pin.With (comma-split)
//	NO                                       → pin.No (comma-split)
//	CATALOG                                  → pin.Extensions (comma-split)
//	USERNAME                                 → ignored (now a build-time arg)
//	CLAUDE_MEM_SERVER_BETA_API_KEY           → prereqs.claude-mem-server-beta.api_key
//	CLAUDE_MEM_SERVER_BETA_TEAM_ID           → prereqs.claude-mem-server-beta.team_id
//	CLAUDE_MEM_SERVER_BETA_PROJECT_ID        → prereqs.claude-mem-server-beta.project_id
//	(anything else)                           → pin.Env[KEY] = VALUE
//
// The Notes return value is a human-readable log of what happened, one
// line per dropped/migrated key — surfaced by the CLI's --dry-run mode.
func ToPin(items []KV) (config.Pin, []string) {
	pin := config.Pin{}
	var notes []string

	for _, kv := range items {
		switch kv.Key {
		case "HARNESS":
			pin.Harness = kv.Value
			notes = append(notes, fmt.Sprintf("HARNESS → harness = %q", kv.Value))
		case "PROFILE":
			pin.Profile = kv.Value
			notes = append(notes, fmt.Sprintf("PROFILE → profile = %q", kv.Value))
		case "SHELL":
			pin.Shell = kv.Value
			notes = append(notes, fmt.Sprintf("SHELL → shell = %q", kv.Value))
		case "WITH":
			pin.With = splitCommaList(kv.Value)
			notes = append(notes, fmt.Sprintf("WITH → with = %v", pin.With))
		case "NO":
			pin.No = splitCommaList(kv.Value)
			notes = append(notes, fmt.Sprintf("NO → no = %v", pin.No))
		case "CATALOG":
			pin.Extensions = splitCommaList(kv.Value)
			notes = append(notes, fmt.Sprintf("CATALOG → extensions = %v", pin.Extensions))
		case "USERNAME":
			notes = append(notes, "USERNAME → (ignored — now a build-time arg, set via --username flag)")
		case "CLAUDE_MEM_SERVER_BETA_API_KEY":
			setPrereq(&pin, "claude-mem-server-beta", "api_key", kv.Value)
			notes = append(notes, "CLAUDE_MEM_SERVER_BETA_API_KEY → [prereqs.claude-mem-server-beta].api_key")
		case "CLAUDE_MEM_SERVER_BETA_TEAM_ID":
			setPrereq(&pin, "claude-mem-server-beta", "team_id", kv.Value)
			notes = append(notes, "CLAUDE_MEM_SERVER_BETA_TEAM_ID → [prereqs.claude-mem-server-beta].team_id")
		case "CLAUDE_MEM_SERVER_BETA_PROJECT_ID":
			setPrereq(&pin, "claude-mem-server-beta", "project_id", kv.Value)
			notes = append(notes, "CLAUDE_MEM_SERVER_BETA_PROJECT_ID → [prereqs.claude-mem-server-beta].project_id")
		default:
			// Fallback: stash under [env] so nothing is silently lost.
			if pin.Env == nil {
				pin.Env = map[string]string{}
			}
			pin.Env[kv.Key] = kv.Value
			notes = append(notes, fmt.Sprintf("%s → [env].%s (unrecognized — preserved verbatim)", kv.Key, kv.Key))
		}
	}

	return pin, notes
}

// setPrereq is a tiny helper to lazily initialize pin.Prereqs[id] before
// inserting into it.
func setPrereq(pin *config.Pin, id, key, value string) {
	if pin.Prereqs == nil {
		pin.Prereqs = map[string]map[string]string{}
	}
	if pin.Prereqs[id] == nil {
		pin.Prereqs[id] = map[string]string{}
	}
	pin.Prereqs[id][key] = value
}

// splitCommaList parses a comma-separated value into a trimmed,
// non-empty slice. Tolerates extra whitespace between entries.
func splitCommaList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
