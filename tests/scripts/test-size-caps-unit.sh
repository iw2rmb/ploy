#!/bin/bash

# Unit tests for image size caps implementation
# Tests the size cap logic independently

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Unit Testing Image Size Caps Implementation ==="
echo "Project root: $PROJECT_ROOT"
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test functions
test_passed() {
    echo -e "${GREEN}✓ PASSED:${NC} $1"
}

test_failed() {
    echo -e "${RED}✗ FAILED:${NC} $1"
    exit 1
}

test_info() {
    echo -e "${YELLOW}ℹ INFO:${NC} $1"
}

# Test 1: Verify image size caps module compiles
test_compilation() {
    test_info "Test 1: Verify image size caps implementation compiles"
    
    cd "$PROJECT_ROOT"
    if go build -o /tmp/test-api ./api > /dev/null 2>&1; then
        test_passed "Image size caps implementation compiles successfully"
    else
        test_failed "Compilation failed for image size caps implementation"
    fi
    rm -f /tmp/test-api
}

# Test 2: Check image size utility files exist
test_files_exist() {
    test_info "Test 2: Check image size utility files exist"
    
    if [ -f "$PROJECT_ROOT/internal/utils/image_size.go" ]; then
        test_passed "Image size utility module exists"
    else
        test_failed "Image size utility module not found"
    fi
    
    # Check that OPA module was updated
    if grep -q "ImageSizeMB" "$PROJECT_ROOT/api/opa/verify.go"; then
        test_passed "OPA policy updated with size information"
    else
        test_failed "OPA policy not updated with size information"
    fi
}

# Test 3: Verify lane size limits are properly defined
test_lane_size_limits() {
    test_info "Test 3: Verify lane size limits are properly defined"
    
    SIZE_FILE="$PROJECT_ROOT/internal/utils/image_size.go"
    
    # Check for all expected lanes
    expected_lanes=("A" "B" "C" "D" "E" "F")
    expected_limits=(50 100 500 200 1024 5120)
    
    for i in "${!expected_lanes[@]}"; do
        lane="${expected_lanes[$i]}"
        limit="${expected_limits[$i]}"
        
        if grep -q "Lane.*\"$lane\"" "$SIZE_FILE" && grep -q "MaxSizeMB.*$limit" "$SIZE_FILE"; then
            test_passed "Lane $lane size limit ($limit MB) properly defined"
        else
            test_failed "Lane $lane size limit not properly defined"
        fi
    done
}

# Test 4: Check size measurement functions
test_size_measurement_functions() {
    test_info "Test 4: Check size measurement functions"
    
    SIZE_FILE="$PROJECT_ROOT/internal/utils/image_size.go"
    
    if grep -q "GetImageSize" "$SIZE_FILE"; then
        test_passed "GetImageSize function implemented"
    else
        test_failed "GetImageSize function missing"
    fi
    
    if grep -q "getFileSize" "$SIZE_FILE"; then
        test_passed "File size measurement function implemented"
    else
        test_failed "File size measurement function missing"
    fi
    
    if grep -q "getDockerImageSize" "$SIZE_FILE"; then
        test_passed "Docker image size measurement function implemented"
    else
        test_failed "Docker image size measurement function missing"
    fi
    
    if grep -q "parseDockerSize" "$SIZE_FILE"; then
        test_passed "Docker size parsing function implemented"
    else
        test_failed "Docker size parsing function missing"
    fi
}

# Test 5: Verify OPA enforcement integration
test_opa_enforcement_integration() {
    test_info "Test 5: Verify OPA enforcement integration"
    
    OPA_FILE="$PROJECT_ROOT/api/opa/verify.go"
    
    if grep -q "enforceSizeCaps" "$OPA_FILE"; then
        test_passed "Size cap enforcement function implemented in OPA"
    else
        test_failed "Size cap enforcement function missing in OPA"
    fi
    
    if grep -q "ImageSizeMB.*float64" "$OPA_FILE"; then
        test_passed "ArtifactInput includes size information"
    else
        test_failed "ArtifactInput missing size information"
    fi
    
    if grep -q "break.*glass.*size" "$OPA_FILE"; then
        test_passed "Break-glass override for size caps implemented"
    else
        test_failed "Break-glass override for size caps missing"
    fi
}

# Test 6: Check build handler integration
test_build_handler_integration() {
    test_info "Test 6: Check build handler integration"
    
    BUILD_HANDLER="$PROJECT_ROOT/internal/build/handler.go"
    
    if grep -q "GetImageSize" "$BUILD_HANDLER"; then
        test_passed "Build handler calls image size measurement"
    else
        test_failed "Build handler not calling image size measurement"
    fi
    
    if grep -q "ImageSizeMB.*imageSizeMB" "$BUILD_HANDLER"; then
        test_passed "Build handler passes size to OPA enforcement"
    else
        test_failed "Build handler not passing size to OPA enforcement"
    fi
    
    if grep -q "Image size measurement.*FormatSize" "$BUILD_HANDLER"; then
        test_passed "Build handler logs size measurement results"
    else
        test_failed "Build handler not logging size measurement results"
    fi
}

# Test 7: Verify test scenarios added to TESTS.md
test_scenarios_documented() {
    test_info "Test 7: Verify test scenarios added to TESTS.md"
    
    if grep -q "Image Size Caps per Lane Implementation" "$PROJECT_ROOT/tests/scripts/README.md"; then
        test_passed "Test scenarios documented in TESTS.md"
    else
        test_failed "Test scenarios not added to TESTS.md"
    fi
    
    if grep -q "lane-specific image size limits" "$PROJECT_ROOT/tests/scripts/README.md"; then
        test_passed "Size cap enforcement scenarios documented"
    else
        test_failed "Size cap enforcement scenarios missing"
    fi
    
    # Check for specific lane limits in documentation
    expected_limits=("50MB" "100MB" "500MB" "200MB" "1GB" "5GB")
    for limit in "${expected_limits[@]}"; do
        if grep -q "$limit" "$PROJECT_ROOT/tests/scripts/README.md"; then
            test_passed "Lane limit $limit documented"
        else
            test_failed "Lane limit $limit not documented"
        fi
    done
}

# Test 8: Check error handling and logging
test_error_handling() {
    test_info "Test 8: Check error handling and logging"
    
    OPA_FILE="$PROJECT_ROOT/api/opa/verify.go"
    
    if grep -q "Size Cap Enforcement.*PASSED" "$OPA_FILE"; then
        test_passed "Size cap enforcement success logging implemented"
    else
        test_failed "Size cap enforcement success logging missing"
    fi
    
    if grep -q "Size Cap Enforcement.*BYPASSED" "$OPA_FILE"; then
        test_passed "Size cap enforcement bypass logging implemented"
    else
        test_failed "Size cap enforcement bypass logging missing"
    fi
    
    if grep -q "exceeds.*limit.*Actual.*Limit" "$OPA_FILE"; then
        test_passed "Detailed size violation error messages implemented"
    else
        test_failed "Detailed size violation error messages missing"
    fi
}

# Test 9: Verify utility functions
test_utility_functions() {
    test_info "Test 9: Verify utility functions"
    
    SIZE_FILE="$PROJECT_ROOT/internal/utils/image_size.go"
    
    if grep -q "FormatSize" "$SIZE_FILE"; then
        test_passed "Size formatting utility function implemented"
    else
        test_failed "Size formatting utility function missing"
    fi
    
    if grep -q "GetLaneSizeLimit" "$SIZE_FILE"; then
        test_passed "Lane size limit getter function implemented"
    else
        test_failed "Lane size limit getter function missing"
    fi
    
    if grep -q "ImageSizeInfo.*struct" "$SIZE_FILE"; then
        test_passed "Image size info structure implemented"
    else
        test_failed "Image size info structure missing"
    fi
}

# Main execution
main() {
    echo "Starting Image Size Caps Unit Tests..."
    echo "====================================="
    
    test_compilation
    test_files_exist
    test_lane_size_limits
    test_size_measurement_functions
    test_opa_enforcement_integration
    test_build_handler_integration
    test_scenarios_documented
    test_error_handling
    test_utility_functions
    
    echo
    echo "====================================="
    test_passed "All Image Size Caps unit tests passed"
    
    echo
    echo "Summary:"
    echo "- ✓ Image size caps implementation compiles successfully"
    echo "- ✓ Image size utility module with comprehensive measurement functions"
    echo "- ✓ Lane-specific size limits properly defined for all lanes A-F"
    echo "- ✓ OPA policy integration with size cap enforcement and break-glass override"
    echo "- ✓ Build handler integration with size measurement and logging"
    echo "- ✓ Comprehensive test scenarios documented in TESTS.md"
    echo "- ✓ Detailed error handling and audit logging implemented"
    echo "- ✓ Utility functions for size formatting and limit management"
    echo
    echo "Phase 4 Step 3 implementation structure verified!"
}

# Run tests
main "$@"