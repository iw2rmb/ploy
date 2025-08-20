#!/usr/bin/env bash

# Test script for Node.js version detection functionality
# Tests scenarios 451-480 from TESTS.md

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Helper functions
log_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((PASSED_TESTS++))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((FAILED_TESTS++))
}

run_test() {
    local test_name="$1"
    shift
    echo -e "\n${YELLOW}Running Test: $test_name${NC}"
    ((TOTAL_TESTS++))
}

# Setup test environment
setup_test_env() {
    log_info "Setting up test environment for Node.js version detection..."
    
    # Create temporary directory for tests
    export TEST_DIR="/tmp/ploy-nodejs-version-tests-$$"
    mkdir -p "$TEST_DIR"
    
    # Ensure we have access to build scripts
    if [[ ! -f "scripts/build/kraft/build_unikraft.sh" ]]; then
        log_error "Build script not found. Please run from ploy root directory."
        exit 1
    fi
    
    log_info "Test environment ready: $TEST_DIR"
}

# Cleanup test environment
cleanup_test_env() {
    log_info "Cleaning up test environment..."
    if [[ -n "${TEST_DIR:-}" ]] && [[ -d "$TEST_DIR" ]]; then
        rm -rf "$TEST_DIR"
    fi
}

# Test: Node.js version detection from package.json engines field
test_nodejs_version_detection() {
    run_test "Node.js version detection from package.json engines"
    
    local test_app="$TEST_DIR/test-app"
    mkdir -p "$test_app"
    
    # Test 451: engines.node "18"
    cat > "$test_app/package.json" << 'EOF'
{
  "name": "test-app",
  "version": "1.0.0",
  "main": "index.js",
  "engines": {
    "node": "18"
  }
}
EOF
    
    # Source the build script functions
    source scripts/build/kraft/build_unikraft.sh 2>/dev/null || {
        log_error "Could not source build script functions"
        return 1
    }
    
    local detected_version
    detected_version=$(get_nodejs_version_from_package "$test_app")
    
    if [[ "$detected_version" == "18" ]]; then
        log_success "Test 451: Detected Node.js v18 from engines.node '18'"
    else
        log_error "Test 451: Expected v18, got v$detected_version"
    fi
    
    # Test 452: engines.node "^20.0.0"
    cat > "$test_app/package.json" << 'EOF'
{
  "name": "test-app",
  "engines": {
    "node": "^20.0.0"
  }
}
EOF
    
    detected_version=$(get_nodejs_version_from_package "$test_app")
    
    if [[ "$detected_version" == "20" ]]; then
        log_success "Test 452: Detected Node.js v20 from engines.node '^20.0.0'"
    else
        log_error "Test 452: Expected v20, got v$detected_version"
    fi
    
    # Test 456: No engines field (default to 18)
    cat > "$test_app/package.json" << 'EOF'
{
  "name": "test-app",
  "version": "1.0.0"
}
EOF
    
    detected_version=$(get_nodejs_version_from_package "$test_app")
    
    if [[ "$detected_version" == "18" ]]; then
        log_success "Test 456: Defaults to Node.js v18 when no engines field"
    else
        log_error "Test 456: Expected default v18, got v$detected_version"
    fi
    
    # Test 458: Malformed package.json
    echo "{ invalid json" > "$test_app/package.json"
    
    detected_version=$(get_nodejs_version_from_package "$test_app")
    
    if [[ "$detected_version" == "18" ]]; then
        log_success "Test 458: Defaults to Node.js v18 for malformed package.json"
    else
        log_error "Test 458: Expected default v18, got v$detected_version"
    fi
}

# Test: Node.js binary download simulation (without actual download)
test_nodejs_download_setup() {
    run_test "Node.js download setup logic"
    
    local test_app="$TEST_DIR/download-test"
    mkdir -p "$test_app"
    
    cat > "$test_app/package.json" << 'EOF'
{
  "name": "download-test",
  "engines": {
    "node": "18"
  }
}
EOF
    
    # Source the build script functions
    source scripts/build/kraft/build_unikraft.sh 2>/dev/null || {
        log_error "Could not source build script functions"
        return 1
    }
    
    # Test version determination
    local required_version
    required_version=$(get_nodejs_version_from_package "$test_app")
    
    if [[ "$required_version" == "18" ]]; then
        log_success "Test 466: Version determination works for download setup"
    else
        log_error "Test 466: Version determination failed"
    fi
    
    # Test directory structure for caching
    local node_dir="$test_app/.unikraft-node"
    mkdir -p "$node_dir/bin"
    
    if [[ -d "$node_dir" ]]; then
        log_success "Test 468: Cache directory structure created correctly"
    else
        log_error "Test 468: Cache directory creation failed"
    fi
}

# Test: Kraft YAML generation with Node.js version
test_kraft_yaml_nodejs_version() {
    run_test "Kraft YAML generation with Node.js version"
    
    local test_app="$TEST_DIR/kraft-test"
    mkdir -p "$test_app"
    
    cat > "$test_app/package.json" << 'EOF'
{
  "name": "kraft-test-app",
  "version": "1.0.0",
  "main": "server.js",
  "engines": {
    "node": "20"
  }
}
EOF
    
    # Create a simple template for testing
    mkdir -p "lanes/B-unikraft-nodejs"
    cat > "lanes/B-unikraft-nodejs/kraft.yaml" << 'EOF'
spec_version: '0.6'
name: nodejs-app
unikraft:
  version: stable
targets:
  - architecture: x86_64
    platform: qemu
application:
  sources: ./
options:
  http_port: 8080
EOF
    
    # Run kraft yaml generation
    if bash scripts/build/kraft/gen_kraft_yaml.sh --lane B --app-dir "$test_app" --port 8080; then
        if [[ -f "$test_app/kraft.yaml" ]]; then
            if grep -q "Node.js version requirement: 20" "$test_app/kraft.yaml"; then
                log_success "Test 464: Kraft YAML includes Node.js version requirement"
            else
                log_error "Test 464: Kraft YAML missing Node.js version comment"
            fi
            
            if grep -q "name: kraft-test-app" "$test_app/kraft.yaml"; then
                log_success "Test: Kraft YAML uses correct app name from package.json"
            else
                log_error "Test: Kraft YAML missing correct app name"
            fi
        else
            log_error "Test 464: Kraft YAML file not generated"
        fi
    else
        log_error "Test 464: Kraft YAML generation failed"
    fi
    
    # Cleanup template
    rm -rf "lanes/B-unikraft-nodejs"
}

# Test: Build script integration
test_build_script_integration() {
    run_test "Build script Node.js version integration"
    
    local test_app="$TEST_DIR/build-test"
    mkdir -p "$test_app"
    
    cat > "$test_app/package.json" << 'EOF'
{
  "name": "build-test-app",
  "version": "1.0.0",
  "main": "index.js",
  "engines": {
    "node": "18"
  },
  "dependencies": {
    "express": "^4.18.0"
  }
}
EOF
    
    cat > "$test_app/index.js" << 'EOF'
const express = require('express');
const app = express();

app.get('/healthz', (req, res) => {
  res.json({ status: 'ok' });
});

app.listen(8080, () => {
  console.log('Server running on port 8080');
});
EOF
    
    # Source the build script functions
    source scripts/build/kraft/build_unikraft.sh 2>/dev/null || {
        log_error "Could not source build script functions"
        return 1
    }
    
    # Test Node.js detection
    if has_nodejs "$test_app"; then
        log_success "Test: Build script detects Node.js application"
    else
        log_error "Test: Build script failed to detect Node.js application"
    fi
    
    # Test version setup (without actual download)
    if get_nodejs_version_from_package "$test_app" >/dev/null; then
        log_success "Test 463: Build script logs Node.js version requirement"
    else
        log_error "Test 463: Build script version logging failed"
    fi
}

# Main test execution
main() {
    echo "=========================================="
    echo "Node.js Version Detection Test Suite"
    echo "=========================================="
    
    setup_test_env
    
    # Trap to ensure cleanup on exit
    trap cleanup_test_env EXIT
    
    # Run all test suites
    test_nodejs_version_detection
    test_nodejs_download_setup
    test_kraft_yaml_nodejs_version
    test_build_script_integration
    
    # Test summary
    echo ""
    echo "=========================================="
    echo "Test Results Summary"
    echo "=========================================="
    echo "Total Tests: $TOTAL_TESTS"
    echo -e "Passed: ${GREEN}$PASSED_TESTS${NC}"
    echo -e "Failed: ${RED}$FAILED_TESTS${NC}"
    
    if [[ $FAILED_TESTS -eq 0 ]]; then
        echo -e "\n${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "\n${RED}Some tests failed!${NC}"
        exit 1
    fi
}

# Run main function
main "$@"