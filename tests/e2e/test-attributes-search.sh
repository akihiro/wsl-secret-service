#!/bin/bash
# SPDX-License-Identifier: Apache-2.0
# Attribute-based search tests

test_search_by_single_attribute() {
    test_start "test_search_by_single_attribute"

    # Store a secret with attributes
    printf 'pass1' | secret-tool store \
        --label "Search Test 1" \
        service github.com \
        user alice \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_search_by_single_attribute" "Failed to store secret"
        return 1
    fi

    # Search by single attribute
    local result=$(secret-tool search service github.com 2>/dev/null)

    if [ -n "$result" ]; then
        test_pass "test_search_by_single_attribute"
        return 0
    fi

    test_fail "test_search_by_single_attribute" "Single attribute search returned no results"
    return 1
}

test_search_by_multiple_attributes() {
    test_start "test_search_by_multiple_attributes"

    # Store multiple secrets with overlapping attributes
    printf 'pass1' | secret-tool store \
        --label "Multi Search 1" \
        service github.com \
        user alice \
        &>/dev/null

    printf 'pass2' | secret-tool store \
        --label "Multi Search 2" \
        service github.com \
        user bob \
        &>/dev/null

    printf 'pass3' | secret-tool store \
        --label "Multi Search 3" \
        service gitlab.com \
        user alice \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_search_by_multiple_attributes" "Failed to store secrets"
        return 1
    fi

    # Search with multiple attributes (AND condition)
    # Should only match the first secret
    local result=$(secret-tool search service github.com user alice 2>/dev/null | head -1)

    if [ "$result" = "Multi Search 1" ]; then
        # Verify only one result matches
        local count=$(secret-tool search service github.com user alice 2>/dev/null | wc -l)
        if [ $count -eq 2 ]; then  # Label line + one empty line
            test_pass "test_search_by_multiple_attributes"
            return 0
        fi
    fi

    test_fail "test_search_by_multiple_attributes" "Multi-attribute search failed"
    return 1
}

test_empty_search_returns_all() {
    test_start "test_empty_search_returns_all"

    # Store multiple distinct secrets
    printf 'secret1' | secret-tool store \
        --label "List All 1" \
        service service1.com \
        &>/dev/null

    printf 'secret2' | secret-tool store \
        --label "List All 2" \
        service service2.com \
        &>/dev/null

    printf 'secret3' | secret-tool store \
        --label "List All 3" \
        service service3.com \
        &>/dev/null

    # Search with --all flag (no attributes)
    local result=$(secret-tool search --all 2>/dev/null)

    if [ -n "$result" ]; then
        # Should contain all three labels
        if echo "$result" | grep -q "List All 1" && \
           echo "$result" | grep -q "List All 2" && \
           echo "$result" | grep -q "List All 3"; then
            test_pass "test_empty_search_returns_all"
            return 0
        fi
    fi

    test_fail "test_empty_search_returns_all" "Empty search did not return all secrets"
    return 1
}

test_search_no_match_returns_empty() {
    test_start "test_search_no_match_returns_empty"

    # Store a secret
    printf 'secret' | secret-tool store \
        --label "No Match Test" \
        service existing.com \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_search_no_match_returns_empty" "Failed to store secret"
        return 1
    fi

    # Search for non-existent attribute value
    local result=$(secret-tool search service nonexistent.com 2>/dev/null | wc -l)

    # Should return 0 or 1 (empty result)
    if [ $result -le 1 ]; then
        test_pass "test_search_no_match_returns_empty"
        return 0
    fi

    test_fail "test_search_no_match_returns_empty" "Search returned results for non-existent attribute"
    return 1
}

test_search_within_collection() {
    test_start "test_search_within_collection"

    # Store secrets
    printf 'pwd1' | secret-tool store \
        --label "Collection Search 1" \
        service col.test \
        user user1 \
        &>/dev/null

    printf 'pwd2' | secret-tool store \
        --label "Collection Search 2" \
        service col.test \
        user user2 \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_search_within_collection" "Failed to store secrets"
        return 1
    fi

    # Search should find both (both in default login collection)
    local result=$(secret-tool search service col.test 2>/dev/null)

    if echo "$result" | grep -q "Collection Search 1" && \
       echo "$result" | grep -q "Collection Search 2"; then
        test_pass "test_search_within_collection"
        return 0
    fi

    test_fail "test_search_within_collection" "Collection search failed"
    return 1
}

test_case_sensitive_attribute_search() {
    test_start "test_case_sensitive_attribute_search"

    # Store secrets with different case variations
    printf 'secret1' | secret-tool store \
        --label "Case Sensitive 1" \
        domain GitHub.com \
        &>/dev/null

    printf 'secret2' | secret-tool store \
        --label "Case Sensitive 2" \
        domain github.com \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_case_sensitive_attribute_search" "Failed to store secrets"
        return 1
    fi

    # Search with exact case
    local result_upper=$(secret-tool search domain GitHub.com 2>/dev/null | head -1)
    local result_lower=$(secret-tool search domain github.com 2>/dev/null | head -1)

    # Both searches should work and return their respective results
    if [ -n "$result_upper" ] && [ -n "$result_lower" ]; then
        # Verify they match the stored secrets
        if [ "$result_upper" = "Case Sensitive 1" ] && [ "$result_lower" = "Case Sensitive 2" ]; then
            test_pass "test_case_sensitive_attribute_search"
            return 0
        fi
    fi

    test_fail "test_case_sensitive_attribute_search" "Case sensitivity test failed"
    debug_dump "Upper result: $result_upper"
    debug_dump "Lower result: $result_lower"
    return 1
}

test_search_with_empty_attribute_value() {
    test_start "test_search_with_empty_attribute_value"

    # Store secrets with empty attribute values
    printf 'secret1' | secret-tool store \
        --label "Empty Attr 1" \
        service empty.test \
        tag "" \
        &>/dev/null

    printf 'secret2' | secret-tool store \
        --label "Empty Attr 2" \
        service empty.test \
        tag "nonempty" \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_search_with_empty_attribute_value" "Failed to store secrets"
        return 1
    fi

    # Both should be searchable by service
    local result=$(secret-tool search service empty.test 2>/dev/null)

    if echo "$result" | grep -q "Empty Attr" ; then
        test_pass "test_search_with_empty_attribute_value"
        return 0
    fi

    test_fail "test_search_with_empty_attribute_value" "Search with empty attributes failed"
    return 1
}

test_search_numeric_attribute_values() {
    test_start "test_search_numeric_attribute_values"

    # Store secrets with numeric attribute values
    printf 'secret1' | secret-tool store \
        --label "Numeric 1" \
        port 8080 \
        protocol https \
        &>/dev/null

    printf 'secret2' | secret-tool store \
        --label "Numeric 2" \
        port 3306 \
        protocol https \
        &>/dev/null

    if [ $? -ne 0 ]; then
        test_fail "test_search_numeric_attribute_values" "Failed to store secrets"
        return 1
    fi

    # Search by numeric port value (as string)
    local result=$(secret-tool search port 8080 2>/dev/null | head -1)

    if [ "$result" = "Numeric 1" ]; then
        test_pass "test_search_numeric_attribute_values"
        return 0
    fi

    test_fail "test_search_numeric_attribute_values" "Numeric attribute search failed"
    return 1
}

# Run all tests
run_attribute_search_tests() {
    log_info "Running attribute search tests..."
    echo ""

    test_search_by_single_attribute
    test_search_by_multiple_attributes
    test_empty_search_returns_all
    test_search_no_match_returns_empty
    test_search_within_collection
    test_case_sensitive_attribute_search
    test_search_with_empty_attribute_value
    test_search_numeric_attribute_values

    echo ""
}
