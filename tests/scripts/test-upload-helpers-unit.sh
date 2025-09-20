#!/usr/bin/env bash

# Unit test for enhanced upload helper functions
# Tests the uploadFileWithRetryAndVerification and uploadBytesWithRetryAndVerification functions

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0

log() {
    echo -e "${BLUE}[$(date '+%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
    ((PASSED_TESTS++))
}

warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
}

run_test() {
    local test_name="$1"
    ((TOTAL_TESTS++))
    log "Running test: $test_name"
}

test_upload_helper_function_compilation() {
    run_test "Upload helper functions compile correctly"
    
    # Test that the build handler compiles without errors
    if go build -o /tmp/test-build-handler ./internal/build/ 2>/dev/null; then
        success "Upload helper functions compile successfully"
    else
        error "Upload helper functions failed to compile"
        # Show compilation errors
        go build -o /tmp/test-build-handler ./internal/build/ 2>&1 || true
    fi
    
    # Clean up test binary
    rm -f /tmp/test-build-handler
}

test_build_handler_syntax() {
    run_test "Build handler syntax validation"
    
    # Use go vet to check for potential issues
    if go vet ./internal/build/ 2>/dev/null; then
        success "Build handler passes go vet checks"
    else
        warning "Build handler has potential issues detected by go vet"
        go vet ./internal/build/ 2>&1 || true
    fi
}

test_import_dependencies() {
    run_test "Upload helper function dependencies"
    
    # Check that required imports are available
    local required_imports=(
        "bytes"
        "fmt"
        "os"
        "path/filepath" 
        "time"
        "github.com/iw2rmb/ploy/internal/storage"
    )
    
    local missing_imports=0
    for import in "${required_imports[@]}"; do
        if ! grep -q "\"$import\"" internal/build/handler.go; then
            error "Missing required import: $import"
            ((missing_imports++))
        fi
    done
    
    if [[ $missing_imports -eq 0 ]]; then
        success "All required imports are present"
    else
        error "$missing_imports required imports are missing"
    fi
}

test_function_signatures() {
    run_test "Upload helper function signatures"
    
    # Check that the helper functions have correct signatures
    if grep -q "func uploadFileWithRetryAndVerification.*error" internal/build/handler.go; then
        success "uploadFileWithRetryAndVerification function signature is correct"
    else
        error "uploadFileWithRetryAndVerification function signature is missing or incorrect"
    fi
    
    if grep -q "func uploadBytesWithRetryAndVerification.*error" internal/build/handler.go; then
        success "uploadBytesWithRetryAndVerification function signature is correct"
    else
        error "uploadBytesWithRetryAndVerification function signature is missing or incorrect"
    fi
}

test_retry_logic_constants() {
    run_test "Retry logic constants validation"
    
    # Check that retry constants are properly defined
    if grep -q "const maxRetries = 3" internal/build/handler.go; then
        success "maxRetries constant is properly defined"
    else
        warning "maxRetries constant may not be set to expected value"
    fi
    
    if grep -q "const baseDelay = time.Second" internal/build/handler.go; then
        success "baseDelay constant is properly defined"
    else
        warning "baseDelay constant may not be set to expected value"
    fi
}

test_error_handling_patterns() {
    run_test "Error handling patterns in upload functions"
    
    # Check for proper error wrapping and logging
    if grep -q "fmt.Errorf.*failed to open file.*%w" internal/build/handler.go; then
        success "File open error handling uses proper error wrapping"
    else
        warning "File open error handling may not use proper error wrapping"
    fi
    
    if grep -q "Upload attempt.*failed:" internal/build/handler.go; then
        success "Upload attempt error logging is present"
    else
        warning "Upload attempt error logging may be missing"
    fi
    
    if grep -q "uploaded and verified.*successfully" internal/build/handler.go; then
        success "Success logging is present"
    else
        warning "Success logging may be missing"
    fi
}

test_storage_client_integration() {
    run_test "Storage client integration points"
    
    # Check that helper functions properly use storage client methods
    if grep -q "storeClient.PutObject" internal/build/handler.go; then
        success "Storage client PutObject integration is present"
    else
        error "Storage client PutObject integration is missing"
    fi
    
    if grep -q "storage.NewIntegrityVerifier" internal/build/handler.go; then
        success "Integrity verifier integration is present"
    else
        warning "Integrity verifier integration may be missing"
    fi
}

test_file_handling_patterns() {
    run_test "File handling patterns in upload functions"
    
    # Check for proper file handling
    if grep -q "os.Open.*filePath" internal/build/handler.go; then
        success "File opening pattern is correct"
    else
        warning "File opening pattern may be incorrect"
    fi
    
    if grep -q "f.Close()" internal/build/handler.go; then
        success "File closing pattern is present"
    else
        warning "File closing pattern may be missing"
    fi
    
    if grep -q "bytes.NewReader" internal/build/handler.go; then
        success "Bytes reader pattern is present for byte uploads"
    else
        warning "Bytes reader pattern may be missing"
    fi
}

test_enhanced_upload_usage() {
    run_test "Enhanced upload function usage in build handler"
    
    # Check that the new upload functions are actually used
    if grep -q "uploadFileWithRetryAndVerification.*source.sbom.json" internal/build/handler.go; then
        success "Enhanced file upload is used for source SBOM"
    else
        error "Enhanced file upload is not used for source SBOM"
    fi
    
    if grep -q "uploadFileWithRetryAndVerification.*container.sbom.json" internal/build/handler.go; then
        success "Enhanced file upload is used for container SBOM"
    else
        error "Enhanced file upload is not used for container SBOM"
    fi
    
    if grep -q "uploadBytesWithRetryAndVerification.*meta.json" internal/build/handler.go; then
        success "Enhanced bytes upload is used for metadata"
    else
        error "Enhanced bytes upload is not used for metadata"
    fi
}

check_build_handler_exists() {
    if [[ ! -f "internal/build/handler.go" ]]; then
        error "Build handler file not found at internal/build/handler.go"
        exit 1
    fi
    
    success "Build handler file exists"
}

main() {
    echo "Enhanced Upload Helper Functions Unit Test Suite"
    echo "=============================================="
    
    # Check prerequisites
    check_build_handler_exists
    
    # Run tests
    test_upload_helper_function_compilation
    test_build_handler_syntax
    test_import_dependencies
    test_function_signatures
    test_retry_logic_constants
    test_error_handling_patterns
    test_storage_client_integration
    test_file_handling_patterns
    test_enhanced_upload_usage
    
    # Report results
    echo ""
    echo "Unit Test Results Summary"
    echo "========================"
    echo "Total tests: $TOTAL_TESTS"
    echo "Passed: $PASSED_TESTS"
    echo "Failed: $((TOTAL_TESTS - PASSED_TESTS))"
    
    if [[ $PASSED_TESTS -eq $TOTAL_TESTS ]]; then
        success "All unit tests passed!"
        exit 0
    else
        error "Some unit tests failed!"
        exit 1
    fi
}

# Only run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
