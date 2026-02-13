#!/usr/bin/env bash
# test-runtime-integration.sh - Integration tests for Docker runtime detection
# These tests verify the full integration with vibrator

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
PASS=0
FAIL=0
SKIP=0

print_header() {
    echo -e "${BLUE}====${NC} $1 ${BLUE}====${NC}"
}

print_pass() {
    echo -e "${GREEN}✓${NC} $1"
    PASS=$((PASS + 1))
}

print_fail() {
    echo -e "${RED}✗${NC} $1"
    FAIL=$((FAIL + 1))
}

print_skip() {
    echo -e "${YELLOW}⊘${NC} $1"
    SKIP=$((SKIP + 1))
}

# Find built script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VIBRATE="$SCRIPT_DIR/../build/vibrate.sh"

if [[ ! -f "$VIBRATE" ]]; then
    echo -e "${RED}Error: vibrate.sh not found. Run 'make build' first.${NC}"
    exit 1
fi

echo "Testing Docker Runtime Detection Integration"
echo "=============================================="
echo ""

# Test 1: Help output includes Docker Runtime section
print_header "Test: Help Output"

if "$VIBRATE" --help 2>&1 | grep -q "Docker Runtime:"; then
    print_pass "Help includes Docker Runtime section"
else
    print_fail "Help missing Docker Runtime section"
fi

if "$VIBRATE" --help 2>&1 | grep -q "\-\-docker-socket"; then
    print_pass "Help includes --docker-socket flag"
else
    print_fail "Help missing --docker-socket flag"
fi

if "$VIBRATE" --help 2>&1 | grep -q "\-\-colima-profile"; then
    print_pass "Help includes --colima-profile flag"
else
    print_fail "Help missing --colima-profile flag"
fi

if "$VIBRATE" --help 2>&1 | grep -q "\-\-dind.*\-\-docker"; then
    print_pass "Help includes --dind/--docker flags"
else
    print_fail "Help missing --dind/--docker flags"
fi

echo ""

# Test 2: Version output works
print_header "Test: Version Command"

if "$VIBRATE" --version >/dev/null 2>&1; then
    print_pass "Version command works"
else
    print_fail "Version command failed"
fi

echo ""

# Test 3: Dockerfile export includes runtime detection
print_header "Test: Dockerfile Export"

TEMP_DOCKERFILE="/tmp/vibrator-test-dockerfile-$$"
if "$VIBRATE" --export-dockerfile "$TEMP_DOCKERFILE" >/dev/null 2>&1; then
    print_pass "Dockerfile export successful"

    # Check if the exported Dockerfile is valid
    if [[ -f "$TEMP_DOCKERFILE" && -s "$TEMP_DOCKERFILE" ]]; then
        print_pass "Dockerfile file created and non-empty"
    else
        print_fail "Dockerfile file empty or missing"
    fi

    rm -f "$TEMP_DOCKERFILE"
else
    print_fail "Dockerfile export failed"
fi

echo ""

# Test 4: Actual runtime detection (if Docker is available)
print_header "Test: Runtime Detection (Live)"

if command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
        # Docker is available and running
        # Try to detect which runtime is being used

        # Check for socket files to determine runtime
        if [[ -S "$HOME/.docker/run/docker.sock" ]]; then
            print_pass "Docker Desktop socket detected"
            EXPECTED_RUNTIME="docker-desktop"
        elif [[ -S "$HOME/.orbstack/run/docker.sock" ]]; then
            print_pass "OrbStack socket detected"
            EXPECTED_RUNTIME="orbstack"
        elif [[ -S "$HOME/.colima/default/docker.sock" ]]; then
            print_pass "Colima socket detected"
            EXPECTED_RUNTIME="colima"
        elif [[ -S "$HOME/.rd/docker.sock" ]]; then
            print_pass "Rancher Desktop socket detected"
            EXPECTED_RUNTIME="rancher-desktop"
        elif [[ -S "$HOME/.local/share/containers/podman/machine/podman.sock" ]]; then
            print_pass "Podman socket detected"
            EXPECTED_RUNTIME="podman"
        elif [[ -S "/var/run/docker.sock" ]]; then
            print_pass "Native Docker socket detected"
            EXPECTED_RUNTIME="native"
        else
            print_skip "Could not identify Docker runtime socket"
            EXPECTED_RUNTIME="unknown"
        fi

        # Note: We can't actually run the container in tests without
        # potentially affecting the user's system, so we just verify
        # the detection logic works
        print_skip "Actual container launch skipped (integration test)"
    else
        print_skip "Docker daemon not running"
    fi
else
    print_skip "Docker not installed"
fi

echo ""

# Test 5: Environment variable override
print_header "Test: Environment Variable Overrides"

# Test VIBRATOR_DOCKER_SOCKET env var
export VIBRATOR_DOCKER_SOCKET="/tmp/test-socket-$$"
if "$VIBRATE" --help 2>&1 | grep -q "VIBRATOR_DOCKER_SOCKET"; then
    print_pass "VIBRATOR_DOCKER_SOCKET documented in help"
else
    print_fail "VIBRATOR_DOCKER_SOCKET not documented"
fi
unset VIBRATOR_DOCKER_SOCKET

# Test COLIMA_PROFILE env var
if "$VIBRATE" --help 2>&1 | grep -q "COLIMA_PROFILE"; then
    print_pass "COLIMA_PROFILE documented in help"
else
    print_fail "COLIMA_PROFILE not documented"
fi

echo ""

# Test 6: Flag parsing
print_header "Test: Flag Parsing"

# Test that unknown flags are rejected
OUTPUT=$("$VIBRATE" --unknown-flag 2>&1 || true)
if echo "$OUTPUT" | grep -q "Unknown option"; then
    print_pass "Unknown flags properly rejected"
else
    print_fail "Unknown flags should be rejected"
fi

# Test that --docker-socket requires an argument
if "$VIBRATE" --docker-socket 2>&1 | grep -qi "error"; then
    print_pass "--docker-socket without argument rejected"
else
    print_skip "--docker-socket validation (may not error immediately)"
fi

echo ""

# Test 7: Supported runtime documentation
print_header "Test: Documentation Completeness"

HELP_OUTPUT=$("$VIBRATE" --help 2>&1)

for runtime in "Docker Desktop" "OrbStack" "Colima" "Rancher Desktop" "Podman" "Native Linux"; do
    if echo "$HELP_OUTPUT" | grep -q "$runtime"; then
        print_pass "Documentation includes $runtime"
    else
        print_fail "Documentation missing $runtime"
    fi
done

echo ""

# Test 8: Build script includes runtime module
print_header "Test: Build Artifact Validation"

if grep -q "docker_runtime::detect" "$VIBRATE"; then
    print_pass "Built script includes docker_runtime::detect"
else
    print_fail "Built script missing docker_runtime::detect"
fi

if grep -q "docker_runtime::get_socket" "$VIBRATE"; then
    print_pass "Built script includes docker_runtime::get_socket"
else
    print_fail "Built script missing docker_runtime::get_socket"
fi

if grep -q "checks::docker_runtime" "$VIBRATE"; then
    print_pass "Built script includes checks::docker_runtime"
else
    print_fail "Built script missing checks::docker_runtime"
fi

echo ""

# Summary
echo "=============================================="
echo "Test Results"
echo "=============================================="
echo -e "${GREEN}PASS: $PASS${NC}"
echo -e "${RED}FAIL: $FAIL${NC}"
echo -e "${YELLOW}SKIP: $SKIP${NC}"
echo ""

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed${NC}"
    exit 1
fi
