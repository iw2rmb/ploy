#!/bin/bash

# OpenRewrite Transformation Test Script
# This script tests various OpenRewrite recipes on test repositories

set -e

API_URL="https://api.dev.ployman.app"
RESULTS_FILE="/tmp/openrewrite-test-results.json"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== OpenRewrite Transformation Test Suite ===${NC}"
echo "Testing ARF transformations with actual code rewrites"
echo ""

# Test repositories (using simple variables for compatibility)
REPO_JAVA="https://github.com/iw2rmb/ploy-orw-test-java.git"
REPO_LEGACY="https://github.com/iw2rmb/ploy-orw-test-legacy.git"
REPO_SPRING="https://github.com/iw2rmb/ploy-orw-test-spring.git"

# Test recipes
RECIPE_CLEANUP="org.openrewrite.java.cleanup.RemoveUnusedImports"
RECIPE_JAVA11="org.openrewrite.java.migrate.Java8toJava11"
RECIPE_JAVA17="org.openrewrite.java.migrate.UpgradeToJava17"
RECIPE_SPRINGBOOT="org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_2"

# Function to run transformation
run_transformation() {
    local repo_name=$1
    local repo_url=$2
    local recipe_name=$3
    local recipe_id=$4
    
    echo -e "${YELLOW}Testing:${NC} $recipe_name on $repo_name repository"
    
    # Start transformation
    response=$(curl -s -X POST "$API_URL/v1/arf/transforms" \
        -H "Content-Type: application/json" \
        -d "{
            \"recipe_id\": \"$recipe_id\",
            \"type\": \"openrewrite\",
            \"codebase\": {
                \"repository\": \"$repo_url\",
                \"branch\": \"main\"
            }
        }")
    
    transform_id=$(echo "$response" | jq -r '.transformation_id')
    
    if [ "$transform_id" = "null" ]; then
        echo -e "${RED}Failed to start transformation${NC}"
        echo "$response"
        return 1
    fi
    
    echo "  Transform ID: $transform_id"
    
    # Poll for completion
    max_attempts=30
    attempt=0
    
    while [ $attempt -lt $max_attempts ]; do
        sleep 2
        status=$(curl -s "$API_URL/v1/arf/transforms/$transform_id/status" | jq -r '.status')
        
        if [ "$status" = "completed" ]; then
            echo -e "  Status: ${GREEN}COMPLETED${NC}"
            
            # Immediately capture transformation details
            details=$(curl -s "$API_URL/v1/arf/transforms/$transform_id")
            
            if [ "$(echo "$details" | jq -r '.error')" != "null" ]; then
                # Try alternate endpoint
                details=$(curl -s "$API_URL/v1/arf/transforms/$transform_id/status")
            fi
            
            # Save results
            echo "{
                \"repo\": \"$repo_name\",
                \"recipe\": \"$recipe_name\",
                \"transform_id\": \"$transform_id\",
                \"timestamp\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",
                \"details\": $details
            }" >> "$RESULTS_FILE"
            
            # Check for diff
            diff=$(echo "$details" | jq -r '.diff // "No diff available"')
            if [ "$diff" != "No diff available" ] && [ "$diff" != "null" ]; then
                echo -e "  Changes: ${GREEN}Code modifications detected${NC}"
                echo "  Diff preview:"
                echo "$diff" | head -20
            else
                echo -e "  Changes: ${YELLOW}No diff captured${NC}"
            fi
            
            return 0
        elif [ "$status" = "failed" ] || [ "$status" = "error" ]; then
            echo -e "  Status: ${RED}FAILED${NC}"
            curl -s "$API_URL/v1/arf/transforms/$transform_id/status" | jq '.error'
            return 1
        fi
        
        attempt=$((attempt + 1))
        echo -n "."
    done
    
    echo -e "\n  ${RED}Timeout waiting for transformation${NC}"
    return 1
}

# Initialize results file
echo "[]" > "$RESULTS_FILE"

# Run tests
echo -e "\n${GREEN}Starting transformation tests...${NC}\n"

# Test 1: Remove unused imports on Java project
run_transformation "java" "$REPO_JAVA" "RemoveUnusedImports" "$RECIPE_CLEANUP"
echo ""

# Test 2: Java 17 upgrade on legacy project  
run_transformation "legacy" "$REPO_LEGACY" "UpgradeToJava17" "$RECIPE_JAVA17"
echo ""

# Test 3: Spring Boot upgrade
run_transformation "spring" "$REPO_SPRING" "SpringBoot3.2" "$RECIPE_SPRINGBOOT"
echo ""

# Test 4: Java 11 migration on Java project
run_transformation "java" "$REPO_JAVA" "Java8toJava11" "$RECIPE_JAVA11"
echo ""

# Summary
echo -e "\n${GREEN}=== Test Summary ===${NC}"
total_tests=$(cat "$RESULTS_FILE" | jq '. | length')
echo "Total tests run: $total_tests"

# Count successful transformations with diffs
with_diffs=$(cat "$RESULTS_FILE" | jq '[.[] | select(.details.diff != null and .details.diff != "null")] | length')
echo "Transformations with verified code changes: $with_diffs"

echo -e "\nResults saved to: $RESULTS_FILE"
echo ""

# Display all transformation IDs for reference
echo -e "${GREEN}Transformation IDs for manual verification:${NC}"
cat "$RESULTS_FILE" | jq -r '.[] | "  \(.recipe): \(.transform_id)"'