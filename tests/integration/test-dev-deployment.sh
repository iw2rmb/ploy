#!/bin/bash

# Integration test for dev environment deployment
# Tests unified deployment system in development environment
# Must be run on VPS with full Ploy infrastructure stack

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_APP_NAME="test-dev-app-$(date +%s)"
TEST_TIMESTAMP=$(date +%Y%m%d-%H%M%S)
CONTROLLER_URL="${PLOY_CONTROLLER:-http://localhost:8081/v1}"

echo "🧪 Starting dev environment deployment integration test"
echo "📅 Timestamp: $TEST_TIMESTAMP"
echo "🎯 Controller: $CONTROLLER_URL"
echo ""

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Test prerequisites
test_prerequisites() {
    log_info "Checking test prerequisites..."
    
    # Check if we're on VPS with ploy infrastructure
    if ! [ -x "/opt/hashicorp/bin/nomad-job-manager.sh" ]; then
        log_error "Nomad wrapper not found - this test must run on VPS with Ploy infrastructure"
        exit 1
    fi
    
    if ! command -v consul &> /dev/null; then
        log_error "Consul not found - this test must run on VPS with Ploy infrastructure"
        exit 1
    fi
    
    # Check if ploy and ployman binaries exist
    if ! command -v ploy &> /dev/null && ! [ -f "./bin/ploy" ]; then
        log_error "ploy binary not found - build required"
        exit 1
    fi
    
    if ! command -v ployman &> /dev/null && ! [ -f "./bin/ployman" ]; then
        log_error "ployman binary not found - build required"
        exit 1
    fi
    
    # Check controller connectivity
    if ! curl -sf "$CONTROLLER_URL/health" > /dev/null; then
        log_error "Controller not accessible at $CONTROLLER_URL"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Test user app deployment (ploy push)
test_user_app_deployment() {
    log_info "Testing user app deployment via ploy push..."
    
    # Create temporary test app
    TEST_APP_DIR="/tmp/test-user-app-$TEST_TIMESTAMP"
    mkdir -p "$TEST_APP_DIR"
    cd "$TEST_APP_DIR"
    
    # Create simple test app
    cat > main.go << 'EOF'
package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, `{"status":"healthy","app":"test-dev-app","env":"dev"}`)
    })
    
    fmt.Printf("Test app listening on port %s\n", port)
    http.ListenAndServe(":"+port, nil)
}
EOF
    
    # Create go.mod
    cat > go.mod << EOF
module test-dev-app

go 1.22
EOF
    
    # Deploy using ploy push with dev environment
    log_info "Deploying test user app to dev environment..."
    
    if command -v ploy &> /dev/null; then
        PLOY_CMD="ploy"
    else
        PLOY_CMD="$(pwd)/../../bin/ploy"
    fi
    
    if ! PLOY_CONTROLLER="$CONTROLLER_URL" $PLOY_CMD push -a "$TEST_APP_NAME" -env dev; then
        log_error "User app deployment failed"
        cleanup_test_resources
        exit 1
    fi
    
    log_success "User app deployment completed"
    
    # Verify deployment via health check
    log_info "Verifying user app deployment..."
    EXPECTED_URL="https://${TEST_APP_NAME}.dev.ployd.app/health"
    
    # Wait for deployment with timeout
    TIMEOUT=120
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if curl -sf "$EXPECTED_URL" > /dev/null; then
            log_success "User app health check passed: $EXPECTED_URL"
            break
        fi
        log_warning "Waiting for user app deployment... ($ELAPSED/$TIMEOUT seconds)"
        sleep 10
        ELAPSED=$((ELAPSED + 10))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        log_error "User app health check timeout after $TIMEOUT seconds"
        return 1
    fi
    
    # Verify response content
    RESPONSE=$(curl -s "$EXPECTED_URL")
    if echo "$RESPONSE" | grep -q '"env":"dev"'; then
        log_success "User app response contains correct dev environment marker"
    else
        log_error "User app response missing dev environment marker: $RESPONSE"
        return 1
    fi
    
    cd - > /dev/null
    rm -rf "$TEST_APP_DIR"
    return 0
}

# Test platform service deployment (ployman push)
test_platform_service_deployment() {
    log_info "Testing platform service deployment via ployman push..."
    
    # Use existing ploy-api for platform service test
    if command -v ployman &> /dev/null; then
        PLOYMAN_CMD="ployman"
    else
        PLOYMAN_CMD="$(pwd)/bin/ployman"
    fi
    
    log_info "Deploying ploy-api platform service to dev environment..."
    
    if ! PLOY_CONTROLLER="$CONTROLLER_URL" $PLOYMAN_CMD push -a ploy-api -env dev; then
        log_error "Platform service deployment failed"
        exit 1
    fi
    
    log_success "Platform service deployment completed"
    
    # Verify deployment via health check
    log_info "Verifying platform service deployment..."
    EXPECTED_URL="https://api.dev.ployman.app/health"
    
    # Wait for deployment with timeout
    TIMEOUT=120
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if curl -sf "$EXPECTED_URL" > /dev/null; then
            log_success "Platform service health check passed: $EXPECTED_URL"
            break
        fi
        log_warning "Waiting for platform service deployment... ($ELAPSED/$TIMEOUT seconds)"
        sleep 10
        ELAPSED=$((ELAPSED + 10))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        log_error "Platform service health check timeout after $TIMEOUT seconds"
        return 1
    fi
    
    # Verify version endpoint
    VERSION_URL="https://api.dev.ployman.app/version"
    if curl -sf "$VERSION_URL" > /dev/null; then
        VERSION_RESPONSE=$(curl -s "$VERSION_URL")
        log_success "Platform service version endpoint accessible: $VERSION_RESPONSE"
    else
        log_error "Platform service version endpoint not accessible"
        return 1
    fi
    
    return 0
}

# Cleanup test resources
cleanup_test_resources() {
    log_info "Cleaning up test resources..."
    
    # Remove test user app if it exists (best-effort via wrapper)
    if [ -x "/opt/hashicorp/bin/nomad-job-manager.sh" ]; then
        log_info "Stopping test user app (best-effort): $TEST_APP_NAME"
        /opt/hashicorp/bin/nomad-job-manager.sh stop --job "$TEST_APP_NAME" || true
        sleep 5
        /opt/hashicorp/bin/nomad-job-manager.sh stop --job "$TEST_APP_NAME" || true
    fi
    
    # Clean up temporary directories
    rm -rf "/tmp/test-user-app-$TEST_TIMESTAMP" 2>/dev/null || true
    
    log_success "Cleanup completed"
}

# Main test execution
main() {
    echo "=================================================================="
    echo "🧪 Dev Environment Deployment Integration Test"
    echo "=================================================================="
    
    # Trap to ensure cleanup on exit
    trap cleanup_test_resources EXIT
    
    # Run test phases
    test_prerequisites
    echo ""
    
    if test_user_app_deployment; then
        log_success "✅ User app deployment test PASSED"
    else
        log_error "❌ User app deployment test FAILED"
        exit 1
    fi
    echo ""
    
    if test_platform_service_deployment; then
        log_success "✅ Platform service deployment test PASSED"
    else
        log_error "❌ Platform service deployment test FAILED"  
        exit 1
    fi
    echo ""
    
    echo "=================================================================="
    log_success "🎉 ALL DEV ENVIRONMENT DEPLOYMENT TESTS PASSED"
    echo "=================================================================="
    
    echo ""
    echo "📊 Test Summary:"
    echo "   ✅ User app deployment (ploy push) to *.dev.ployd.app"
    echo "   ✅ Platform service deployment (ployman push) to *.dev.ployman.app"  
    echo "   ✅ Health check verification for both domains"
    echo "   ✅ Environment-specific routing validated"
    echo ""
    echo "🎯 Next step: Test production deployment"
}

# Execute main function
main "$@"
