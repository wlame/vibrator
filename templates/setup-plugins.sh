#!/bin/sh
# Bake Claude plugins into the Docker image at build time.
# Called from Dockerfile with the list of detected plugins.
set -e

PLUGIN_LIST="$1"
if [ -z "$PLUGIN_LIST" ]; then
  echo "No plugins to install"
  exit 0
fi

CLAUDE_DIR="$HOME/.claude"
MKT_NAME="claude-plugins-official"
MKT_REPO="https://github.com/anthropics/claude-plugins-official.git"
MKT_DIR="$CLAUDE_DIR/plugins/marketplaces/$MKT_NAME"
CACHE_DIR="$CLAUDE_DIR/plugins/cache/$MKT_NAME"

mkdir -p "$(dirname "$MKT_DIR")"
git clone --depth 1 "$MKT_REPO" "$MKT_DIR"

GIT_SHA=$(cd "$MKT_DIR" && git rev-parse HEAD)
GIT_SHORT=$(echo "$GIT_SHA" | cut -c1-12)
TS=$(date -u +%Y-%m-%dT%H:%M:%S.000Z)

mkdir -p "$CACHE_DIR"

INST="$CLAUDE_DIR/plugins/installed_plugins.json"
SETT="$CLAUDE_DIR/settings.json"

printf '{\n  "version": 2,\n  "plugins": {' > "$INST"
printf '{\n  "enabledPlugins": {' > "$SETT"

FIRST=true
for ENTRY in $PLUGIN_LIST; do
  NAME="${ENTRY%%@*}"

  SRC=""
  [ -d "$MKT_DIR/plugins/$NAME" ] && SRC="$MKT_DIR/plugins/$NAME"
  [ -d "$MKT_DIR/external_plugins/$NAME" ] && SRC="$MKT_DIR/external_plugins/$NAME"

  if [ -z "$SRC" ]; then
    echo "Warning: Plugin $NAME not found in marketplace, skipping"
    continue
  fi

  DEST="$CACHE_DIR/$NAME/$GIT_SHORT"
  mkdir -p "$DEST"
  cp -r "$SRC/." "$DEST/"

  if [ "$FIRST" = true ]; then
    FIRST=false
  else
    printf "," >> "$INST"
    printf "," >> "$SETT"
  fi

  printf '\n    "%s": [{"scope":"user","installPath":"%s","version":"%s","installedAt":"%s","lastUpdated":"%s","gitCommitSha":"%s"}]' \
    "$ENTRY" "$DEST" "$GIT_SHORT" "$TS" "$TS" "$GIT_SHA" >> "$INST"
  printf '\n    "%s": true' "$ENTRY" >> "$SETT"

  echo "Installed plugin: $ENTRY"
done

printf '\n  }\n}\n' >> "$INST"
printf '\n  }\n}\n' >> "$SETT"

printf '{\n  "%s": {\n    "source": {"source":"github","repo":"anthropics/%s"},\n    "installLocation": "%s",\n    "lastUpdated": "%s"\n  }\n}\n' \
  "$MKT_NAME" "$MKT_NAME" "$MKT_DIR" "$TS" > "$CLAUDE_DIR/plugins/known_marketplaces.json"

echo "Plugin setup complete"
