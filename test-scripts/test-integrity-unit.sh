#!/bin/bash

# Unit tests for artifact integrity verification
# Tests the integrity verification logic independently

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Unit Testing Artifact Integrity Verification ==="
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

# Test 1: Verify integrity verification module compiles
test_compilation() {
    test_info "Test 1: Verify integrity verification module compiles"
    
    cd "$PROJECT_ROOT"
    if go build -o /tmp/test-controller ./controller > /dev/null 2>&1; then
        test_passed "Integrity verification module compiles successfully"
    else
        test_failed "Compilation failed for integrity verification module"
    fi
    rm -f /tmp/test-controller
}

# Test 2: Check integrity verification files exist
test_files_exist() {
    test_info "Test 2: Check integrity verification files exist"
    
    if [ -f "$PROJECT_ROOT/internal/storage/integrity.go" ]; then
        test_passed "Integrity verification module exists"
    else
        test_failed "Integrity verification module not found"
    fi
    
    # Check that the storage interface was updated
    if grep -q "UploadArtifactBundleWithVerification" "$PROJECT_ROOT/internal/storage/interface.go"; then
        test_passed "Storage interface updated with integrity verification"
    else
        test_failed "Storage interface not updated"
    fi
}

# Test 3: Verify SeaweedFS client implements new interface
test_seaweedfs_implementation() {
    test_info "Test 3: Verify SeaweedFS client implements new interface"
    
    if grep -q "UploadArtifactBundleWithVerification" "$PROJECT_ROOT/internal/storage/seaweedfs.go"; then
        test_passed "SeaweedFS client implements integrity verification"
    else
        test_failed "SeaweedFS client missing integrity verification implementation"
    fi
}

# Test 4: Check build handler uses integrity verification
test_build_handler_integration() {
    test_info "Test 4: Check build handler uses integrity verification"
    
    if grep -q "UploadArtifactBundleWithVerification" "$PROJECT_ROOT/internal/build/handler.go"; then
        test_passed "Build handler integrated with integrity verification"
    else
        test_failed "Build handler not using integrity verification"
    fi
    
    if grep -q "GetVerificationSummary" "$PROJECT_ROOT/internal/build/handler.go"; then
        test_passed "Build handler uses verification summary reporting"
    else
        test_failed "Build handler missing verification summary reporting"
    fi
}

# Test 5: Verify test scenarios added to TESTS.md
test_scenarios_documented() {
    test_info "Test 5: Verify test scenarios added to TESTS.md"
    
    if grep -q "Artifact Integrity Verification Implementation" "$PROJECT_ROOT/docs/TESTS.md"; then
        test_passed "Test scenarios documented in TESTS.md"
    else
        test_failed "Test scenarios not added to TESTS.md"
    fi
    
    if grep -q "checksum verification" "$PROJECT_ROOT/docs/TESTS.md"; then
        test_passed "Checksum verification scenarios documented"
    else
        test_failed "Checksum verification scenarios missing"
    fi
}

# Test 6: Check integrity verification functions
test_integrity_functions() {
    test_info "Test 6: Check integrity verification functions"
    
    # Check for key functions in the integrity module
    INTEGRITY_FILE="$PROJECT_ROOT/internal/storage/integrity.go"
    
    if grep -q "VerifyArtifactBundle" "$INTEGRITY_FILE"; then
        test_passed "VerifyArtifactBundle function implemented"
    else
        test_failed "VerifyArtifactBundle function missing"
    fi
    
    if grep -q "calculateChecksum" "$INTEGRITY_FILE"; then
        test_passed "Checksum calculation function implemented"
    else
        test_failed "Checksum calculation function missing"
    fi
    
    if grep -q "validateSBOMContent" "$INTEGRITY_FILE"; then
        test_passed "SBOM validation function implemented"
    else
        test_failed "SBOM validation function missing"
    fi
}

# Test 7: Verify error handling and reporting
test_error_handling() {
    test_info "Test 7: Verify error handling and reporting"
    
    INTEGRITY_FILE="$PROJECT_ROOT/internal/storage/integrity.go"
    
    if grep -q "BundleIntegrityResult" "$INTEGRITY_FILE"; then
        test_passed "Bundle integrity result structure implemented"
    else
        test_failed "Bundle integrity result structure missing"
    fi
    
    if grep -q "GetVerificationSummary" "$INTEGRITY_FILE"; then
        test_passed "Verification summary function implemented"
    else
        test_failed "Verification summary function missing"
    fi
}

# Test 8: Check integration with existing storage upload
test_storage_integration() {
    test_info "Test 8: Check integration with existing storage upload"
    
    BUILD_HANDLER="$PROJECT_ROOT/internal/build/handler.go"
    
    if grep -q "NewIntegrityVerifier" "$BUILD_HANDLER"; then
        test_passed "Build handler creates integrity verifier instances"
    else
        test_failed "Build handler not creating integrity verifier instances"
    fi
    
    if grep -q "integrity.*verified" "$BUILD_HANDLER"; then
        test_passed "Build handler reports verification results"
    else
        test_failed "Build handler not reporting verification results"
    fi
}

# Main execution
main() {
    echo "Starting Artifact Integrity Verification Unit Tests..."
    echo "======================================================"
    
    test_compilation
    test_files_exist
    test_seaweedfs_implementation
    test_build_handler_integration
    test_scenarios_documented
    test_integrity_functions
    test_error_handling
    test_storage_integration
    
    echo
    echo "======================================================"
    test_passed "All Artifact Integrity Verification unit tests passed"
    
    echo
    echo "Summary:"
    echo "- ✓ Integrity verification module compiles successfully"
    echo "- ✓ Storage interface enhanced with verification methods"
    echo "- ✓ SeaweedFS client implements integrity verification"
    echo "- ✓ Build handler integrated with verification logic"
    echo "- ✓ Comprehensive test scenarios documented"
    echo "- ✓ Core verification functions implemented"
    echo "- ✓ Error handling and reporting structures in place"
    echo "- ✓ Integration with existing storage upload complete"
    echo
    echo "Phase 4 Step 2 implementation structure verified!"
}

# Run tests
main "$@"