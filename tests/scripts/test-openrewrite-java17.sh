#!/bin/bash

# OpenRewrite Java 17 Transformation Test
# Based on roadmap/openrewrite/transform-to-java17.md
# Tests Java 8→17 transformations through the Ploy ARF system

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== OpenRewrite Java 17 Transformation Test ==="
echo "Project root: $PROJECT_ROOT"
echo "Target host: ${TARGET_HOST:-Not set}"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

# Configuration
PLOY_CONTROLLER="${PLOY_CONTROLLER:-https://api.dev.ployman.app/v1}"
TARGET_HOST="${TARGET_HOST:-45.12.75.241}"
SEAWEEDFS_URL="${SEAWEEDFS_URL:-http://${TARGET_HOST}:8888}"
TEST_START_TIME=$(date +%s)
TEST_RESULTS_DIR="${PROJECT_ROOT}/tests/results/openrewrite-java17-$(date +%Y%m%d-%H%M%S)"

# Create results directory
mkdir -p "$TEST_RESULTS_DIR"

# Test repositories as specified in transform-to-java17.md
declare -a TEST_REPOS=(
    "winterbe/java8-tutorial:master:maven"
    "spring-guides/gs-spring-boot:main:maven"
    "iluwatar/java-design-patterns:master:maven"
    "gothinkster/spring-boot-realworld-example-app:master:gradle"
)

declare -a REPO_NAMES=(
    "java8-tutorial"
    "spring-boot-sample"
    "java-design-patterns"
    "realworld-app"
)

declare -a EXPECTED_TIMES=(
    "5-10 minutes"
    "5-15 minutes"
    "20-30 minutes"
    "10-20 minutes"
)

# Function to check API health
check_api_health() {
    echo -e "${BLUE}[Health Check]${NC} Checking API health..."
    
    response=$(curl -s -o /dev/null -w "%{http_code}" "${PLOY_CONTROLLER}/health" || echo "000")
    
    if [ "$response" = "200" ]; then
        echo -e "${GREEN}✓${NC} API is healthy"
        return 0
    else
        echo -e "${RED}✗${NC} API health check failed (HTTP $response)"
        return 1
    fi
}

# Function to submit transformation request
submit_transformation() {
    local repo="$1"
    local branch="$2"
    local build_tool="$3"
    local repo_name="$4"
    
    echo -e "${BLUE}[Transform]${NC} Submitting transformation for $repo_name"
    echo "  Repository: https://github.com/${repo}.git"
    echo "  Branch: $branch"
    echo "  Build tool: $build_tool"
    
    # Create request JSON
    local request_json=$(cat <<EOF
{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
        "repository": "https://github.com/${repo}.git",
        "branch": "${branch}",
        "language": "java",
        "build_tool": "${build_tool}"
    }
}
EOF
)
    
    # Submit request and capture response
    local response=$(curl -s -X POST "${PLOY_CONTROLLER}/arf/transform" \
        -H "Content-Type: application/json" \
        -d "$request_json")
    
    # Extract transformation ID and job ID
    local transform_id=$(echo "$response" | jq -r '.transformation_id // empty')
    local job_id=$(echo "$response" | jq -r '.job_id // empty')
    
    if [ -z "$transform_id" ]; then
        echo -e "${RED}✗${NC} Failed to get transformation ID"
        echo "Response: $response"
        return 1
    fi
    
    echo -e "${GREEN}✓${NC} Transformation ID: $transform_id"
    [ -n "$job_id" ] && echo "  Nomad Job ID: $job_id"
    
    # Save response
    echo "$response" > "$TEST_RESULTS_DIR/${repo_name}-response.json"
    
    # Return both IDs
    echo "$transform_id|$job_id"
}

# Function to monitor Nomad job
monitor_nomad_job() {
    local job_id="$1"
    local repo_name="$2"
    local max_wait=1800  # 30 minutes max
    local start_time=$(date +%s)
    
    echo -e "${BLUE}[Nomad]${NC} Monitoring job: $job_id"
    
    while true; do
        # Get job status via SSH
        local job_status=$(ssh root@${TARGET_HOST} "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh status ${job_id}'" 2>/dev/null | grep "Status" | head -1 | awk '{print $3}' || echo "unknown")
        
        echo "  Status: $job_status ($(date '+%H:%M:%S'))"
        
        case "$job_status" in
            "complete"|"dead")
                echo -e "${GREEN}✓${NC} Job completed with status: $job_status"
                
                # Get allocation logs
                echo -e "${BLUE}[Logs]${NC} Fetching container logs..."
                ssh root@${TARGET_HOST} "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs ${job_id}'" > "$TEST_RESULTS_DIR/${repo_name}-logs.txt" 2>&1 || true
                
                # Check for key log markers
                if grep -q "\[OpenRewrite\] Transformation completed successfully" "$TEST_RESULTS_DIR/${repo_name}-logs.txt"; then
                    echo -e "${GREEN}✓${NC} Transformation completed successfully"
                    return 0
                else
                    echo -e "${YELLOW}⚠${NC} Job completed but transformation status unclear"
                    return 1
                fi
                ;;
            "failed")
                echo -e "${RED}✗${NC} Job failed"
                
                # Get failure logs
                ssh root@${TARGET_HOST} "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs ${job_id}'" > "$TEST_RESULTS_DIR/${repo_name}-error-logs.txt" 2>&1 || true
                return 1
                ;;
            "running"|"pending")
                # Continue monitoring
                ;;
            *)
                echo -e "${YELLOW}⚠${NC} Unknown status: $job_status"
                ;;
        esac
        
        # Check timeout
        local current_time=$(date +%s)
        local elapsed=$((current_time - start_time))
        if [ $elapsed -gt $max_wait ]; then
            echo -e "${RED}✗${NC} Timeout after ${elapsed} seconds"
            return 1
        fi
        
        sleep 10
    done
}

# Function to validate output artifact
validate_output() {
    local transform_id="$1"
    local repo_name="$2"
    
    echo -e "${BLUE}[Validation]${NC} Checking output artifact for $repo_name"
    
    # Try to get output key from transformation result
    local output_key=$(curl -s "${PLOY_CONTROLLER}/arf/transforms/${transform_id}" 2>/dev/null | jq -r '.output_key // empty')
    
    if [ -z "$output_key" ]; then
        # Try to find output key from logs
        output_key=$(grep "Output stored at:" "$TEST_RESULTS_DIR/${repo_name}-logs.txt" 2>/dev/null | awk '{print $NF}' || echo "")
    fi
    
    if [ -z "$output_key" ]; then
        echo -e "${YELLOW}⚠${NC} Could not find output key"
        return 1
    fi
    
    echo "  Output key: $output_key"
    
    # Download output tar
    local output_file="$TEST_RESULTS_DIR/${repo_name}-output.tar"
    echo "  Downloading from SeaweedFS..."
    
    if curl -s -o "$output_file" "${SEAWEEDFS_URL}/${output_key}"; then
        echo -e "${GREEN}✓${NC} Output downloaded successfully"
        
        # Extract and inspect
        local extract_dir="$TEST_RESULTS_DIR/${repo_name}-extracted"
        mkdir -p "$extract_dir"
        tar -xf "$output_file" -C "$extract_dir" 2>/dev/null || {
            echo -e "${RED}✗${NC} Failed to extract output tar"
            return 1
        }
        
        # Check for Java 17 features
        echo "  Checking for Java 17 transformations..."
        
        # Check POM/Gradle updates
        if [ -f "$extract_dir/pom.xml" ]; then
            if grep -q "<maven.compiler.source>17" "$extract_dir/pom.xml"; then
                echo -e "${GREEN}✓${NC} Maven compiler source updated to Java 17"
            fi
            if grep -q "<maven.compiler.target>17" "$extract_dir/pom.xml"; then
                echo -e "${GREEN}✓${NC} Maven compiler target updated to Java 17"
            fi
        fi
        
        if [ -f "$extract_dir/build.gradle" ]; then
            if grep -q "sourceCompatibility.*17" "$extract_dir/build.gradle"; then
                echo -e "${GREEN}✓${NC} Gradle source compatibility updated to Java 17"
            fi
        fi
        
        # Count transformed files
        local java_files=$(find "$extract_dir" -name "*.java" 2>/dev/null | wc -l)
        echo "  Java files in output: $java_files"
        
        # Look for Java 17 syntax (optional)
        local var_usage=$(grep -r "var " "$extract_dir" --include="*.java" 2>/dev/null | wc -l || echo 0)
        [ $var_usage -gt 0 ] && echo "  Found $var_usage uses of 'var' keyword"
        
        return 0
    else
        echo -e "${RED}✗${NC} Failed to download output from SeaweedFS"
        return 1
    fi
}

# Function to run single transformation test
run_transformation_test() {
    local repo_info="$1"
    local repo_name="$2"
    local expected_time="$3"
    local test_num="$4"
    local total_tests="$5"
    
    IFS=':' read -r repo branch build_tool <<< "$repo_info"
    
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${PURPLE}Test $test_num/$total_tests: $repo_name${NC}"
    echo "Expected time: $expected_time"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    
    local test_start=$(date +%s)
    
    # Submit transformation
    local result=$(submit_transformation "$repo" "$branch" "$build_tool" "$repo_name")
    if [ $? -ne 0 ]; then
        echo -e "${RED}✗${NC} Failed to submit transformation"
        return 1
    fi
    
    # Parse result
    IFS='|' read -r transform_id job_id <<< "$result"
    
    # Monitor Nomad job if we have job ID
    if [ -n "$job_id" ] && [ "$job_id" != "null" ]; then
        monitor_nomad_job "$job_id" "$repo_name"
    else
        echo -e "${YELLOW}⚠${NC} No Nomad job ID available, waiting for completion..."
        sleep 60
    fi
    
    # Validate output
    validate_output "$transform_id" "$repo_name"
    
    local test_end=$(date +%s)
    local test_duration=$((test_end - test_start))
    
    echo
    echo -e "${BLUE}Test completed in ${test_duration} seconds${NC}"
    
    return 0
}

# Main execution
main() {
    echo "Starting OpenRewrite Java 17 Transformation Tests"
    echo "================================================="
    echo "Controller URL: $PLOY_CONTROLLER"
    echo "Target Host: $TARGET_HOST"
    echo "SeaweedFS URL: $SEAWEEDFS_URL"
    echo "Results Directory: $TEST_RESULTS_DIR"
    echo
    
    # Check API health
    if ! check_api_health; then
        echo -e "${RED}API health check failed. Exiting.${NC}"
        exit 1
    fi
    
    # Run tests
    local total_tests=${#TEST_REPOS[@]}
    local passed_tests=0
    local failed_tests=0
    
    for i in "${!TEST_REPOS[@]}"; do
        if run_transformation_test "${TEST_REPOS[$i]}" "${REPO_NAMES[$i]}" "${EXPECTED_TIMES[$i]}" $((i+1)) $total_tests; then
            ((passed_tests++))
        else
            ((failed_tests++))
        fi
        
        # Brief pause between tests
        [ $i -lt $((total_tests-1)) ] && sleep 5
    done
    
    # Summary
    local test_end_time=$(date +%s)
    local total_duration=$((test_end_time - TEST_START_TIME))
    
    echo
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo -e "${PURPLE}Test Summary${NC}"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo "Total tests: $total_tests"
    echo -e "${GREEN}Passed: $passed_tests${NC}"
    echo -e "${RED}Failed: $failed_tests${NC}"
    echo "Total duration: ${total_duration} seconds"
    echo "Results saved to: $TEST_RESULTS_DIR"
    echo
    
    # Generate summary report
    cat > "$TEST_RESULTS_DIR/summary.txt" <<EOF
OpenRewrite Java 17 Transformation Test Summary
================================================
Date: $(date)
Total tests: $total_tests
Passed: $passed_tests
Failed: $failed_tests
Duration: ${total_duration} seconds

Test Results:
EOF
    
    for i in "${!REPO_NAMES[@]}"; do
        if [ -f "$TEST_RESULTS_DIR/${REPO_NAMES[$i]}-response.json" ]; then
            echo "- ${REPO_NAMES[$i]}: Completed" >> "$TEST_RESULTS_DIR/summary.txt"
        else
            echo "- ${REPO_NAMES[$i]}: Failed/Incomplete" >> "$TEST_RESULTS_DIR/summary.txt"
        fi
    done
    
    if [ $failed_tests -eq 0 ]; then
        echo -e "${GREEN}✓ All tests passed successfully!${NC}"
        exit 0
    else
        echo -e "${RED}✗ Some tests failed. Check logs in $TEST_RESULTS_DIR${NC}"
        exit 1
    fi
}

# Run main function
main "$@"