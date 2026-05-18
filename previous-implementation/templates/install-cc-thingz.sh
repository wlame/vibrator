#!/bin/sh
# Install cc-thingz plugins from https://github.com/umputun/cc-thingz
set -e

MKT="umputun-cc-thingz"
MKT_DIR="$HOME/.claude/plugins/marketplaces/$MKT"
CACHE_DIR="$HOME/.claude/plugins/cache/$MKT"
INST="$HOME/.claude/plugins/installed_plugins.json"
SETT="$HOME/.claude/settings.json"
MARKETS="$HOME/.claude/plugins/known_marketplaces.json"
PLUGINS="brainstorm review planning release-tools thinking-tools skill-eval workflow"

mkdir -p "$(dirname "$MKT_DIR")"
git clone --depth 1 https://github.com/umputun/cc-thingz.git "$MKT_DIR"
GIT_SHA=$(cd "$MKT_DIR" && git rev-parse HEAD)
GIT_SHORT=$(echo "$GIT_SHA" | cut -c1-12)
TS=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)

mkdir -p "$CACHE_DIR"

# Build JSON entries for all plugins
NEW_PLUGINS='{}'
NEW_ENABLED='{}'
for p in $PLUGINS; do
    dest="$CACHE_DIR/$p/$GIT_SHORT"
    mkdir -p "$dest"
    cp -r "$MKT_DIR/plugins/$p/." "$dest/"
    entry="${p}@${MKT}"
    NEW_PLUGINS=$(printf '%s' "$NEW_PLUGINS" | jq \
        --arg k "$entry" --arg d "$dest" --arg v "$GIT_SHORT" --arg t "$TS" --arg s "$GIT_SHA" \
        '.[$k] = [{"scope":"user","installPath":$d,"version":$v,"installedAt":$t,"lastUpdated":$t,"gitCommitSha":$s}]')
    NEW_ENABLED=$(printf '%s' "$NEW_ENABLED" | jq --arg k "$entry" '.[$k] = true')
    echo "  Installed: $entry"
done

# Create or merge installed_plugins.json
mkdir -p "$(dirname "$INST")"
if [ -f "$INST" ]; then
    jq --argjson p "$NEW_PLUGINS" '.plugins += $p' "$INST" > "$INST.tmp" && mv "$INST.tmp" "$INST"
else
    printf '%s' "$NEW_PLUGINS" | jq '{"version":2,"plugins":.}' > "$INST"
fi

# Create or merge settings.json
mkdir -p "$(dirname "$SETT")"
if [ -f "$SETT" ]; then
    jq --argjson e "$NEW_ENABLED" '.enabledPlugins += $e' "$SETT" > "$SETT.tmp" && mv "$SETT.tmp" "$SETT"
else
    printf '%s' "$NEW_ENABLED" | jq '{"enabledPlugins":.}' > "$SETT"
fi

# Register marketplace (merge with existing if present)
NEW_MARKET=$(printf '%s' '{}' | jq \
    --arg m "$MKT" --arg d "$MKT_DIR" --arg t "$TS" \
    '{($m): {"source": {"source": "github", "repo": "umputun/cc-thingz"}, "installLocation": $d, "lastUpdated": $t}}')
if [ -f "$MARKETS" ]; then
    jq --argjson m "$NEW_MARKET" '. += $m' "$MARKETS" > "$MARKETS.tmp" && mv "$MARKETS.tmp" "$MARKETS"
else
    mkdir -p "$(dirname "$MARKETS")"
    printf '%s' "$NEW_MARKET" > "$MARKETS"
fi

echo "cc-thingz plugins installed: $PLUGINS"
