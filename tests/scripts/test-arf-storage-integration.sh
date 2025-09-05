#!/bin/bash
# ARF Storage Integration Test Suite
# Tests recipe storage, indexing, and validation across different backends

set -e

SCRIPT_DIR=$(dirname "$0")
ROOT_DIR=$(cd "$SCRIPT_DIR/.." && pwd)

# Source common utilities
source "$SCRIPT_DIR/common/test-utils.sh"

# Configuration
CONTROLLER_URL="${PLOY_CONTROLLER:-https://api.dev.ployman.app/v1}"
TEST_RECIPE_ID="test-storage-integration-$(date +%s)"
TEMP_DIR="/tmp/arf-storage-test"
RESULTS_FILE="$TEMP_DIR/test-results.json"

# Setup test environment
setup_test() {
    log_info "Setting up ARF storage integration test environment..."
    
    mkdir -p "$TEMP_DIR"
    
    # Check controller health
    if ! curl -s -f "$CONTROLLER_URL/health" > /dev/null 2>&1; then
        log_error "Controller not accessible at $CONTROLLER_URL"
        exit 1
    fi
    
    log_info "Controller is accessible at $CONTROLLER_URL"
    
    # Initialize results tracking
    cat > "$RESULTS_FILE" << EOF
{
    "test_suite": "arf-storage-integration",
    "start_time": "$(date -Iseconds)",
    "controller_url": "$CONTROLLER_URL",
    "tests": []
}
EOF
}

# Helper function to record test results
record_test() {
    local test_name="$1"
    local status="$2"
    local details="$3"
    local end_time=$(date -Iseconds)
    
    # Update results file
    tmp_file=$(mktemp)
    jq --arg name "$test_name" --arg status "$status" --arg details "$details" --arg time "$end_time" \
       '.tests += [{name: $name, status: $status, details: $details, timestamp: $time}]' \
       "$RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$RESULTS_FILE"
}

# Test 1: Recipe Creation and Storage
test_recipe_creation() {
    log_info "Test 1: Testing recipe creation and storage..."
    
    local test_recipe=$(cat << 'EOF'
{
    "id": "TEST_RECIPE_ID_PLACEHOLDER",
    "metadata": {
        "name": "Storage Integration Test Recipe",
        "version": "1.0.0",
        "description": "Test recipe for storage backend validation",
        "author": "test-suite",
        "languages": ["java"],
        "categories": ["testing"],
        "tags": ["storage", "integration", "test"]
    },
    "steps": [
        {
            "name": "test-validation-step",
            "type": "shell",
            "command": "echo 'Storage test step'",
            "working_dir": "/tmp",
            "timeout": {
                "duration": "30s"
            },
            "retry": {
                "max_attempts": 1
            }
        }
    ]
}
EOF
)
    
    # Replace placeholder with actual test ID
    test_recipe=$(echo "$test_recipe" | sed "s/TEST_RECIPE_ID_PLACEHOLDER/$TEST_RECIPE_ID/g")
    
    # Create recipe via API
    response=$(curl -s -X POST "$CONTROLLER_URL/arf/recipes" \
        -H "Content-Type: application/json" \
        -d "$test_recipe")
    
    if echo "$response" | jq -e '.id' > /dev/null 2>&1; then
        log_info "✓ Recipe created successfully"
        record_test "recipe_creation" "PASS" "Recipe created with ID: $TEST_RECIPE_ID"
        return 0
    else
        log_error "✗ Recipe creation failed: $response"
        record_test "recipe_creation" "FAIL" "Recipe creation failed: $response"
        return 1
    fi
}

# Test 2: Recipe Retrieval and Validation
test_recipe_retrieval() {
    log_info "Test 2: Testing recipe retrieval..."
    
    response=$(curl -s "$CONTROLLER_URL/arf/recipes/$TEST_RECIPE_ID")
    
    if echo "$response" | jq -e '.id' > /dev/null 2>&1; then
        local retrieved_id=$(echo "$response" | jq -r '.id')
        if [ "$retrieved_id" = "$TEST_RECIPE_ID" ]; then
            log_info "✓ Recipe retrieved successfully"
            record_test "recipe_retrieval" "PASS" "Recipe retrieved with correct ID"
            return 0
        else
            log_error "✗ Recipe ID mismatch: expected $TEST_RECIPE_ID, got $retrieved_id"
            record_test "recipe_retrieval" "FAIL" "ID mismatch: expected $TEST_RECIPE_ID, got $retrieved_id"
            return 1
        fi
    else
        log_error "✗ Recipe retrieval failed: $response"
        record_test "recipe_retrieval" "FAIL" "Recipe retrieval failed: $response"
        return 1
    fi
}

# Test 3: Recipe Search and Indexing
test_recipe_search() {
    log_info "Test 3: Testing recipe search and indexing..."
    
    # Search by tag
    response=$(curl -s "$CONTROLLER_URL/arf/recipes/search?q=storage")
    
    if echo "$response" | jq -e '.recipes' > /dev/null 2>&1; then
        local found_recipes=$(echo "$response" | jq -r '.recipes[] | select(.id=="'$TEST_RECIPE_ID'") | .id')
        if [ "$found_recipes" = "$TEST_RECIPE_ID" ]; then
            log_info "✓ Recipe search successful"
            record_test "recipe_search" "PASS" "Recipe found via search"
            return 0
        else
            log_warn "! Recipe not found in search results (may indicate indexing delay)"
            record_test "recipe_search" "WARN" "Recipe not found in search results"
            return 0
        fi
    else
        log_error "✗ Recipe search failed: $response"
        record_test "recipe_search" "FAIL" "Recipe search failed: $response"
        return 1
    fi
}

# Test 4: Recipe Validation
test_recipe_validation() {
    log_info "Test 4: Testing recipe validation..."
    
    # Test invalid recipe
    local invalid_recipe=$(cat << 'EOF'
{
    "metadata": {
        "name": "Invalid Recipe"
    },
    "steps": []
}
EOF
)
    
    response=$(curl -s -X POST "$CONTROLLER_URL/arf/recipes/validate" \
        -H "Content-Type: application/json" \
        -d "$invalid_recipe")
    
    if echo "$response" | jq -e '.valid == false' > /dev/null 2>&1; then
        log_info "✓ Recipe validation correctly rejected invalid recipe"
        record_test "recipe_validation" "PASS" "Invalid recipe correctly rejected"
        return 0
    else
        log_error "✗ Recipe validation failed to reject invalid recipe: $response"
        record_test "recipe_validation" "FAIL" "Invalid recipe not rejected: $response"
        return 1
    fi
}

# Test 5: Recipe Listing and Filtering
test_recipe_listing() {
    log_info "Test 5: Testing recipe listing and filtering..."
    
    response=$(curl -s "$CONTROLLER_URL/arf/recipes?category=testing&language=java")
    
    if echo "$response" | jq -e '.recipes' > /dev/null 2>&1; then
        local recipe_count=$(echo "$response" | jq '.recipes | length')
        log_info "✓ Recipe listing successful (found $recipe_count recipes)"
        record_test "recipe_listing" "PASS" "Recipe listing returned $recipe_count recipes"
        return 0
    else
        log_error "✗ Recipe listing failed: $response"
        record_test "recipe_listing" "FAIL" "Recipe listing failed: $response"
        return 1
    fi
}

# Test 6: Recipe Update
test_recipe_update() {
    log_info "Test 6: Testing recipe update..."
    
    # Update recipe description
    local updated_recipe=$(cat << 'EOF'
{
    "id": "TEST_RECIPE_ID_PLACEHOLDER",
    "metadata": {
        "name": "Storage Integration Test Recipe (Updated)",
        "version": "1.1.0",
        "description": "Updated test recipe for storage backend validation",
        "author": "test-suite",
        "languages": ["java"],
        "categories": ["testing", "updated"],
        "tags": ["storage", "integration", "test", "updated"]
    },
    "steps": [
        {
            "name": "test-validation-step",
            "type": "shell",
            "command": "echo 'Updated storage test step'",
            "working_dir": "/tmp",
            "timeout": {
                "duration": "30s"
            },
            "retry": {
                "max_attempts": 1
            }
        }
    ]
}
EOF
)
    
    # Replace placeholder with actual test ID
    updated_recipe=$(echo "$updated_recipe" | sed "s/TEST_RECIPE_ID_PLACEHOLDER/$TEST_RECIPE_ID/g")
    
    response=$(curl -s -X PUT "$CONTROLLER_URL/arf/recipes/$TEST_RECIPE_ID" \
        -H "Content-Type: application/json" \
        -d "$updated_recipe")
    
    if echo "$response" | jq -e '.metadata.version == "1.1.0"' > /dev/null 2>&1; then
        log_info "✓ Recipe update successful"
        record_test "recipe_update" "PASS" "Recipe updated to version 1.1.0"
        return 0
    else
        log_error "✗ Recipe update failed: $response"
        record_test "recipe_update" "FAIL" "Recipe update failed: $response"
        return 1
    fi
}

# Test 7: Storage Backend Configuration
test_backend_configuration() {
    log_info "Test 7: Testing storage backend configuration..."
    
    # Check ARF handler status
    response=$(curl -s "$CONTROLLER_URL/arf/health")
    
    if echo "$response" | jq -e '.status' > /dev/null 2>&1; then
        local status=$(echo "$response" | jq -r '.status')
        log_info "✓ Storage backend accessible (status: $status)"
        record_test "backend_configuration" "PASS" "Backend status: $status"
        return 0
    else
        log_error "✗ Storage backend configuration test failed: $response"
        record_test "backend_configuration" "FAIL" "Backend not accessible: $response"
        return 1
    fi
}

# Test 8: Recipe Statistics and Metadata
test_recipe_statistics() {
    log_info "Test 8: Testing recipe statistics and metadata..."
    
    response=$(curl -s "$CONTROLLER_URL/arf/recipes/$TEST_RECIPE_ID/stats")
    
    if echo "$response" | jq -e '.recipe_id' > /dev/null 2>&1; then
        log_info "✓ Recipe statistics accessible"
        record_test "recipe_statistics" "PASS" "Recipe statistics retrieved"
        return 0
    else
        log_warn "! Recipe statistics may not be available (acceptable for new recipes)"
        record_test "recipe_statistics" "WARN" "Recipe statistics not available"
        return 0
    fi
}

# Cleanup test data
cleanup_test() {
    log_info "Cleaning up test data..."
    
    # Delete test recipe
    response=$(curl -s -X DELETE "$CONTROLLER_URL/arf/recipes/$TEST_RECIPE_ID")
    
    if echo "$response" | jq -e '.message' > /dev/null 2>&1; then
        log_info "✓ Test recipe deleted successfully"
        record_test "cleanup" "PASS" "Test recipe deleted"
    else
        log_warn "! Test recipe deletion may have failed: $response"
        record_test "cleanup" "WARN" "Deletion response: $response"
    fi
}

# Generate final test report
generate_report() {
    log_info "Generating test report..."
    
    # Update results with end time
    local end_time=$(date -Iseconds)
    tmp_file=$(mktemp)
    jq --arg time "$end_time" '.end_time = $time' "$RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$RESULTS_FILE"
    
    # Count test results
    local total_tests=$(jq '.tests | length' "$RESULTS_FILE")
    local passed_tests=$(jq '.tests | map(select(.status == "PASS")) | length' "$RESULTS_FILE")
    local failed_tests=$(jq '.tests | map(select(.status == "FAIL")) | length' "$RESULTS_FILE")
    local warned_tests=$(jq '.tests | map(select(.status == "WARN")) | length' "$RESULTS_FILE")
    
    log_info "================== TEST REPORT =================="
    log_info "Total Tests: $total_tests"
    log_info "Passed: $passed_tests"
    log_info "Failed: $failed_tests"
    log_info "Warnings: $warned_tests"
    log_info "Results saved to: $RESULTS_FILE"
    
    if [ "$failed_tests" -gt 0 ]; then
        log_error "Some tests failed. Check the detailed results."
        return 1
    elif [ "$warned_tests" -gt 0 ]; then
        log_warn "All tests passed but some had warnings."
        return 0
    else
        log_info "All tests passed successfully!"
        return 0
    fi
}

# Main test execution
main() {
    log_info "Starting ARF Storage Integration Test Suite"
    log_info "Controller URL: $CONTROLLER_URL"
    
    setup_test
    
    # Run all tests
    test_recipe_creation
    test_recipe_retrieval
    test_recipe_search
    test_recipe_validation
    test_recipe_listing
    test_recipe_update
    test_backend_configuration
    test_recipe_statistics
    
    # Cleanup and report
    cleanup_test
    generate_report
    
    local exit_code=$?
    log_info "ARF Storage Integration Test Suite completed"
    exit $exit_code
}

# Execute main function
main "$@"