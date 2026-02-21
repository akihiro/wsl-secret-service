#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Helper functions for e2e tests

set -o pipefail

# Global counters
PASSED_TESTS=0
FAILED_TESTS=0
TOTAL_TESTS=0

# Configuration
TEST_CONFIG_DIR="${TEST_CONFIG_DIR:-$HOME/.config/wsl-secret-service}"
TEST_DATA_DIR="${TEST_DATA_DIR:-$HOME/.local/share/wsl-secret-service}"
DAEMON_BIN="${DAEMON_BIN:-$HOME/.local/bin/wsl-secret-service}"
DAEMON_PID=""
TEST_TIMEOUT=30

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

#############################################################################
# Logging and Output Functions
#############################################################################

log_info() {
    echo "[INFO] $*"
}

log_test() {
    echo "[TEST] $*"
}

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

debug_dump() {
    if [ "${VERBOSE}" = "1" ]; then
        echo "--- Debug Dump ---"
        echo "$*"
        echo "--- End Debug Dump ---"
    fi
}

#############################################################################
# Prerequisite Checking
#############################################################################

check_wsl2_environment() {
    if ! grep -qi microsoft /proc/version 2>/dev/null; then
        log_error "Not in WSL2 environment"
        return 1
    fi
    log_info "WSL2 environment detected"
    return 0
}

check_systemd_user() {
    if ! systemctl --user list-units > /dev/null 2>&1; then
        log_error "systemd user instance not available"
        return 1
    fi
    log_info "systemd user instance available"
    return 0
}

check_secret_tool() {
    if ! command -v secret-tool > /dev/null 2>&1; then
        log_error "secret-tool not found"
        log_error "Install with: sudo apt-get install -y libsecret-tools"
        return 1
    fi
    local version=$(secret-tool --version 2>/dev/null || echo "unknown")
    log_info "secret-tool found: $version"
    return 0
}

check_jq() {
    if ! command -v jq > /dev/null 2>&1; then
        log_error "jq not found"
        log_error "Install with: sudo apt-get install -y jq"
        return 1
    fi
    log_info "jq found"
    return 0
}

check_dbus_send() {
    if ! command -v dbus-send > /dev/null 2>&1; then
        log_warn "dbus-send not found (debug features limited)"
        log_warn "Install with: sudo apt-get install -y dbus-tools"
        return 0  # Non-fatal
    fi
    log_info "dbus-send found (dbus-tools package)"
    return 0
}

check_dbus_session() {
    if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
        log_warn "DBUS_SESSION_BUS_ADDRESS not set, initializing..."
        eval $(dbus-launch --sh-syntax)
        export DBUS_SESSION_BUS_ADDRESS
    fi
    log_info "D-Bus session: $DBUS_SESSION_BUS_ADDRESS"
    return 0
}

check_prerequisites() {
    log_info "Checking prerequisites..."

    check_wsl2_environment || return 1
    check_systemd_user || return 1
    check_secret_tool || return 1
    check_jq || return 1
    check_dbus_send || return 0  # Non-fatal
    check_dbus_session || return 1

    log_info "All prerequisites met"
    return 0
}

#############################################################################
# Service Management
#############################################################################

stop_existing_service() {
    log_info "Stopping any existing service..."
    systemctl --user stop wsl-secret-service 2>/dev/null || true
    pkill -f "wsl-secret-service" 2>/dev/null || true
    sleep 1
}

clear_metadata() {
    log_info "Clearing metadata..."
    rm -rf "$TEST_CONFIG_DIR/metadata.json"
    rm -rf "$TEST_CONFIG_DIR"/*.tmp
    mkdir -p "$TEST_CONFIG_DIR" 2>/dev/null || true
}

verify_daemon_binary() {
    if [ ! -x "$DAEMON_BIN" ]; then
        log_error "Daemon binary not found or not executable: $DAEMON_BIN"
        return 1
    fi
    log_info "Daemon binary verified: $DAEMON_BIN"
    return 0
}

start_service() {
    log_info "Starting service..."

    verify_daemon_binary || return 1

    # Start daemon in background
    "$DAEMON_BIN" \
        --config-dir "$TEST_CONFIG_DIR" \
        --helper-path "$TEST_DATA_DIR/wincred-helper.exe" \
        &> /tmp/daemon.log &

    DAEMON_PID=$!
    log_info "Daemon started with PID $DAEMON_PID"

    return 0
}

wait_for_service() {
    local max_attempts=$TEST_TIMEOUT
    local attempt=0

    log_info "Waiting for service to be ready (max ${max_attempts}s)..."

    while [ $attempt -lt $max_attempts ]; do
        if dbus-send --session \
            --print-reply \
            --dest=org.freedesktop.secrets \
            /org/freedesktop/secrets \
            org.freedesktop.DBus.Properties.GetAll \
            string:org.freedesktop.Secret.Service \
            &>/dev/null; then

            log_info "Service became ready after ${attempt}s"
            return 0
        fi

        sleep 1
        ((attempt++))

        # Check if daemon process is still alive
        if ! kill -0 $DAEMON_PID 2>/dev/null; then
            log_error "Daemon process died unexpectedly"
            cat /tmp/daemon.log 2>/dev/null || true
            return 1
        fi
    done

    log_error "Service failed to become ready within ${max_attempts}s"
    log_error "Daemon logs:"
    cat /tmp/daemon.log 2>/dev/null || true
    return 1
}

stop_service() {
    log_info "Stopping service..."

    if [ -n "$DAEMON_PID" ]; then
        if kill -0 $DAEMON_PID 2>/dev/null; then
            kill $DAEMON_PID 2>/dev/null || true
            sleep 1

            # Force kill if still running
            if kill -0 $DAEMON_PID 2>/dev/null; then
                kill -9 $DAEMON_PID 2>/dev/null || true
            fi
        fi
        DAEMON_PID=""
    fi

    systemctl --user stop wsl-secret-service 2>/dev/null || true
    pkill -f "wsl-secret-service" 2>/dev/null || true

    return 0
}

verify_service_running() {
    if ! dbus-send --session \
        --print-reply \
        --dest=org.freedesktop.secrets \
        /org/freedesktop/secrets \
        org.freedesktop.DBus.Properties.GetAll \
        string:org.freedesktop.Secret.Service \
        &>/dev/null; then
        return 1
    fi
    return 0
}

#############################################################################
# Test Setup and Teardown
#############################################################################

setup_test_environment() {
    log_info "Setting up test environment..."

    stop_existing_service || return 1
    clear_metadata || return 1
    start_service || return 1
    wait_for_service || return 1

    log_info "Test environment ready"
    return 0
}

teardown_test_environment() {
    log_info "Tearing down test environment..."

    stop_service || true
    clear_metadata || true

    return 0
}

cleanup_on_exit() {
    local exit_code=$?

    log_info "Cleaning up..."
    teardown_test_environment || true

    # Print summary
    echo ""
    echo "======================================"
    echo "Test Summary:"
    echo "  Total:  $TOTAL_TESTS"
    echo "  Passed: $PASSED_TESTS"
    echo "  Failed: $FAILED_TESTS"
    echo "======================================"

    if [ $FAILED_TESTS -gt 0 ]; then
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

    if [ $exit_code -ne 0 ]; then
        log_fail "Command failed: $description"
        return 1
    fi

    return 0
}

assert_command_failure() {
    local exit_code=$1
    local description=$2

    if [ $exit_code -eq 0 ]; then
        log_fail "Command should have failed: $description"
        return 1
    fi

    return 0
}

assert_string_equals() {
    local expected=$1
    local actual=$2
    local description=$3

    if [ "$expected" != "$actual" ]; then
        log_fail "$description"
        log_fail "Expected: '$expected'"
        log_fail "Got: '$actual'"
        return 1
    fi

    return 0
}

assert_file_exists() {
    local filepath=$1
    local description=$2

    if [ ! -f "$filepath" ]; then
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
    local name=$1
    ((PASSED_TESTS++))
    log_pass "$name"
}

test_fail() {
    local name=$1
    local reason=$2

    ((FAILED_TESTS++))
    log_fail "$name"
    if [ -n "$reason" ]; then
        echo "  Reason: $reason"
    fi
}

#############################################################################
# Debug Functions
#############################################################################

debug_dump_collections() {
    if [ "${VERBOSE}" != "1" ]; then
        return 0
    fi

    echo "--- Collections Debug Dump ---"

    if [ -f "$TEST_CONFIG_DIR/metadata.json" ]; then
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
    if [ "${VERBOSE}" != "1" ]; then
        return 0
    fi

    echo "--- Daemon Logs ---"
    tail -20 /tmp/daemon.log 2>/dev/null || echo "No daemon logs found"
    echo "--- End Logs ---"
}

debug_service_status() {
    echo "--- Service Status ---"
    systemctl --user status wsl-secret-service --no-pager 2>/dev/null || \
        echo "Service not running via systemd"

    if [ -n "$DAEMON_PID" ]; then
        echo "Daemon PID: $DAEMON_PID"
        ps -p $DAEMON_PID 2>/dev/null || echo "Daemon process not found"
    fi
    echo "--- End Status ---"
}

#############################################################################
# Export trap for automatic cleanup
#############################################################################

trap cleanup_on_exit EXIT
