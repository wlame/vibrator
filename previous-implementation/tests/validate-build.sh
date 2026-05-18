#!/bin/bash
# Validate the built vibrate.sh artifact.
set -e

SCRIPT="${1:-build/vibrate.sh}"
PASS=0
FAIL=0

check() {
    local label="$1"
    shift
    if "$@" >/dev/null 2>&1; then
        echo "  PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $label"
        FAIL=$((FAIL + 1))
    fi
}

echo "Validating: $SCRIPT"
echo ""

# File exists and is executable
check "File exists" test -f "$SCRIPT"
check "File is executable" test -x "$SCRIPT"

# Bash syntax check
check "Syntax valid" bash -n "$SCRIPT"

# Version flag works
check "--version exits 0" bash -c "$SCRIPT --version"

# Help flag works
check "--help exits 0" bash -c "$SCRIPT --help"

# Export Dockerfile produces output
TMPDF=$(mktemp)
trap "rm -f $TMPDF" EXIT
check "--export-dockerfile produces file" bash -c "$SCRIPT --export-dockerfile $TMPDF && test -s $TMPDF"

# Exported Dockerfile contains expected stages
if [ -s "$TMPDF" ]; then
    check "Dockerfile has stage: base" grep -q "AS base" "$TMPDF"
    check "Dockerfile has stage: user-env" grep -q "AS user-env" "$TMPDF"
    check "Dockerfile has stage: claude-mcp" grep -q "AS claude-mcp" "$TMPDF"
    check "Dockerfile has stage: runtime" grep -q "AS runtime" "$TMPDF"
    check "Dockerfile has ENTRYPOINT" grep -q "ENTRYPOINT" "$TMPDF"
fi

# Built script contains expected markers
check "Contains version string" grep -q "VIBRATOR_VERSION=" "$SCRIPT"
check "Contains log::info function" grep -q "log::info()" "$SCRIPT"
check "Contains docker_cmd::build function" grep -q "docker_cmd::build()" "$SCRIPT"
check "Contains main function" grep -q "^main()" "$SCRIPT"

# No unresolved placeholders
check "No %%VERSION%% placeholders" bash -c "! grep -q '%%VERSION%%' $SCRIPT"
check "No %%DOCKERFILE_TPL_B64%% placeholders" bash -c "! grep -q '%%DOCKERFILE_TPL_B64%%' $SCRIPT"
check "No %%ENTRYPOINT_B64%% placeholders" bash -c "! grep -q '\"%%ENTRYPOINT_B64%%\"' $SCRIPT"

echo ""
echo "Results: $PASS passed, $FAIL failed"
[[ $FAIL -eq 0 ]] && exit 0 || exit 1
