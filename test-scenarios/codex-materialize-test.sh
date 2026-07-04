#!/bin/sh
# Verification gate for codex-materialize.sh. Requires `codex` + `jq` on
# PATH. Run: sh test-scenarios/codex-materialize-test.sh
set -u
SCRIPT_DIR=$(cd "$(dirname "$0")/.." && pwd)
MAT="$SCRIPT_DIR/templates/scripts/codex-materialize.sh"
fail=0
check() { if [ "$2" = "$3" ] || { [ "$1" = contains ] && printf '%s' "$2" | grep -qF "$3"; }; then echo "ok: $4"; else echo "FAIL: $4 (got: $2)"; fail=1; fi; }

# --- Case 1: host sidecar + baked snapshot -> union in config.toml ---
T=$(mktemp -d); export CODEX_HOME="$T/.codex"; mkdir -p "$CODEX_HOME"
cat > "$CODEX_HOME/config.host.toml" <<'TOML'
model = "o3"
[mcp_servers.user-thing]
command = "user-cmd"
args = ["--flag"]
TOML
export VIBRATOR_CODEX_BAKED_MCP="$T/baked.json"
cat > "$VIBRATOR_CODEX_BAKED_MCP" <<'JSON'
[{"name":"baked-stdio","transport":{"type":"stdio","command":"npx","args":["-y","pkg"],"env":{}}},
 {"name":"baked-http","transport":{"type":"streamable_http","url":"https://example.com/mcp"}}]
JSON
export VIBRATOR_MANIFEST="$T/none.json"; echo '[]' > "$VIBRATOR_MANIFEST"
export VIBRATOR_HARNESS=codex
sh "$MAT"
got=$(cat "$CODEX_HOME/config.toml")
check contains "$got" 'model = "o3"'       "user model preserved"
check contains "$got" 'user-thing'          "user MCP preserved"
check contains "$got" 'baked-stdio'         "baked stdio MCP restored"
check contains "$got" 'baked-http'          "baked http MCP restored"

# --- Case 1b: a multi-arg + multi-env stdio server replays as separate argv
#     entries (regression guard for the set-- argv accumulation in
#     _vb_add_mcp — a naive unquoted-expansion approach would silently
#     collapse or misalign these). ---
T1B=$(mktemp -d); export CODEX_HOME="$T1B/.codex"; mkdir -p "$CODEX_HOME"
cat > "$CODEX_HOME/config.host.toml" <<'TOML'
model = "o3"
TOML
export VIBRATOR_CODEX_BAKED_MCP="$T1B/baked.json"
cat > "$VIBRATOR_CODEX_BAKED_MCP" <<'JSON'
[{"name":"baked-multi","transport":{"type":"stdio","command":"npx","args":["-y","pkg","--flag","value with space"],"env":{"FOO":"bar","BAZ":"qux value"}}}]
JSON
export VIBRATOR_MANIFEST="$T1B/none.json"; echo '[]' > "$VIBRATOR_MANIFEST"
export VIBRATOR_HARNESS=codex
sh "$MAT"
got1b=$(codex mcp list --json 2>/dev/null | jq -c '.[] | select(.name == "baked-multi") | .transport')
check contains "$got1b" '"command":"npx"'                    "multi-arg stdio: command correct"
check contains "$got1b" '"value with space"'                 "multi-arg stdio: multi-word arg intact as one entry"
check contains "$got1b" '"FOO":"bar"'                         "multi-arg stdio: first env var registered"
check contains "$got1b" '"BAZ":"qux value"'                   "multi-arg stdio: second env var (with space) registered"
argcount=$(printf '%s' "$got1b" | jq '.args | length')
check exact "$argcount" '4'                                   "multi-arg stdio: arg count is exactly 4 (no splitting/collapsing)"

# --- Case 2: no host sidecar -> baked config.toml left intact ---
T2=$(mktemp -d); export CODEX_HOME="$T2/.codex"; mkdir -p "$CODEX_HOME"
cat > "$CODEX_HOME/config.toml" <<'TOML'
[mcp_servers.prebaked]
command = "x"
TOML
rm -f "$CODEX_HOME/config.host.toml"
sh "$MAT"
check contains "$(cat "$CODEX_HOME/config.toml")" 'prebaked' "no-host: baked config left intact"

# --- Case 3: manifest mode=off removes an entry ---
T3=$(mktemp -d); export CODEX_HOME="$T3/.codex"; mkdir -p "$CODEX_HOME"
codex mcp add doomed -- some-cmd >/dev/null 2>&1
export VIBRATOR_MANIFEST="$T3/m.json"
echo '[{"harness":"codex","mcp":{"name":"doomed","http":{"url":"https://h/mcp"}}}]' > "$VIBRATOR_MANIFEST"
export VIBRATOR_INTEGRATION_MODE_DOOMED=off
export VIBRATOR_CODEX_BAKED_MCP="$T3/nobaked.json"; echo '[]' > "$VIBRATOR_CODEX_BAKED_MCP"
rm -f "$CODEX_HOME/config.host.toml"
sh "$MAT"
if grep -q 'doomed' "$CODEX_HOME/config.toml" 2>/dev/null; then echo "FAIL: mode=off did not remove"; fail=1; else echo "ok: mode=off removed the entry"; fi

# --- Case 4: missing jq or codex -> no-op, exits 0 (never blocks startup) ---
T4=$(mktemp -d); export CODEX_HOME="$T4/.codex"; mkdir -p "$CODEX_HOME"
FAKEBIN="$T4/fakebin"; mkdir -p "$FAKEBIN"
# PATH with no jq/codex at all. Invoke the interpreter by absolute path
# (/bin/sh) so trimming PATH doesn't also break finding `sh` itself.
( PATH="$FAKEBIN" /bin/sh "$MAT" )
check exact "$?" '0' "missing jq/codex: exits 0 without crashing"
check exact "$(ls -A "$CODEX_HOME" 2>/dev/null)" '' "missing jq/codex: no config written"

# --- Case 5: host custom agents merge into the baked agents dir --------------
T5=$(mktemp -d); export CODEX_HOME="$T5/.codex"
mkdir -p "$CODEX_HOME/agents" "$CODEX_HOME/agents.host"
printf 'name = "baked-only"\n' > "$CODEX_HOME/agents/baked-only.toml"
printf 'name = "shared"\nmodel = "baked"\n' > "$CODEX_HOME/agents/shared.toml"
printf 'name = "shared"\nmodel = "host"\n' > "$CODEX_HOME/agents.host/shared.toml"
printf 'name = "host-only"\n' > "$CODEX_HOME/agents.host/host-only.toml"
export VIBRATOR_CODEX_BAKED_MCP="$T5/baked.json"; echo '[]' > "$VIBRATOR_CODEX_BAKED_MCP"
export VIBRATOR_MANIFEST="$T5/manifest.json"; echo '[]' > "$VIBRATOR_MANIFEST"
sh "$MAT"
check contains "$(cat "$CODEX_HOME/agents/shared.toml")" 'model = "host"' "host wins on agent name collision"
[ -f "$CODEX_HOME/agents/baked-only.toml" ] && echo "ok: baked-only agent survives" || { echo "FAIL: baked-only agent missing"; fail=1; }
[ -f "$CODEX_HOME/agents/host-only.toml" ] && echo "ok: host-only agent copied in" || { echo "FAIL: host-only agent missing"; fail=1; }

# --- Case 5b: no host agents dir -> baked agents untouched -------------------
T5B=$(mktemp -d); export CODEX_HOME="$T5B/.codex"; mkdir -p "$CODEX_HOME/agents"
printf 'name = "keep"\n' > "$CODEX_HOME/agents/keep.toml"
export VIBRATOR_CODEX_BAKED_MCP="$T5B/baked.json"; echo '[]' > "$VIBRATOR_CODEX_BAKED_MCP"
export VIBRATOR_MANIFEST="$T5B/manifest.json"; echo '[]' > "$VIBRATOR_MANIFEST"
sh "$MAT"
check exact "$(ls "$CODEX_HOME/agents")" 'keep.toml' "no host agents dir: baked agents untouched"

exit $fail
