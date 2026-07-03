#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# E2E regression tests for behaviors that only appear when the daemon is
# started on demand via real D-Bus activation (i.e. NOT observable with
# unit tests or with a pre-started daemon):
#
#  1. D-Bus activation race
#     The very first method call after a cold start must succeed. The bus
#     daemon queues the client's message and delivers it the instant the
#     well-known name org.freedesktop.secrets is acquired. If the daemon
#     requests the name BEFORE exporting its D-Bus objects, that first
#     call fails with org.freedesktop.DBus.Error.UnknownInterface and
#     clients (e.g. the GitHub Copilot CLI keyring probe) fall back to
#     plaintext storage.
#
#  2. Content-type round-trip
#     `secret-tool store` followed by `secret-tool lookup` must return
#     the stored password. libsecret's secret_value_get_text() only
#     decodes secrets whose content type is exactly "text/plain"; if the
#     daemon overrides the client-supplied content type, lookup fails
#     with "secret does not contain a textual password".
#
# The test starts its own private dbus-daemon with a servicedir that
# activates the daemon under test, so no systemd, WSL2 or Windows is
# required (the mock wincred helper is used for secret storage).
#
# Usage:
#   bash tests/e2e/run-tests-activation.sh
#   make e2e-test-activation
#
# Environment overrides:
#   DAEMON_BIN  path to wsl-secret-service   (default: <repo>/bin/wsl-secret-service)
#   HELPER_BIN  path to mock-wincred-helper  (default: <repo>/bin/mock-wincred-helper)

set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

DAEMON_BIN="${DAEMON_BIN:-$REPO_ROOT/bin/wsl-secret-service}"
HELPER_BIN="${HELPER_BIN:-$REPO_ROOT/bin/mock-wincred-helper}"

# Number of cold-start attempts for the activation race test. The race is
# deterministic in practice, but repeat to guard against timing flukes.
COLD_START_ATTEMPTS="${COLD_START_ATTEMPTS:-3}"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

log_info() { echo "[INFO] $*"; }
log_pass() { echo -e "${GREEN}[PASS]${NC} $*"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $*"; }

PASSED=0
FAILED=0

#############################################################################
# Private D-Bus session with activation configured
#############################################################################

TMP="$(mktemp -d "${TMPDIR:-/tmp}/wsl-ss-activation-XXXXXX")"
BUS_SOCKET="$TMP/bus"
BUS_ADDRESS="unix:path=$BUS_SOCKET"
SERVICE_DIR="$TMP/services"
CONFIG_DIR="$TMP/config"
export MOCK_WINCRED_STORE="$TMP/store.jsonl"
BUS_PID=""

cleanup() {
    local code=$?
    stop_daemon || true
    [[ -n "$BUS_PID" ]] && kill "$BUS_PID" 2>/dev/null
    rm -rf "$TMP"
    echo ""
    echo "======================================"
    echo "Test Summary:"
    echo "  Passed: $PASSED"
    echo "  Failed: $FAILED"
    echo "======================================"
    [[ $FAILED -gt 0 ]] && exit 1
    exit $code
}
trap cleanup EXIT

setup_bus() {
    mkdir -p "$SERVICE_DIR" "$CONFIG_DIR"

    cat > "$TMP/session.conf" <<EOF
<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>$BUS_ADDRESS</listen>
  <servicedir>$SERVICE_DIR</servicedir>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <!-- Same rules as the stock session.conf: allow everything. -->
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
EOF

    # D-Bus activation launches the daemon directly (no systemd), so the
    # session bus address must be injected explicitly via env.
    cat > "$SERVICE_DIR/org.freedesktop.secrets.service" <<EOF
[D-BUS Service]
Name=org.freedesktop.secrets
Exec=/usr/bin/env DBUS_SESSION_BUS_ADDRESS=$BUS_ADDRESS MOCK_WINCRED_STORE=$MOCK_WINCRED_STORE $DAEMON_BIN --config-dir $CONFIG_DIR --helper-path $HELPER_BIN --disable-memprotect --timeout 300s
EOF

    dbus-daemon --config-file="$TMP/session.conf" --nofork &
    BUS_PID=$!
    export DBUS_SESSION_BUS_ADDRESS="$BUS_ADDRESS"

    local i=0
    while [[ ! -S "$BUS_SOCKET" ]]; do
        sleep 0.1
        ((i++))
        if [[ $i -gt 50 ]] || ! kill -0 "$BUS_PID" 2>/dev/null; then
            echo "ERROR: private dbus-daemon failed to start" >&2
            exit 1
        fi
    done
    log_info "private D-Bus session ready: $BUS_ADDRESS"
}

#############################################################################
# Daemon lifecycle helpers
#############################################################################

# PID of the (bus-activated) daemon currently owning org.freedesktop.secrets.
get_daemon_pid() {
    dbus-send --session --print-reply \
        --dest=org.freedesktop.DBus /org/freedesktop/DBus \
        org.freedesktop.DBus.GetConnectionUnixProcessID \
        string:org.freedesktop.secrets 2>/dev/null \
        | awk '/uint32/ {print $2}'
}

name_has_owner() {
    dbus-send --session --print-reply \
        --dest=org.freedesktop.DBus /org/freedesktop/DBus \
        org.freedesktop.DBus.NameHasOwner \
        string:org.freedesktop.secrets 2>/dev/null \
        | grep -q "boolean true"
}

# Terminate the daemon (if running) and wait until the name is released,
# guaranteeing that the next call performs a genuine cold-start activation.
stop_daemon() {
    local pid
    pid="$(get_daemon_pid)"
    [[ -z "$pid" ]] && return 0
    kill "$pid" 2>/dev/null
    local i=0
    while name_has_owner; do
        sleep 0.1
        ((i++))
        if [[ $i -gt 50 ]]; then
            kill -KILL "$pid" 2>/dev/null
            sleep 0.5
            break
        fi
    done
    return 0
}

# A single OpenSession("plain") call. On a cold bus this triggers D-Bus
# activation; the bus daemon queues the message and delivers it as soon as
# the daemon claims the name.
open_session_once() {
    dbus-send --session --print-reply \
        --dest=org.freedesktop.secrets /org/freedesktop/secrets \
        org.freedesktop.Secret.Service.OpenSession \
        string:plain variant:string:"" 2>&1
}

#############################################################################
# Test 1: first call after cold-start activation must succeed
#############################################################################

test_activation_race() {
    local attempt output
    for attempt in $(seq 1 "$COLD_START_ATTEMPTS"); do
        log_info "cold-start attempt $attempt/$COLD_START_ATTEMPTS"
        stop_daemon

        if ! output="$(open_session_once)"; then
            log_fail "activation race: first call after cold start failed (attempt $attempt)"
            echo "$output" | sed 's/^/    /'
            ((FAILED++))
            return 1
        fi
    done
    log_pass "activation race: first call succeeded on all $COLD_START_ATTEMPTS cold starts"
    ((PASSED++))
    return 0
}

#############################################################################
# Test 2: secret-tool store → lookup round trip
#############################################################################

test_content_type_roundtrip() {
    local secret="activation-e2e-secret-$$"
    local output

    if ! printf '%s' "$secret" | secret-tool store --label="activation-e2e" \
            service activation-e2e 2>&1; then
        log_fail "content-type: secret-tool store failed"
        ((FAILED++))
        return 1
    fi

    if ! output="$(secret-tool lookup service activation-e2e 2>&1)"; then
        log_fail "content-type: secret-tool lookup failed: $output"
        ((FAILED++))
        return 1
    fi

    if [[ "$output" != "$secret" ]]; then
        log_fail "content-type: lookup returned wrong value"
        echo "    expected: $secret"
        echo "    got:      $output"
        ((FAILED++))
        return 1
    fi

    secret-tool clear service activation-e2e 2>/dev/null
    log_pass "content-type: store/lookup round trip returned the stored password"
    ((PASSED++))
    return 0
}

#############################################################################
# Main
#############################################################################

main() {
    log_info "=============================================="
    log_info "E2E Activation Test Runner (mock mode)"
    log_info "=============================================="

    for bin in "$DAEMON_BIN" "$HELPER_BIN"; do
        if [[ ! -x "$bin" ]]; then
            echo "ERROR: binary not found: $bin (run: make build-linux build-mock-helper)" >&2
            exit 1
        fi
    done
    for cmd in dbus-daemon dbus-send secret-tool; do
        if ! command -v "$cmd" &>/dev/null; then
            echo "ERROR: required command not found: $cmd" >&2
            exit 1
        fi
    done

    log_info "daemon under test: $DAEMON_BIN"
    setup_bus

    test_activation_race
    test_content_type_roundtrip
}

main
