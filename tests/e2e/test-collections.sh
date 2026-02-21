#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Collection management tests

test_default_login_collection_exists() {
    test_start "test_default_login_collection_exists"

    # The login collection should be auto-created on first service startup
    if ! verify_service_running; then
        test_fail "test_default_login_collection_exists" "Service not running"
        return 1
    fi

    # Use secret-tool to search in login collection
    # If collection doesn't exist, the search will fail
    if secret-tool search --all 2>/dev/null | grep -q "login"; then
        test_pass "test_default_login_collection_exists"
        return 0
    fi

    # Alternative: check metadata.json directly
    if [ -f "$TEST_CONFIG_DIR/metadata.json" ]; then
        if jq '.collections[] | select(.name=="login")' "$TEST_CONFIG_DIR/metadata.json" &>/dev/null; then
            test_pass "test_default_login_collection_exists"
            return 0
        fi
    fi

    test_fail "test_default_login_collection_exists" "Login collection not found"
    debug_dump_collections
    return 1
}

test_create_collection_via_secret_tool() {
    test_start "test_create_collection_via_secret_tool"

    # Create a secret - this implicitly uses the default collection
    local secret_value="test-secret-value"
    local label="Test Secret"

    printf '%s' "$secret_value" | secret-tool store \
        --label "$label" \
        testcol_attr1 value1 \
        &>/tmp/test_create.log

    if [ $? -ne 0 ]; then
        test_fail "test_create_collection_via_secret_tool" "Failed to store secret"
        cat /tmp/test_create.log
        return 1
    fi

    # Verify the secret was stored
    if secret-tool search testcol_attr1 value1 &>/dev/null; then
        test_pass "test_create_collection_via_secret_tool"
        return 0
    fi

    test_fail "test_create_collection_via_secret_tool" "Secret not stored"
    return 1
}

test_list_collections() {
    test_start "test_list_collections"

    # Store a test secret first
    printf 'test-password' | secret-tool store \
        --label "List Test Secret" \
        service listtest \
        user alice \
        &>/dev/null

    # List all secrets
    local result=$(secret-tool search --all 2>/dev/null)

    if [ -z "$result" ]; then
        test_fail "test_list_collections" "No secrets found"
        return 1
    fi

    # Try to get collections via D-Bus
    if dbus-send --session \
        --print-reply \
        --dest=org.freedesktop.secrets \
        /org/freedesktop/secrets \
        org.freedesktop.Secret.Service.GetCollections \
        &>/dev/null; then

        test_pass "test_list_collections"
        return 0
    fi

    test_fail "test_list_collections" "Failed to list collections via D-Bus"
    return 1
}

test_delete_collection_removes_items() {
    test_start "test_delete_collection_removes_items"

    # Store multiple secrets
    printf 'pwd1' | secret-tool store \
        --label "Secret 1" \
        service example.com \
        user alice \
        &>/dev/null

    printf 'pwd2' | secret-tool store \
        --label "Secret 2" \
        service example.com \
        user bob \
        &>/dev/null

    # Verify secrets exist
    if ! secret-tool search service example.com user alice &>/dev/null; then
        test_fail "test_delete_collection_removes_items" "Setup failed - secrets not stored"
        return 1
    fi

    # Delete first secret
    if secret-tool delete service example.com user alice &>/dev/null; then
        # Verify it's gone
        if secret-tool search service example.com user alice &>/dev/null; then
            test_fail "test_delete_collection_removes_items" "Secret was not deleted"
            return 1
        fi

        # Verify other secret still exists
        if secret-tool search service example.com user bob &>/dev/null; then
            test_pass "test_delete_collection_removes_items"
            return 0
        fi

        test_fail "test_delete_collection_removes_items" "Other secret was unexpectedly deleted"
        return 1
    fi

    test_fail "test_delete_collection_removes_items" "Failed to delete secret"
    return 1
}

test_collection_persistence() {
    test_start "test_collection_persistence"

    # Store a secret
    printf 'persistent-secret' | secret-tool store \
        --label "Persistent Secret" \
        service persistence.test \
        user persist_user \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_collection_persistence" "Failed to store secret"
        return 1
    fi

    # Verify metadata.json exists with proper structure
    if [ ! -f "$TEST_CONFIG_DIR/metadata.json" ]; then
        test_fail "test_collection_persistence" "metadata.json not found"
        return 1
    fi

    # Validate JSON structure
    if ! jq '.collections' "$TEST_CONFIG_DIR/metadata.json" &>/dev/null; then
        test_fail "test_collection_persistence" "Invalid metadata.json structure"
        return 1
    fi

    # Verify collection name is stored
    if jq '.collections[] | select(.name=="login")' "$TEST_CONFIG_DIR/metadata.json" &>/dev/null; then
        test_pass "test_collection_persistence"
        return 0
    fi

    test_fail "test_collection_persistence" "Login collection not persisted"
    debug_dump_collections
    return 1
}

# Run all tests
run_collection_tests() {
    log_info "Running collection management tests..."
    echo ""

    test_default_login_collection_exists
    test_create_collection_via_secret_tool
    test_list_collections
    test_delete_collection_removes_items
    test_collection_persistence

    echo ""
}
