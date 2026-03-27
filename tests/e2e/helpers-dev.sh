#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Helper functions for E2E tests in devcontainer/mock mode.
# Implements the same function signatures as helpers.sh but uses
# mock-wincred-helper instead of wincred-helper.exe and does not
# require WSL2 or systemd.

set -o pipefail

# Locate the repo root relative to this script file.
_DEV_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
_DEV_REPO_ROOT="$(cd "$_DEV_SCRIPT_DIR/../.." && pwd)"

# Binary paths (resolved from repo root, not ~/.local/bin)
DAEMON_BIN="$_DEV_REPO_ROOT/bin/wsl-secret-service"
HELPER_BIN="$_DEV_REPO_ROOT/bin/mock-wincred-helper"

# Temporary directory unique to this test run (supports parallel runs)
_E2E_TMPDIR="${TMPDIR:-/tmp}/wsl-ss-e2e-$$"
TEST_CONFIG_DIR="$_E2E_TMPDIR/config"
MOCK_WINCRED_STORE="$_E2E_TMPDIR/store.json"
_DAEMON_LOG="$_E2E_TMPDIR/daemon.log"
_DAEMON_PID=""

# Test counters
PASSED_TESTS=0
FAILED_TESTS=0
TOTAL_TESTS=0

VERBOSE="${VERBOSE:-0}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

#############################################################################
# Logging
#############################################################################

log_info()  { echo "[INFO] $*"; }
log_test()  { echo "[TEST] $*"; }
log_pass()  { echo -e "${GREEN}[PASS]${NC} $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC} $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $*"; }

debug_dump() {
    if [[ "${VERBOSE}" == "1" ]]; then
        echo "--- Debug Dump ---"
        echo "$*"
        echo "--- End Debug Dump ---"
    fi
}

#############################################################################
# Prerequisites
#############################################################################

check_prerequisites() {
    log_info "Checking prerequisites..."
    local ok=1

    if [[ ! -x "$DAEMON_BIN" ]]; then
        log_error "daemon binary not found: $DAEMON_BIN"
        log_error "Run: make build-linux"
        ok=0
    fi

    if [[ ! -x "$HELPER_BIN" ]]; then
        log_error "mock helper not found: $HELPER_BIN"
        log_error "Run: make build-mock-helper"
        ok=0
    fi

    if ! command -v secret-tool &>/dev/null; then
        log_error "secret-tool not found (apt install libsecret-tools)"
        ok=0
    fi

    if ! command -v jq &>/dev/null; then
        log_error "jq not found (apt install jq)"
        ok=0
    fi

    if ! command -v dbus-send &>/dev/null; then
        log_warn "dbus-send not found — debug features limited"
    fi

    if [[ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]]; then
        log_error "DBUS_SESSION_BUS_ADDRESS is not set"
        log_error "Wrap the test in dbus-run-session or run via run-tests-dev.sh"
        ok=0
    else
        log_info "D-Bus session: $DBUS_SESSION_BUS_ADDRESS"
    fi

    [[ $ok -eq 1 ]] || return 1
    log_info "All prerequisites met"
    return 0
}

#############################################################################
# Service Management
#############################################################################

clear_metadata() {
    log_info "Clearing metadata..."
    rm -f "$TEST_CONFIG_DIR/metadata.json" "$MOCK_WINCRED_STORE"
    mkdir -p "$TEST_CONFIG_DIR"
}

verify_daemon_binary() {
    if [[ ! -x "$DAEMON_BIN" ]]; then
        log_error "Daemon binary not found or not executable: $DAEMON_BIN"
        return 1
    fi
    log_info "Daemon binary verified: $DAEMON_BIN"
    return 0
}

start_service() {
    log_info "Starting service..."
    verify_daemon_binary || return 1

    mkdir -p "$TEST_CONFIG_DIR"

    # MOCK_WINCRED_STORE is exported; the daemon inherits it and passes it
    # to every mock-wincred-helper subprocess it spawns.
    "$DAEMON_BIN" \
        --config-dir "$TEST_CONFIG_DIR" \
        --helper-path "$HELPER_BIN" \
        --disable-memprotect \
        --timeout 300s \
        &>"$_DAEMON_LOG" &

    _DAEMON_PID=$!
    log_info "Daemon started (PID $_DAEMON_PID)"
    return 0
}

wait_for_service() {
    local max_attempts=30
    local attempt=0

    log_info "Waiting for service to be ready (max ${max_attempts}s)..."

    while [[ $attempt -lt $max_attempts ]]; do
        if dbus-send --session \
            --print-reply \
            --dest=org.freedesktop.secrets \
            /org/freedesktop/secrets \
            org.freedesktop.DBus.Properties.GetAll \
            string:org.freedesktop.Secret.Service \
            &>/dev/null 2>&1; then
            log_info "Service ready after ${attempt}s"
            return 0
        fi

        sleep 1
        ((attempt++))

        if [[ -n "$_DAEMON_PID" ]] && ! kill -0 "$_DAEMON_PID" 2>/dev/null; then
            log_error "Daemon process died unexpectedly"
            debug_dump_daemon_logs
            return 1
        fi
    done

    log_error "Service failed to become ready within ${max_attempts}s"
    debug_dump_daemon_logs
    return 1
}

stop_service() {
    log_info "Stopping service..."
    if [[ -n "$_DAEMON_PID" ]]; then
        if kill -0 "$_DAEMON_PID" 2>/dev/null; then
            kill -TERM "$_DAEMON_PID" 2>/dev/null || true
            local i=0
            while kill -0 "$_DAEMON_PID" 2>/dev/null && [[ $i -lt 10 ]]; do
                sleep 0.5
                ((i++))
            done
            kill -KILL "$_DAEMON_PID" 2>/dev/null || true
        fi
        _DAEMON_PID=""
    fi
    # Belt-and-suspenders: kill stray processes from previous runs.
    pkill -f "wsl-secret-service" 2>/dev/null || true
    return 0
}

verify_service_running() {
    dbus-send --session \
        --print-reply \
        --dest=org.freedesktop.secrets \
        /org/freedesktop/secrets \
        org.freedesktop.DBus.Properties.GetAll \
        string:org.freedesktop.Secret.Service \
        &>/dev/null 2>&1
}

#############################################################################
# Test Setup / Teardown
#############################################################################

setup_test_environment() {
    log_info "Setting up test environment..."
    mkdir -p "$_E2E_TMPDIR"
    check_prerequisites || return 1
    stop_service 2>/dev/null || true
    clear_metadata
    start_service || return 1
    wait_for_service || return 1
    log_info "Test environment ready"
    return 0
}

teardown_test_environment() {
    log_info "Tearing down test environment..."
    stop_service || true
    rm -rf "$_E2E_TMPDIR"
    return 0
}

cleanup_on_exit() {
    local exit_code=$?
    log_info "Cleaning up..."
    teardown_test_environment || true

    echo ""
    echo "======================================"
    echo "Test Summary:"
    echo "  Total:  $TOTAL_TESTS"
    echo "  Passed: $PASSED_TESTS"
    echo "  Failed: $FAILED_TESTS"
    echo "======================================"

    if [[ $FAILED_TESTS -gt 0 ]]; then
        exit 1
    fi
    exit $exit_code
}

#############################################################################
# Assertion Functions
#############################################################################

assert_command_success() {
    local exit_code=$1
    local description=$2
    if [[ $exit_code -ne 0 ]]; then
        log_fail "Command failed: $description"
        return 1
    fi
    return 0
}

assert_command_failure() {
    local exit_code=$1
    local description=$2
    if [[ $exit_code -eq 0 ]]; then
        log_fail "Command should have failed: $description"
        return 1
    fi
    return 0
}

assert_string_equals() {
    local expected=$1
    local actual=$2
    local description=$3
    if [[ "$expected" != "$actual" ]]; then
        log_fail "$description"
        log_fail "Expected: '$expected'"
        log_fail "Got:      '$actual'"
        return 1
    fi
    return 0
}

assert_file_exists() {
    local filepath=$1
    local description=$2
    if [[ ! -f "$filepath" ]]; then
        log_fail "$description: File not found: $filepath"
        return 1
    fi
    return 0
}

assert_secret_exists() {
    local service=$1
    local user=$2
    if ! secret-tool search service "$service" user "$user" &>/dev/null; then
        log_fail "Secret not found: service=$service, user=$user"
        return 1
    fi
    return 0
}

assert_secret_not_exists() {
    local service=$1
    local user=$2
    if secret-tool search service "$service" user "$user" &>/dev/null; then
        log_fail "Secret should not exist: service=$service, user=$user"
        return 1
    fi
    return 0
}

#############################################################################
# Test Result Tracking
#############################################################################

test_start() {
    ((TOTAL_TESTS++))
    log_test "$1"
}

test_pass() {
    ((PASSED_TESTS++))
    log_pass "$1"
}

test_fail() {
    local name=$1
    local reason=${2:-}
    ((FAILED_TESTS++))
    log_fail "$name"
    if [[ -n "$reason" ]]; then
        echo "  Reason: $reason"
    fi
}

#############################################################################
# Debug Functions
#############################################################################

debug_dump_collections() {
    if [[ "${VERBOSE}" != "1" ]]; then
        return 0
    fi
    echo "--- Collections Debug Dump ---"
    if [[ -f "$TEST_CONFIG_DIR/metadata.json" ]]; then
        echo "Metadata contents:"
        jq '.collections' "$TEST_CONFIG_DIR/metadata.json" 2>/dev/null || \
            echo "Failed to parse metadata.json"
    else
        echo "No metadata.json found"
    fi
    echo "D-Bus collections:"
    dbus-send --session \
        --print-reply \
        --dest=org.freedesktop.secrets \
        /org/freedesktop/secrets \
        org.freedesktop.Secret.Service.GetCollections \
        2>/dev/null || echo "Failed to get collections"
    echo "--- End Debug Dump ---"
}

debug_dump_daemon_logs() {
    echo "--- Daemon Logs ---"
    tail -30 "$_DAEMON_LOG" 2>/dev/null || echo "No daemon logs found"
    echo "--- End Logs ---"
}

debug_service_status() {
    echo "--- Service Status ---"
    if [[ -n "$_DAEMON_PID" ]]; then
        echo "Daemon PID: $_DAEMON_PID"
        ps -p "$_DAEMON_PID" 2>/dev/null || echo "Daemon process not found"
    fi
    echo "--- End Status ---"
}

#############################################################################
# Export and trap
#############################################################################

# Export MOCK_WINCRED_STORE so the daemon (and the mock helpers it spawns)
# inherit the store path automatically.
export MOCK_WINCRED_STORE

trap cleanup_on_exit EXIT
