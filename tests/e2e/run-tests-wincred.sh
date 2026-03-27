#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Test runner for wincred-helper.exe IPC tests

set -o pipefail

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse options
VERBOSE=0
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose)
            VERBOSE=1
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [-v|--verbose]"
            exit 1
            ;;
    esac
done

export VERBOSE

# Path to wincred-helper
WINCRED_HELPER_BIN="${WINCRED_HELPER_BIN:-$HOME/.local/share/wsl-secret-service/wincred-helper.exe}"
export WINCRED_HELPER_BIN

# Source helper functions
source "$SCRIPT_DIR/helpers.sh"

# Source wincred-helper test module
source "$SCRIPT_DIR/test-wincred-helper.sh"

#############################################################################
# Main Test Execution
#############################################################################

main() {
    log_info "=========================================="
    log_info "wincred-helper Direct Test Suite"
    log_info "=========================================="
    echo ""

    # Check if helper exists
    if [ ! -x "$WINCRED_HELPER_BIN" ]; then
        log_error "wincred-helper not found: $WINCRED_HELPER_BIN"
        log_error "Build with: make build"
        exit 1
    fi

    log_info "Using wincred-helper: $WINCRED_HELPER_BIN"
    echo ""

    # Run tests
    run_wincred_helper_tests

    # Print final summary (done by cleanup_on_exit trap)
    log_info "wincred-helper test suite completed"
    echo ""
}

# Execute main function
main
# Note: cleanup_on_exit is called automatically via trap
