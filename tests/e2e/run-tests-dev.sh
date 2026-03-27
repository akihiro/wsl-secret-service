#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# E2E test runner for devcontainer / non-WSL2 environments.
#
# Uses mock-wincred-helper (file-based secret storage) instead of the real
# wincred-helper.exe, so no Windows Credential Manager is required.
#
# The script automatically wraps itself in dbus-run-session when
# DBUS_SESSION_BUS_ADDRESS is not already set, providing a temporary D-Bus
# session for the duration of the tests.
#
# Usage:
#   bash tests/e2e/run-tests-dev.sh [-v|--verbose]
#   make e2e-test-dev
#   make e2e-test-dev-verbose

set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse options before the potential re-exec so they survive into the inner run.
VERBOSE=0
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose) VERBOSE=1; shift ;;
        *)
            echo "Usage: $0 [-v|--verbose]"
            exit 1
            ;;
    esac
done

export VERBOSE

# Re-exec inside a temporary D-Bus session if one is not already active.
# dbus-run-session starts its own dbus-daemon, sets DBUS_SESSION_BUS_ADDRESS,
# runs the given command, then tears everything down automatically.
if [[ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]]; then
    _args=()
    [[ $VERBOSE -eq 1 ]] && _args+=("-v")
    exec dbus-run-session -- bash "$0" "${_args[@]}"
fi

# Source dev helper functions (compatible signatures with helpers.sh).
source "$SCRIPT_DIR/helpers-dev.sh"

# Source the same test modules used by run-tests.sh.
source "$SCRIPT_DIR/test-collections.sh"
source "$SCRIPT_DIR/test-secrets.sh"
source "$SCRIPT_DIR/test-attributes-search.sh"

#############################################################################
# Main
#############################################################################

main() {
    log_info "=========================================="
    log_info "E2E Test Runner (devcontainer / mock mode)"
    log_info "=========================================="
    echo ""

    if ! check_prerequisites; then
        log_error "Prerequisites check failed"
        exit 1
    fi
    echo ""

    if ! setup_test_environment; then
        log_error "Failed to setup test environment"
        exit 1
    fi
    echo ""

    run_collection_tests
    run_secret_tests
    run_attribute_search_tests

    log_info "All test suites completed"
    echo ""
}

main
# cleanup_on_exit runs automatically via the EXIT trap in helpers-dev.sh
