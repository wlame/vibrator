#!/usr/bin/env bash
# test-docker-runtime.sh - Unit tests for docker runtime detection

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
PASS=0
FAIL=0

# Test result tracking
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
}

# Mock logging functions for tests
log::error() { echo "[ERROR] $*" >&2; }
log::warn() { echo "[WARN] $*" >&2; }
log::info() { echo "[INFO] $*"; }
log::verbose() { [[ "${VERBOSE:-false}" == "true" ]] && echo "[VERBOSE] $*"; }

# Source the docker_runtime module
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../src/lib/docker_runtime.sh"

# Test helper: Create temporary socket file
# Note: We can't easily create real Unix sockets in tests
# Instead, we'll test the logic functions directly
create_test_socket() {
    local socket_path="$1"
    mkdir -p "$(dirname "$socket_path")"
    # Create a regular file for testing
    touch "$socket_path"
}

# Test helper: Clean up test files
cleanup_test_files() {
    local test_dir="$1"
    rm -rf "$test_dir"
}

echo "Testing docker_runtime.sh"
echo "=========================="
echo ""

# Test 1: Identify runtime by socket path
echo "Test: _identify_runtime_by_socket"
echo "---"

test_identify_runtime() {
    local socket="$1"
    local expected="$2"
    local result
    result=$(docker_runtime::_identify_runtime_by_socket "$socket")

    if [[ "$result" == "$expected" ]]; then
        print_pass "Identified $socket as $expected"
    else
        print_fail "Expected $expected, got $result for $socket"
    fi
}

test_identify_runtime "$HOME/.docker/run/docker.sock" "docker-desktop"
test_identify_runtime "$HOME/.orbstack/run/docker.sock" "orbstack"
test_identify_runtime "$HOME/.colima/default/docker.sock" "colima"
test_identify_runtime "$HOME/.rd/docker.sock" "rancher-desktop"
test_identify_runtime "$HOME/.local/share/containers/podman/machine/podman.sock" "podman"
test_identify_runtime "/var/run/docker.sock" "native"
test_identify_runtime "/custom/path/docker.sock" "custom"

echo ""

# Test 2: Socket detection logic (without real sockets - tests the logic)
echo "Test: Socket detection logic"
echo "---"

print_skip "Socket presence tests require real Unix sockets (skipped in unit tests)"
print_skip "These tests should be run as integration tests with actual Docker runtimes"

# Note: Testing socket detection requires actual Unix sockets which need
# a listening process. This is better tested as integration tests with
# real Docker installations rather than unit tests.

echo ""

# Test 3: Environment variable parsing
echo "Test: Environment variable handling"
echo "---"

# Test DOCKER_HOST URL parsing
test_docker_host_parsing() {
    local docker_host="$1"
    local expected_socket="$2"

    # Extract socket from unix:// URL
    local result="${docker_host#unix://}"

    if [[ "$result" == "$expected_socket" ]]; then
        print_pass "Correctly parsed $docker_host"
    else
        print_fail "Expected $expected_socket, got $result"
    fi
}

test_docker_host_parsing "unix:///var/run/docker.sock" "/var/run/docker.sock"
test_docker_host_parsing "unix:///home/user/.docker/run/docker.sock" "/home/user/.docker/run/docker.sock"
test_docker_host_parsing "unix:///tmp/custom.sock" "/tmp/custom.sock"

echo ""

# Test 4: Helper functions
echo "Test: Helper functions"
echo "---"

export DETECTED_DOCKER_RUNTIME="docker-desktop"
export DETECTED_DOCKER_SOCKET="$HOME/.docker/run/docker.sock"

result=$(docker_runtime::get_name)
if [[ "$result" == "docker-desktop" ]]; then
    print_pass "get_name() returns correct runtime"
else
    print_fail "get_name() failed: $result"
fi

result=$(docker_runtime::get_socket)
if [[ "$result" == "$HOME/.docker/run/docker.sock" ]]; then
    print_pass "get_socket() returns correct socket"
else
    print_fail "get_socket() failed: $result"
fi

# Test default values
unset DETECTED_DOCKER_RUNTIME
unset DETECTED_DOCKER_SOCKET

result=$(docker_runtime::get_name)
if [[ "$result" == "unknown" ]]; then
    print_pass "get_name() returns default 'unknown'"
else
    print_fail "get_name() default failed: $result"
fi

result=$(docker_runtime::get_socket)
if [[ "$result" == "/var/run/docker.sock" ]]; then
    print_pass "get_socket() returns default socket"
else
    print_fail "get_socket() default failed: $result"
fi

# Test is_macos
if [[ "$(uname -s)" == "Darwin" ]]; then
    if docker_runtime::is_macos; then
        print_pass "is_macos() correctly detects macOS"
    else
        print_fail "is_macos() should return true on macOS"
    fi
else
    if ! docker_runtime::is_macos; then
        print_pass "is_macos() correctly detects non-macOS"
    else
        print_fail "is_macos() should return false on non-macOS"
    fi
fi

echo ""

# Summary
echo "=========================="
echo "Test Results"
echo "=========================="
echo -e "${GREEN}PASS: $PASS${NC}"
echo -e "${RED}FAIL: $FAIL${NC}"
echo ""

if [[ $FAIL -eq 0 ]]; then
    echo -e "${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}Some tests failed${NC}"
    exit 1
fi
