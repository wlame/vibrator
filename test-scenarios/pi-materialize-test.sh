#!/bin/sh
# Verification gate for pi-materialize.sh. Requires `jq` on PATH. Optional
# end-to-end assertion runs when PI_BIN points at a pi binary (pi derives
# ~/.pi from HOME, so each case runs under a temp HOME; never rely on
# PI_CODING_AGENT_DIR — pi honors it only partially).
# Run: sh test-scenarios/pi-materialize-test.sh
set -u
SCRIPT_DIR=$(cd "$(dirname "$0")/.." && pwd)
MAT="$SCRIPT_DIR/templates/scripts/pi-materialize.sh"
fail=0
check() { if [ "$2" = "$3" ] || { [ "$1" = contains ] && printf '%s' "$2" | grep -qF "$3"; }; then echo "ok: $4"; else echo "FAIL: $4 (got: $2)"; fail=1; fi; }

make_baked() { # $1 = tree root (gets agent/ underneath)
    mkdir -p "$1/agent/providers" "$1/agent/themes" "$1/agent/bin"
    printf '%s' '{"mcpServers":{"context7":{"command":"npx","args":["-y","@upstash/context7-mcp"]},"shared-mcp":{"command":"baked-cmd"}}}' > "$1/agent/mcp.json"
    printf '%s' '{"packages":["npm:pi-mcp-adapter"]}' > "$1/agent/settings.json"
    printf '%s' '{"baseUrl":"https://api.anthropic.com"}' > "$1/agent/providers/anthropic.json"
    echo '{"name":"dracula"}' > "$1/agent/themes/dracula.json"
    echo 'baked-fd' > "$1/agent/bin/fd"
}
make_host() { # $1 = tree root
    mkdir -p "$1/agent/themes" "$1/agent/bin" "$1/agent/sessions"
    printf '%s' '{"mcpServers":{"user-thing":{"command":"user-cmd"},"shared-mcp":{"command":"host-cmd"}}}' > "$1/agent/mcp.json"
    printf '%s' '{"packages":["npm:host-pkg"],"defaultProvider":"openai"}' > "$1/agent/settings.json"
    echo '{"name":"host-theme"}' > "$1/agent/themes/host-theme.json"
    echo 'host-fd' > "$1/agent/bin/fd"
    printf '%s' '{"anthropic":{"apiKey":"host-secret"}}' > "$1/agent/auth.json"
    echo 'host session data' > "$1/agent/sessions/host-session.jsonl"
}
setup_world() { # $1 = temp root; container ~/.pi = baked image state + rw-mount stand-ins
    export HOME="$1/home"
    export VIBRATOR_PI_BAKED="$1/baked"
    export VIBRATOR_MANIFEST="$1/none.json"
    export VIBRATOR_HARNESS=pi
    mkdir -p "$HOME"
    echo '[]' > "$VIBRATOR_MANIFEST"
    make_baked "$VIBRATOR_PI_BAKED"
    make_baked "$HOME/.pi"
    echo 'container-fd' > "$HOME/.pi/agent/bin/fd"
    mkdir -p "$HOME/.pi/agent/sessions"
    printf '%s' '{"mounted":"host-auth"}' > "$HOME/.pi/agent/auth.json"
    echo 'mounted host session' > "$HOME/.pi/agent/sessions/live.jsonl"
}

# --- Case 1: seeded merge — host settings + baked artifacts coexist ----------
T=$(mktemp -d); setup_world "$T"
make_host "$HOME/.pi.host"
sh "$MAT"
MCP="$HOME/.pi/agent/mcp.json"; SET="$HOME/.pi/agent/settings.json"
check contains "$(cat "$MCP")" 'context7'   "baked MCP survives"
check contains "$(cat "$MCP")" 'user-thing' "host MCP present"
check exact "$(jq -r '.mcpServers["shared-mcp"].command' "$MCP")" 'host-cmd' "host wins per-key in mcp.json"
check exact "$(jq -r '.packages | sort | join(",")' "$SET")" 'npm:host-pkg,npm:pi-mcp-adapter' "settings packages are the union"
check exact "$(jq -r '.defaultProvider' "$SET")" 'openai' "host scalar setting preserved"
[ -f "$HOME/.pi/agent/providers/anthropic.json" ] && echo "ok: baked provider present" || { echo "FAIL: baked provider missing"; fail=1; }
[ -f "$HOME/.pi/agent/themes/host-theme.json" ] && echo "ok: host theme present" || { echo "FAIL: host theme missing"; fail=1; }
[ -f "$HOME/.pi/agent/themes/dracula.json" ] && echo "ok: baked theme present" || { echo "FAIL: baked theme missing"; fail=1; }

# --- Case 2: carve-outs — bin/auth/sessions never touched by the copies ------
check exact "$(cat "$HOME/.pi/agent/bin/fd")" 'container-fd' "container bin survives (host + baked bin excluded from host copy, container file untouched)"
check exact "$(jq -r '.mounted' "$HOME/.pi/agent/auth.json")" 'host-auth' "mounted auth.json not overwritten"
check exact "$(cat "$HOME/.pi/agent/sessions/live.jsonl")" 'mounted host session' "mounted sessions not overwritten"
check exact "$(jq -r '.anthropic.apiKey' "$HOME/.pi.host/agent/auth.json")" 'host-secret' "sidecar never written"

# --- Case 1e: e2e — a real pi lists the union-merged packages ----------------
if [ -n "${PI_BIN:-}" ] && [ -x "$PI_BIN" ]; then
    out=$("$PI_BIN" list 2>&1)
    check contains "$out" 'npm:pi-mcp-adapter' "e2e: pi lists baked package"
    check contains "$out" 'npm:host-pkg'       "e2e: pi lists host package"
else
    echo "warn: PI_BIN not set — skipping end-to-end pi list check"
fi

# --- Case 3: no sidecar -> tree untouched -------------------------------------
T3=$(mktemp -d); setup_world "$T3"
before=$(cat "$HOME/.pi/agent/mcp.json")
sh "$MAT"
check exact "$(cat "$HOME/.pi/agent/mcp.json")" "$before" "unseeded: baked mcp.json untouched"

# --- Case 4: manifest replay (synthetic), gated on seed ----------------------
T4=$(mktemp -d); setup_world "$T4"
make_host "$HOME/.pi.host"
export VIBRATOR_MANIFEST="$T4/manifest.json"
printf '%s' '[{"harness":"pi","mcp":{"name":"synth-int","http":{"url":"http://localhost:9090/mcp"}}}]' > "$VIBRATOR_MANIFEST"
VIBRATOR_INTEGRATION_MODE_SYNTH_INT=auto sh "$MAT"
check exact "$(jq -r '.mcpServers["synth-int"].url' "$HOME/.pi/agent/mcp.json")" 'http://localhost:9090/mcp' "manifest auto mode adds MCP"
VIBRATOR_INTEGRATION_MODE_SYNTH_INT=off sh "$MAT"
check exact "$(jq -r '.mcpServers | has("synth-int")' "$HOME/.pi/agent/mcp.json")" 'false' "manifest off mode removes MCP"
rm -rf "$HOME/.pi.host"
VIBRATOR_INTEGRATION_MODE_SYNTH_INT=auto sh "$MAT"
check exact "$(jq -r '.mcpServers | has("synth-int")' "$HOME/.pi/agent/mcp.json")" 'false' "manifest replay is gated on the seed"

# --- Case 5: idempotency — two runs, identical tree ---------------------------
T5=$(mktemp -d); setup_world "$T5"
make_host "$HOME/.pi.host"
tree_sum() { (cd "$HOME/.pi" && find . -type f | LC_ALL=C sort | while IFS= read -r f; do cat "$f"; done | cksum); }
sh "$MAT"
first=$(tree_sum)
sh "$MAT"
second=$(tree_sum)
check exact "$second" "$first" "two runs produce an identical tree"

[ "$fail" = 0 ] && echo "ALL PASS" || echo "FAILURES PRESENT"
exit "$fail"
