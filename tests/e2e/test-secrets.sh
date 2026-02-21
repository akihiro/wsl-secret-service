#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Secret storage and retrieval tests

test_store_and_retrieve_secret() {
    test_start "test_store_and_retrieve_secret"

    local secret_value="my-secure-password-123"
    local label="Test Password"

    # Store secret
    printf '%s' "$secret_value" | secret-tool store \
        --label "$label" \
        service github.com \
        username testuser \
        &>/tmp/test_store.log

    if [ $? -ne 0 ]; then
        test_fail "test_store_and_retrieve_secret" "Failed to store secret"
        cat /tmp/test_store.log
        return 1
    fi

    # Search for secret
    local search_result=$(secret-tool search service github.com username testuser 2>/dev/null | head -1)

    if [ "$search_result" != "$label" ]; then
        test_fail "test_store_and_retrieve_secret" "Expected '$label', got '$search_result'"
        return 1
    fi

    # Retrieve the actual value
    local retrieved=$(secret-tool lookup service github.com username testuser 2>/dev/null)

    if [ "$retrieved" != "$secret_value" ]; then
        test_fail "test_store_and_retrieve_secret" "Value mismatch: expected '$secret_value', got '$retrieved'"
        return 1
    fi

    test_pass "test_store_and_retrieve_secret"
    return 0
}

test_retrieve_with_encryption_session() {
    test_start "test_retrieve_with_encryption_session"

    # This test verifies that D-Bus session encryption works
    # by storing and retrieving a secret and checking metadata

    local secret_value="encrypted-test-value"
    local label="Encryption Test Secret"

    # Store secret via secret-tool (which uses session encryption)
    printf '%s' "$secret_value" | secret-tool store \
        --label "$label" \
        service enctest.com \
        user crypto_user \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_retrieve_with_encryption_session" "Failed to store secret"
        return 1
    fi

    # Retrieve it - this tests session-based retrieval
    local retrieved=$(secret-tool lookup service enctest.com user crypto_user 2>/dev/null)

    if [ "$retrieved" != "$secret_value" ]; then
        test_fail "test_retrieve_with_encryption_session" "Encrypted retrieval failed"
        return 1
    fi

    # Verify metadata was stored (indicating successful session establishment)
    if [ -f "$TEST_CONFIG_DIR/metadata.json" ]; then
        if jq '.collections[].items[] | select(.label=="'"$label"'")' "$TEST_CONFIG_DIR/metadata.json" &>/dev/null; then
            test_pass "test_retrieve_with_encryption_session"
            return 0
        fi
    fi

    test_fail "test_retrieve_with_encryption_session" "Metadata not found after retrieval"
    return 1
}

test_secret_deleted_after_unlock_and_delete() {
    test_start "test_secret_deleted_after_unlock_and_delete"

    # Store a secret first
    local label="Delete Test Secret"
    printf 'secret-to-delete' | secret-tool store \
        --label "$label" \
        service deleteme.com \
        user deluser \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_secret_deleted_after_unlock_and_delete" "Failed to store secret"
        return 1
    fi

    # Verify it exists
    if ! secret-tool search service deleteme.com user deluser &>/dev/null; then
        test_fail "test_secret_deleted_after_unlock_and_delete" "Setup failed - secret not stored"
        return 1
    fi

    # Delete the secret
    if secret-tool delete service deleteme.com user deluser &>/dev/null; then
        # Verify it's gone
        if secret-tool search service deleteme.com user deluser &>/dev/null; then
            test_fail "test_secret_deleted_after_unlock_and_delete" "Secret still exists after deletion"
            return 1
        fi

        test_pass "test_secret_deleted_after_unlock_and_delete"
        return 0
    fi

    test_fail "test_secret_deleted_after_unlock_and_delete" "Failed to delete secret"
    return 1
}

test_large_secret_storage() {
    test_start "test_large_secret_storage"

    # Create a 10KB secret
    local large_secret=$(head -c 10240 /dev/urandom | base64)
    local label="Large Secret"

    # Store it
    printf '%s' "$large_secret" | secret-tool store \
        --label "$label" \
        service largetest.com \
        user largeuser \
        &>/tmp/test_large.log

    if [ $? -ne 0 ]; then
        test_fail "test_large_secret_storage" "Failed to store large secret"
        tail /tmp/test_large.log
        return 1
    fi

    # Retrieve it
    local retrieved=$(secret-tool lookup service largetest.com user largeuser 2>/dev/null)

    if [ "$retrieved" = "$large_secret" ]; then
        test_pass "test_large_secret_storage"
        return 0
    fi

    test_fail "test_large_secret_storage" "Large secret retrieval mismatch"
    return 1
}

test_special_characters_in_secret() {
    test_start "test_special_characters_in_secret"

    # Test various special characters including UTF-8
    local secret_value='Special chars: !@#$%^&*()_+-=[]{}|;:,.<>?/\~`"'"'"'üñé中文'
    local label="Special Chars Secret"

    # Store secret with special characters
    printf '%s' "$secret_value" | secret-tool store \
        --label "$label" \
        service special.test \
        user special_user \
        &>/tmp/test_special.log

    if [ $? -ne 0 ]; then
        test_fail "test_special_characters_in_secret" "Failed to store special chars"
        tail /tmp/test_special.log
        return 1
    fi

    # Retrieve and verify
    local retrieved=$(secret-tool lookup service special.test user special_user 2>/dev/null)

    if [ "$retrieved" = "$secret_value" ]; then
        test_pass "test_special_characters_in_secret"
        return 0
    fi

    test_fail "test_special_characters_in_secret" "Special character retrieval mismatch"
    debug_dump "Expected: $secret_value"
    debug_dump "Got: $retrieved"
    return 1
}

test_secret_content_type_preserved() {
    test_start "test_secret_content_type_preserved"

    # Store a secret
    local secret_value="content-type-test-value"
    local label="Content Type Test"

    printf '%s' "$secret_value" | secret-tool store \
        --label "$label" \
        service contenttype.test \
        user ctuser \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_secret_content_type_preserved" "Failed to store secret"
        return 1
    fi

    # Verify metadata contains content type information
    if [ -f "$TEST_CONFIG_DIR/metadata.json" ]; then
        local item_metadata=$(jq '.collections[].items[] | select(.label=="'"$label"'")' \
            "$TEST_CONFIG_DIR/metadata.json" 2>/dev/null)

        if [ -n "$item_metadata" ]; then
            # Check that contentType exists (even if empty)
            if echo "$item_metadata" | jq -e '.contentType' &>/dev/null; then
                test_pass "test_secret_content_type_preserved"
                return 0
            fi
        fi
    fi

    test_fail "test_secret_content_type_preserved" "Content type not preserved in metadata"
    debug_dump_collections
    return 1
}

# Run all tests
run_secret_tests() {
    log_info "Running secret storage and retrieval tests..."
    echo ""

    test_store_and_retrieve_secret
    test_retrieve_with_encryption_session
    test_secret_deleted_after_unlock_and_delete
    test_large_secret_storage
    test_special_characters_in_secret
    test_secret_content_type_preserved

    echo ""
}
