#!/bin/sh
# Verification gate for opencode-materialize.sh. Requires `jq` on PATH.
# Optional end-to-end assertion runs when OPENCODE_BIN is set or `opencode`
# is on PATH. Run: sh test-scenarios/opencode-materialize-test.sh
set -u
SCRIPT_DIR=$(cd "$(dirname "$0")/.." && pwd)
MAT="$SCRIPT_DIR/templates/scripts/opencode-materialize.sh"
fail=0
check() { if [ "$2" = "$3" ] || { [ "$1" = contains ] && printf '%s' "$2" | grep -qF "$3"; }; then echo "ok: $4"; else echo "FAIL: $4 (got: $2)"; fail=1; fi; }

# Build one fixture world per case: a fake baked snapshot + a fake host
# sidecar dir, both under a temp HOME/XDG_CONFIG_HOME so the materializer
# and (optionally) a real opencode binary resolve the same paths.
make_baked() { # $1 = dir
    mkdir -p "$1/agent" "$1/themes"
    printf '%s' '{"$schema":"https://opencode.ai/config.json","mcp":{"context7":{"type":"local","command":["npx","-y","@upstash/context7-mcp"],"enabled":true}},"username":"baked-user"}' > "$1/config.json"
    printf '%s' '{"$schema":"https://opencode.ai/tui.json","theme":"dracula"}' > "$1/tui.json"
    echo 'baked agent' > "$1/agent/refactor.md"
    echo '{"name":"dracula"}' > "$1/themes/dracula.json"
}
make_host() { # $1 = dir
    mkdir -p "$1/agent"
    printf '%s' '{"model":"anthropic/claude-x","mcp":{"user-thing":{"type":"local","command":["user-cmd"],"enabled":true}},"username":"host-user"}' > "$1/config.json"
    printf '%s' '{"theme":"host-theme"}' > "$1/tui.json"
    echo 'host agent' > "$1/agent/mine.md"
}

# --- Case 1: seeded merge — host settings + baked artifacts coexist ---------
T=$(mktemp -d)
export HOME="$T/home"; export XDG_CONFIG_HOME="$T/home/.config"
export VIBRATOR_OPENCODE_BAKED="$T/baked"; export VIBRATOR_MANIFEST="$T/none.json"
export VIBRATOR_HARNESS=opencode
mkdir -p "$XDG_CONFIG_HOME"; echo '[]' > "$VIBRATOR_MANIFEST"
make_baked "$VIBRATOR_OPENCODE_BAKED"
make_host "$XDG_CONFIG_HOME/opencode.host"
sh "$MAT"
CFG="$XDG_CONFIG_HOME/opencode/config.json"
got=$(cat "$CFG")
check contains "$got" '"model":"anthropic/claude-x"' "user model preserved"
check contains "$got" 'user-thing'                    "user MCP preserved"
check contains "$got" 'context7'                      "baked MCP restored"
check exact "$(jq -r '.username' "$CFG")" 'host-user' "host wins on colliding key"
check exact "$(jq -r '.theme' "$XDG_CONFIG_HOME/opencode/tui.json")" 'host-theme' "tui.json merged, host theme wins"
[ -f "$XDG_CONFIG_HOME/opencode/agent/refactor.md" ] && echo "ok: baked agent present" || { echo "FAIL: baked agent missing"; fail=1; }
[ -f "$XDG_CONFIG_HOME/opencode/agent/mine.md" ] && echo "ok: host agent present" || { echo "FAIL: host agent missing"; fail=1; }
[ -f "$XDG_CONFIG_HOME/opencode/themes/dracula.json" ] && echo "ok: baked theme file present" || { echo "FAIL: baked theme file missing"; fail=1; }

# --- Case 1e: end-to-end — a real opencode resolves the merged MCPs ---------
OC="${OPENCODE_BIN:-$(command -v opencode || true)}"
if [ -n "$OC" ] && [ -x "$OC" ]; then
    resolved=$("$OC" debug config 2>/dev/null)
    check contains "$resolved" 'context7'   "e2e: opencode resolves baked MCP"
    check contains "$resolved" 'user-thing' "e2e: opencode resolves user MCP"
else
    echo "warn: no opencode binary (set OPENCODE_BIN) — skipping end-to-end resolution check"
fi

# --- Case 2: no host sidecar -> baked dir untouched --------------------------
T2=$(mktemp -d)
export HOME="$T2/home"; export XDG_CONFIG_HOME="$T2/home/.config"
export VIBRATOR_OPENCODE_BAKED="$T2/baked"; export VIBRATOR_MANIFEST="$T2/none.json"
mkdir -p "$XDG_CONFIG_HOME"; echo '[]' > "$VIBRATOR_MANIFEST"
make_baked "$VIBRATOR_OPENCODE_BAKED"
make_baked "$XDG_CONFIG_HOME/opencode"   # image state == snapshot
before=$(cat "$XDG_CONFIG_HOME/opencode/config.json")
sh "$MAT"
check exact "$(cat "$XDG_CONFIG_HOME/opencode/config.json")" "$before" "unseeded: baked config untouched"

# --- Case 3: manifest replay (synthetic entry), gated on seed ----------------
T3=$(mktemp -d)
export HOME="$T3/home"; export XDG_CONFIG_HOME="$T3/home/.config"
export VIBRATOR_OPENCODE_BAKED="$T3/baked"; export VIBRATOR_MANIFEST="$T3/manifest.json"
mkdir -p "$XDG_CONFIG_HOME"
make_baked "$VIBRATOR_OPENCODE_BAKED"
make_host "$XDG_CONFIG_HOME/opencode.host"
printf '%s' '[{"harness":"opencode","mcp":{"name":"synth-int","http":{"url":"http://localhost:9090/mcp"}}}]' > "$VIBRATOR_MANIFEST"
VIBRATOR_INTEGRATION_MODE_SYNTH_INT=auto sh "$MAT"
check exact "$(jq -r '.mcp["synth-int"].url' "$XDG_CONFIG_HOME/opencode/config.json")" 'http://localhost:9090/mcp' "manifest auto mode adds remote MCP"
VIBRATOR_INTEGRATION_MODE_SYNTH_INT=off sh "$MAT"
check exact "$(jq -r '.mcp | has("synth-int")' "$XDG_CONFIG_HOME/opencode/config.json")" 'false' "manifest off mode removes MCP"
# Unseeded: same manifest, no sidecar -> config must not gain the entry.
rm -rf "$XDG_CONFIG_HOME/opencode.host"
make_baked "$XDG_CONFIG_HOME/opencode"
VIBRATOR_INTEGRATION_MODE_SYNTH_INT=auto sh "$MAT"
check exact "$(jq -r '.mcp | has("synth-int")' "$XDG_CONFIG_HOME/opencode/config.json")" 'false' "manifest replay is gated on the seed"

# --- Case 4: idempotency — two runs, identical tree --------------------------
T4=$(mktemp -d)
export HOME="$T4/home"; export XDG_CONFIG_HOME="$T4/home/.config"
export VIBRATOR_OPENCODE_BAKED="$T4/baked"; export VIBRATOR_MANIFEST="$T4/none.json"
mkdir -p "$XDG_CONFIG_HOME"; echo '[]' > "$VIBRATOR_MANIFEST"
make_baked "$VIBRATOR_OPENCODE_BAKED"
make_host "$XDG_CONFIG_HOME/opencode.host"
tree_sum() { (cd "$XDG_CONFIG_HOME/opencode" && find . -type f | LC_ALL=C sort | while IFS= read -r f; do cat "$f"; done | cksum); }
sh "$MAT"
first=$(tree_sum)
sh "$MAT"
second=$(tree_sum)
check exact "$second" "$first" "two runs produce an identical tree"

[ "$fail" = 0 ] && echo "ALL PASS" || echo "FAILURES PRESENT"
exit "$fail"
