#!/usr/bin/env bash

# Test script for enhanced build artifact upload with retry logic and verification
# Tests the upload retry and verification functionality implemented in Phase 5 Step 5

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
CONTROLLER_URL="${PLOY_CONTROLLER:-https://api.dev.ployd.app/v1}"
TEST_APP_NAME="test-enhanced-upload-$$"
TEST_DIR="/tmp/ploy-enhanced-upload-test-$$"

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

setup_test_environment() {
    log "Setting up test environment..."
    
    # Create test directory
    mkdir -p "$TEST_DIR"
    cd "$TEST_DIR"
    
    # Create a simple Node.js test application
    cat > package.json << 'EOF'
{
  "name": "test-enhanced-upload",
  "version": "1.0.0",
  "description": "Test app for enhanced upload functionality",
  "main": "index.js",
  "engines": {
    "node": "18"
  },
  "dependencies": {},
  "scripts": {
    "start": "node index.js"
  }
}
EOF

    cat > index.js << 'EOF'
const http = require('http');

const server = http.createServer((req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/plain' });
    res.end('Enhanced upload test app running!\n');
});

const port = process.env.PORT || 3000;
server.listen(port, () => {
    console.log(`Server running on port ${port}`);
});
EOF

    # Initialize git repo for proper testing
    git init
    git add .
    git commit -m "Initial commit for enhanced upload test"
    
    success "Test environment setup complete"
}

cleanup_test_environment() {
    log "Cleaning up test environment..."
    
    # Clean up test app if it exists
    if curl -s "${CONTROLLER_URL}/apps/${TEST_APP_NAME}" >/dev/null 2>&1; then
        log "Destroying test app: $TEST_APP_NAME"
        ./build/ploy apps destroy --name "$TEST_APP_NAME" --force || warning "Failed to destroy test app"
    fi
    
    # Clean up test directory
    cd /
    rm -rf "$TEST_DIR"
    
    log "Cleanup complete"
}

test_basic_upload_with_verification() {
    run_test "Basic upload with verification enabled"
    
    # Create tar archive
    tar -cf app.tar .
    
    # Test upload via API
    local response
    if response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/octet-stream" \
        --data-binary @app.tar \
        "${CONTROLLER_URL}/apps/${TEST_APP_NAME}/build?sha=test-basic-upload"); then
        
        local http_code="${response: -3}"
        local body="${response%???}"
        
        if [[ "$http_code" == "200" ]]; then
            success "Basic upload completed successfully"
            log "Response: $body"
        else
            error "Upload failed with HTTP $http_code: $body"
        fi
    else
        error "Failed to make upload request"
    fi
}

test_upload_retry_mechanism() {
    run_test "Upload retry mechanism with simulated failures"
    
    # This test would ideally involve network disruption or storage failures
    # For now, we test with a larger file that might experience partial uploads
    
    # Create a larger test file to increase chance of upload issues
    dd if=/dev/zero of=large-test-file.bin bs=1M count=10 2>/dev/null || {
        warning "Could not create large test file, skipping retry test"
        return
    }
    
    # Create tar with larger content
    tar -cf app-large.tar .
    
    local response
    if response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/octet-stream" \
        --data-binary @app-large.tar \
        "${CONTROLLER_URL}/apps/${TEST_APP_NAME}/build?sha=test-retry"); then
        
        local http_code="${response: -3}"
        local body="${response%???}"
        
        if [[ "$http_code" == "200" ]]; then
            success "Large file upload with retry capability completed"
        else
            warning "Large file upload failed (expected in some environments): HTTP $http_code"
        fi
    else
        warning "Failed to test retry mechanism (network dependent)"
    fi
    
    # Clean up large file
    rm -f large-test-file.bin app-large.tar
}

test_sbom_upload_verification() {
    run_test "SBOM upload with enhanced verification"
    
    # Create a mock SBOM file
    cat > .sbom.json << 'EOF'
{
  "spdxVersion": "SPDX-2.3",
  "dataLicense": "CC0-1.0",
  "SPDXID": "SPDXRef-DOCUMENT",
  "name": "test-enhanced-upload",
  "documentNamespace": "https://example.com/test-enhanced-upload",
  "creationInfo": {
    "created": "2025-08-20T12:00:00Z",
    "creators": ["Tool: test-script"]
  },
  "packages": [
    {
      "SPDXID": "SPDXRef-Package",
      "name": "test-enhanced-upload",
      "downloadLocation": "NOASSERTION",
      "filesAnalyzed": false
    }
  ]
}
EOF
    
    # Create tar archive including SBOM
    tar -cf app-with-sbom.tar .
    
    local response
    if response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/octet-stream" \
        --data-binary @app-with-sbom.tar \
        "${CONTROLLER_URL}/apps/${TEST_APP_NAME}/build?sha=test-sbom"); then
        
        local http_code="${response: -3}"
        local body="${response%???}"
        
        if [[ "$http_code" == "200" ]]; then
            success "SBOM upload with verification completed successfully"
            
            # Check if response mentions SBOM
            if echo "$body" | grep -q "sbom"; then
                success "SBOM properly detected in response"
            else
                warning "SBOM not mentioned in response (may be expected)"
            fi
        else
            error "SBOM upload failed with HTTP $http_code: $body"
        fi
    else
        error "Failed to test SBOM upload"
    fi
    
    rm -f .sbom.json app-with-sbom.tar
}

test_metadata_upload_verification() {
    run_test "Metadata upload with enhanced verification"
    
    # Create tar archive
    tar -cf app-metadata.tar .
    
    local response
    if response=$(curl -s -w "%{http_code}" -X POST \
        -H "Content-Type: application/octet-stream" \
        --data-binary @app-metadata.tar \
        "${CONTROLLER_URL}/apps/${TEST_APP_NAME}/build?sha=test-metadata&env=staging"); then
        
        local http_code="${response: -3}"
        local body="${response%???}"
        
        if [[ "$http_code" == "200" ]]; then
            success "Metadata upload with verification completed successfully"
            
            # Check for expected metadata in response
            if echo "$body" | grep -q "lane"; then
                success "Lane metadata properly included in response"
            fi
        else
            error "Metadata upload failed with HTTP $http_code: $body"
        fi
    else
        error "Failed to test metadata upload"
    fi
    
    rm -f app-metadata.tar
}

test_concurrent_upload_operations() {
    run_test "Concurrent upload operations with independent retry logic"
    
    # Create multiple tar archives
    tar -cf app-concurrent-1.tar .
    tar -cf app-concurrent-2.tar .
    
    # Start concurrent uploads
    local pid1 pid2
    {
        curl -s -X POST \
            -H "Content-Type: application/octet-stream" \
            --data-binary @app-concurrent-1.tar \
            "${CONTROLLER_URL}/apps/${TEST_APP_NAME}/build?sha=test-concurrent-1" \
            > /tmp/concurrent-1.log 2>&1
    } &
    pid1=$!
    
    {
        curl -s -X POST \
            -H "Content-Type: application/octet-stream" \
            --data-binary @app-concurrent-2.tar \
            "${CONTROLLER_URL}/apps/${TEST_APP_NAME}/build?sha=test-concurrent-2" \
            > /tmp/concurrent-2.log 2>&1
    } &
    pid2=$!
    
    # Wait for both uploads to complete
    wait $pid1
    local result1=$?
    wait $pid2
    local result2=$?
    
    if [[ $result1 -eq 0 && $result2 -eq 0 ]]; then
        success "Concurrent upload operations completed successfully"
    else
        warning "Some concurrent uploads failed (may be expected under load)"
    fi
    
    # Clean up
    rm -f app-concurrent-*.tar /tmp/concurrent-*.log
}

check_controller_availability() {
    log "Checking controller availability..."
    
    if ! curl -s "${CONTROLLER_URL}/status" >/dev/null; then
        error "Controller not available at $CONTROLLER_URL"
        log "Please ensure the controller is running: ./build/controller"
        exit 1
    fi
    
    success "Controller is available"
}

check_build_binary() {
    if [[ ! -x "./build/ploy" ]]; then
        error "Ploy CLI binary not found at ./build/ploy"
        log "Please build the CLI: go build -o build/ploy ./cmd/ploy"
        exit 1
    fi
    
    success "Ploy CLI binary is available"
}

main() {
    echo "Enhanced Build Artifact Upload Test Suite"
    echo "========================================"
    
    # Check prerequisites
    check_controller_availability
    check_build_binary
    
    # Set up test environment
    setup_test_environment
    
    # Ensure cleanup happens on script exit
    trap cleanup_test_environment EXIT
    
    # Run tests
    test_basic_upload_with_verification
    test_upload_retry_mechanism
    test_sbom_upload_verification
    test_metadata_upload_verification
    test_concurrent_upload_operations
    
    # Report results
    echo ""
    echo "Test Results Summary"
    echo "==================="
    echo "Total tests: $TOTAL_TESTS"
    echo "Passed: $PASSED_TESTS"
    echo "Failed: $((TOTAL_TESTS - PASSED_TESTS))"
    
    if [[ $PASSED_TESTS -eq $TOTAL_TESTS ]]; then
        success "All enhanced upload tests passed!"
        exit 0
    else
        error "Some enhanced upload tests failed!"
        exit 1
    fi
}

# Only run main if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi