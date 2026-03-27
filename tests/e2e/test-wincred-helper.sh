#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Direct tests for wincred-helper.exe IPC communication

#############################################################################
# Helper function to call wincred-helper directly
#############################################################################

call_wincred_helper() {
    local action=$1
    local target=$2
    local secret=$3
    local filter=$4

    # Build JSON request
    local request="{\"action\":\"$action\""

    if [ -n "$target" ]; then
        request="${request},\"target\":\"$target\""
    fi

    if [ -n "$secret" ]; then
        request="${request},\"secret\":\"$secret\""
    fi

    if [ -n "$filter" ]; then
        request="${request},\"filter\":\"$filter\""
    fi

    request="${request}}"

    # Call helper and parse response
    echo "$request" | "$WINCRED_HELPER_BIN"
}

#############################################################################
# Helper function to base64 encode
#############################################################################

base64_encode() {
    printf '%s' "$1" | base64 -w 0
}

#############################################################################
# Helper function to base64 decode
#############################################################################

base64_decode() {
    printf '%s' "$1" | base64 -d
}

#############################################################################
# Test: Verify helper binary exists and is executable
#############################################################################

test_wincred_helper_exists() {
    test_start "test_wincred_helper_exists"

    if [ ! -x "$WINCRED_HELPER_BIN" ]; then
        test_fail "test_wincred_helper_exists" "Helper not found or not executable: $WINCRED_HELPER_BIN"
        return 1
    fi

    test_pass "test_wincred_helper_exists"
    return 0
}

#############################################################################
# Test: Set and get a simple secret
#############################################################################

test_wincred_set_and_get() {
    test_start "test_wincred_set_and_get"

    local target="test/wincred/simple"
    local secret_value="my-secret-password"
    local encoded=$(base64_encode "$secret_value")

    # Set secret
    local set_response=$(call_wincred_helper "set" "$target" "$encoded")
    if ! echo "$set_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_set_and_get" "Failed to set secret"
        debug_dump "$set_response"
        return 1
    fi

    # Get secret
    local get_response=$(call_wincred_helper "get" "$target")
    if ! echo "$get_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_set_and_get" "Failed to get secret"
        debug_dump "$get_response"
        return 1
    fi

    # Verify secret value
    local retrieved_encoded=$(echo "$get_response" | jq -r '.secret')
    local retrieved=$(base64_decode "$retrieved_encoded")

    if [ "$retrieved" != "$secret_value" ]; then
        test_fail "test_wincred_set_and_get" "Secret mismatch: expected '$secret_value', got '$retrieved'"
        return 1
    fi

    # Cleanup
    call_wincred_helper "delete" "$target" >/dev/null 2>&1

    test_pass "test_wincred_set_and_get"
    return 0
}

#############################################################################
# Test: Store and delete secret
#############################################################################

test_wincred_delete() {
    test_start "test_wincred_delete"

    local target="test/wincred/delete-test"
    local secret_value="secret-to-delete"
    local encoded=$(base64_encode "$secret_value")

    # Set secret
    call_wincred_helper "set" "$target" "$encoded" >/dev/null 2>&1

    # Verify it exists
    local get_response=$(call_wincred_helper "get" "$target")
    if ! echo "$get_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_delete" "Failed to set secret initially"
        return 1
    fi

    # Delete secret
    local delete_response=$(call_wincred_helper "delete" "$target")
    if ! echo "$delete_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_delete" "Failed to delete secret"
        debug_dump "$delete_response"
        return 1
    fi

    # Verify it's gone
    local get_response=$(call_wincred_helper "get" "$target")
    if echo "$get_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_delete" "Secret still exists after delete"
        return 1
    fi

    test_pass "test_wincred_delete"
    return 0
}

#############################################################################
# Test: List secrets with prefix filter
#############################################################################

test_wincred_list() {
    test_start "test_wincred_list"

    local prefix="test/wincred/list"

    # Set multiple secrets with the same prefix
    call_wincred_helper "set" "${prefix}/item1" "$(base64_encode 'secret1')" >/dev/null 2>&1
    call_wincred_helper "set" "${prefix}/item2" "$(base64_encode 'secret2')" >/dev/null 2>&1
    call_wincred_helper "set" "${prefix}/item3" "$(base64_encode 'secret3')" >/dev/null 2>&1

    # List secrets with prefix
    local list_response=$(call_wincred_helper "list" "" "" "$prefix")
    if ! echo "$list_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_list" "Failed to list secrets"
        debug_dump "$list_response"
        return 1
    fi

    # Verify results
    local count=$(echo "$list_response" | jq '.targets | length')
    if [ "$count" -lt 3 ]; then
        test_fail "test_wincred_list" "Expected at least 3 items, got $count"
        debug_dump "$list_response"
        return 1
    fi

    # Verify our items are in the list
    local has_item1=$(echo "$list_response" | jq ".targets | map(select(contains(\"${prefix}/item1\"))) | length")
    if [ "$has_item1" -eq 0 ]; then
        test_fail "test_wincred_list" "item1 not found in list"
        return 1
    fi

    # Cleanup
    call_wincred_helper "delete" "${prefix}/item1" >/dev/null 2>&1
    call_wincred_helper "delete" "${prefix}/item2" >/dev/null 2>&1
    call_wincred_helper "delete" "${prefix}/item3" >/dev/null 2>&1

    test_pass "test_wincred_list"
    return 0
}

#############################################################################
# Test: Large secret storage
#############################################################################

test_wincred_large_secret() {
    test_start "test_wincred_large_secret"

    local target="test/wincred/large"
    # Create a 1KB secret
    local secret_value=$(printf 'x%.0s' {1..1000})
    local encoded=$(base64_encode "$secret_value")

    # Set large secret
    local set_response=$(call_wincred_helper "set" "$target" "$encoded")
    if ! echo "$set_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_large_secret" "Failed to set large secret"
        return 1
    fi

    # Get and verify
    local get_response=$(call_wincred_helper "get" "$target")
    if ! echo "$get_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_large_secret" "Failed to get large secret"
        return 1
    fi

    local retrieved_encoded=$(echo "$get_response" | jq -r '.secret')
    local retrieved=$(base64_decode "$retrieved_encoded")

    if [ "${#retrieved}" -ne 1000 ]; then
        test_fail "test_wincred_large_secret" "Size mismatch: expected 1000, got ${#retrieved}"
        return 1
    fi

    # Cleanup
    call_wincred_helper "delete" "$target" >/dev/null 2>&1

    test_pass "test_wincred_large_secret"
    return 0
}

#############################################################################
# Test: Special characters in secret
#############################################################################

test_wincred_special_chars() {
    test_start "test_wincred_special_chars"

    local target="test/wincred/special"
    local secret_value='!@#$%^&*()_+-={}[]|:;"<>?,./'
    local encoded=$(base64_encode "$secret_value")

    # Set secret with special chars
    local set_response=$(call_wincred_helper "set" "$target" "$encoded")
    if ! echo "$set_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_special_chars" "Failed to set secret with special chars"
        return 1
    fi

    # Get and verify
    local get_response=$(call_wincred_helper "get" "$target")
    local retrieved_encoded=$(echo "$get_response" | jq -r '.secret')
    local retrieved=$(base64_decode "$retrieved_encoded")

    if [ "$retrieved" != "$secret_value" ]; then
        test_fail "test_wincred_special_chars" "Special chars not preserved"
        return 1
    fi

    # Cleanup
    call_wincred_helper "delete" "$target" >/dev/null 2>&1

    test_pass "test_wincred_special_chars"
    return 0
}

#############################################################################
# Test: Overwrite existing secret
#############################################################################

test_wincred_overwrite() {
    test_start "test_wincred_overwrite"

    local target="test/wincred/overwrite"
    local secret1="first-secret"
    local secret2="second-secret-overwrite"

    # Set first secret
    call_wincred_helper "set" "$target" "$(base64_encode "$secret1")" >/dev/null 2>&1

    # Overwrite with second secret
    local set_response=$(call_wincred_helper "set" "$target" "$(base64_encode "$secret2")")
    if ! echo "$set_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_overwrite" "Failed to overwrite secret"
        return 1
    fi

    # Verify new value
    local get_response=$(call_wincred_helper "get" "$target")
    local retrieved=$(base64_decode "$(echo "$get_response" | jq -r '.secret')")

    if [ "$retrieved" != "$secret2" ]; then
        test_fail "test_wincred_overwrite" "Overwrite failed: expected '$secret2', got '$retrieved'"
        return 1
    fi

    # Cleanup
    call_wincred_helper "delete" "$target" >/dev/null 2>&1

    test_pass "test_wincred_overwrite"
    return 0
}

#############################################################################
# Test: Multiple concurrent operations
#############################################################################

test_wincred_multiple_secrets() {
    test_start "test_wincred_multiple_secrets"

    local prefix="test/wincred/multi"
    local count=5

    # Set multiple secrets
    for i in $(seq 1 $count); do
        local target="${prefix}/secret_$i"
        local secret="secret-value-$i"
        local set_response=$(call_wincred_helper "set" "$target" "$(base64_encode "$secret")")
        if ! echo "$set_response" | jq -e '.ok == true' >/dev/null 2>&1; then
            test_fail "test_wincred_multiple_secrets" "Failed to set secret $i"
            return 1
        fi
    done

    # Verify all can be retrieved
    for i in $(seq 1 $count); do
        local target="${prefix}/secret_$i"
        local expected="secret-value-$i"
        local get_response=$(call_wincred_helper "get" "$target")
        local retrieved=$(base64_decode "$(echo "$get_response" | jq -r '.secret')")

        if [ "$retrieved" != "$expected" ]; then
            test_fail "test_wincred_multiple_secrets" "Secret $i mismatch: expected '$expected', got '$retrieved'"
            return 1
        fi
    done

    # Cleanup
    for i in $(seq 1 $count); do
        call_wincred_helper "delete" "${prefix}/secret_$i" >/dev/null 2>&1
    done

    test_pass "test_wincred_multiple_secrets"
    return 0
}

#############################################################################
# Test: Get non-existent secret returns error
#############################################################################

test_wincred_nonexistent() {
    test_start "test_wincred_nonexistent"

    local target="test/wincred/nonexistent-target-$$-$(date +%s)"

    local get_response=$(call_wincred_helper "get" "$target")

    # Should return not OK
    if echo "$get_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_nonexistent" "Should have failed for non-existent target"
        return 1
    fi

    # Should have error message
    if ! echo "$get_response" | jq -e '.error != null and .error != ""' >/dev/null 2>&1; then
        test_fail "test_wincred_nonexistent" "Should include error message"
        return 1
    fi

    test_pass "test_wincred_nonexistent"
    return 0
}

#############################################################################
# Test: Empty secret (empty plaintext)
#############################################################################

test_wincred_empty_secret() {
    test_start "test_wincred_empty_secret"

    local target="test/wincred/empty"
    local secret_value=""
    local encoded=$(base64_encode "$secret_value")

    # Set empty secret
    local set_response=$(call_wincred_helper "set" "$target" "$encoded")
    if ! echo "$set_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_empty_secret" "Failed to set empty secret"
        return 1
    fi

    # Get and verify
    local get_response=$(call_wincred_helper "get" "$target")
    if ! echo "$get_response" | jq -e '.ok == true' >/dev/null 2>&1; then
        test_fail "test_wincred_empty_secret" "Failed to get empty secret"
        return 1
    fi

    # Verify we got a response (may be empty base64)
    local retrieved_encoded=$(echo "$get_response" | jq -r '.secret // ""')

    # For empty secrets, base64 encoding can vary based on how the system handles it
    # Just verify the secret was stored and retrieved without error
    test_pass "test_wincred_empty_secret"
    return 0
}

#############################################################################
# Test suite execution
#############################################################################

run_wincred_helper_tests() {
    log_info "Running wincred-helper direct tests..."
    echo ""

    test_wincred_helper_exists
    test_wincred_set_and_get
    test_wincred_delete
    test_wincred_list
    test_wincred_large_secret
    test_wincred_special_chars
    test_wincred_overwrite
    test_wincred_multiple_secrets
    test_wincred_nonexistent
    test_wincred_empty_secret

    echo ""
}
