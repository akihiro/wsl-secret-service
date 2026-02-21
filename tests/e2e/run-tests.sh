#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Main E2E test runner

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

# Source helper functions
source "$SCRIPT_DIR/helpers.sh"

# Source test modules
source "$SCRIPT_DIR/test-collections.sh"
source "$SCRIPT_DIR/test-secrets.sh"
source "$SCRIPT_DIR/test-attributes-search.sh"

#############################################################################
# Main Test Execution
#############################################################################

main() {
    log_info "=========================================="
    log_info "E2E Test Runner for wsl-secret-service"
    log_info "=========================================="
    echo ""

    # Check prerequisites
    if ! check_prerequisites; then
        log_error "Prerequisites check failed"
        exit 1
    fi
    echo ""

    # Setup test environment
    if ! setup_test_environment; then
        log_error "Failed to setup test environment"
        exit 1
    fi
    echo ""

    # Run collection tests
    run_collection_tests

    # Run secret tests
    run_secret_tests

    # Run attribute search tests
    run_attribute_search_tests

    # Print final summary (done by cleanup_on_exit trap)
    log_info "All test suites completed"
    echo ""
}

# Execute main function
main
# Note: cleanup_on_exit is called automatically via trap
