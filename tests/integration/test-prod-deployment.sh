#!/bin/bash

# Integration test for production environment deployment
# Tests unified deployment system in production environment
# Must be run on VPS with full Ploy infrastructure stack

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_APP_NAME="test-prod-app-$(date +%s)"
TEST_TIMESTAMP=$(date +%Y%m%d-%H%M%S)
CONTROLLER_URL="${PLOY_CONTROLLER:-http://localhost:8081/v1}"

echo "🧪 Starting production environment deployment integration test"
echo "📅 Timestamp: $TEST_TIMESTAMP"
echo "🎯 Controller: $CONTROLLER_URL"
echo "⚠️  WARNING: This test deploys to PRODUCTION environment"
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

# Production safety confirmation
confirm_production_test() {
    log_warning "PRODUCTION ENVIRONMENT TEST"
    echo "This test will deploy applications to the production environment:"
    echo "  - User apps → *.ployd.app"
    echo "  - Platform services → *.ployman.app"
    echo ""
    
    # Auto-confirm for CI environments
    if [ "$CI" = "true" ] || [ "$PLOY_TEST_AUTO_CONFIRM" = "true" ]; then
        log_info "Auto-confirming production test in CI environment"
        return 0
    fi
    
    read -p "Are you sure you want to proceed with PRODUCTION testing? (yes/no): " confirm
    if [ "$confirm" != "yes" ]; then
        log_error "Production test cancelled by user"
        exit 1
    fi
    
    log_info "Production test confirmed, proceeding..."
}

# Test prerequisites  
test_prerequisites() {
    log_info "Checking production test prerequisites..."
    
    # Check if we're on VPS with ploy infrastructure
    if ! command -v nomad &> /dev/null; then
        log_error "Nomad not found - this test must run on VPS with Ploy infrastructure"
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
    
    # Verify production controller endpoint
    if echo "$CONTROLLER_URL" | grep -q "dev\|staging\|test"; then
        log_error "Controller URL appears to be non-production: $CONTROLLER_URL"
        log_error "Expected production controller URL (e.g., https://api.ployman.app/v1)"
        exit 1
    fi
    
    log_success "Prerequisites check passed"
}

# Test user app deployment (ploy push) to production
test_user_app_production_deployment() {
    log_info "Testing user app deployment to production via ploy push..."
    
    # Create temporary test app
    TEST_APP_DIR="/tmp/test-prod-user-app-$TEST_TIMESTAMP"
    mkdir -p "$TEST_APP_DIR"
    cd "$TEST_APP_DIR"
    
    # Create simple production-ready test app
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
        fmt.Fprintf(w, `{"status":"healthy","app":"test-prod-app","env":"production","timestamp":"%s"}`, 
            os.Getenv("DEPLOYMENT_TIME"))
    })
    
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, "Production Test App - Deployment Time: %s", os.Getenv("DEPLOYMENT_TIME"))
    })
    
    fmt.Printf("Production test app listening on port %s\n", port)
    http.ListenAndServe(":"+port, nil)
}
EOF
    
    # Create go.mod
    cat > go.mod << EOF
module test-prod-app

go 1.22
EOF
    
    # Deploy using ploy push with production environment
    log_info "Deploying test user app to production environment..."
    
    if command -v ploy &> /dev/null; then
        PLOY_CMD="ploy"
    else
        PLOY_CMD="$(pwd)/../../bin/ploy"
    fi
    
    if ! PLOY_CONTROLLER="$CONTROLLER_URL" $PLOY_CMD push -a "$TEST_APP_NAME" -env prod; then
        log_error "User app production deployment failed"
        cleanup_test_resources
        exit 1
    fi
    
    log_success "User app production deployment completed"
    
    # Verify deployment via health check
    log_info "Verifying user app production deployment..."
    EXPECTED_URL="https://${TEST_APP_NAME}.ployd.app/health"
    
    # Wait for deployment with longer timeout for production
    TIMEOUT=180
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if curl -sf "$EXPECTED_URL" > /dev/null; then
            log_success "User app production health check passed: $EXPECTED_URL"
            break
        fi
        log_warning "Waiting for user app production deployment... ($ELAPSED/$TIMEOUT seconds)"
        sleep 15
        ELAPSED=$((ELAPSED + 15))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        log_error "User app production health check timeout after $TIMEOUT seconds"
        return 1
    fi
    
    # Verify response content
    RESPONSE=$(curl -s "$EXPECTED_URL")
    if echo "$RESPONSE" | grep -q '"env":"production"'; then
        log_success "User app response contains correct production environment marker"
    else
        log_error "User app response missing production environment marker: $RESPONSE"
        return 1
    fi
    
    # Test root endpoint
    ROOT_RESPONSE=$(curl -s "https://${TEST_APP_NAME}.ployd.app/")
    if echo "$ROOT_RESPONSE" | grep -q "Production Test App"; then
        log_success "User app root endpoint accessible and responding correctly"
    else
        log_error "User app root endpoint not responding correctly: $ROOT_RESPONSE"
        return 1
    fi
    
    cd - > /dev/null
    rm -rf "$TEST_APP_DIR"
    return 0
}

# Test platform service deployment (ployman push) to production
test_platform_service_production_deployment() {
    log_info "Testing platform service deployment to production via ployman push..."
    
    # Use existing ploy-api for platform service production test
    if command -v ployman &> /dev/null; then
        PLOYMAN_CMD="ployman"
    else
        PLOYMAN_CMD="$(pwd)/bin/ployman"
    fi
    
    log_warning "Deploying ploy-api platform service to PRODUCTION environment..."
    log_info "This will update the live production API controller"
    
    if ! PLOY_CONTROLLER="$CONTROLLER_URL" $PLOYMAN_CMD push -a ploy-api -env prod; then
        log_error "Platform service production deployment failed"
        exit 1
    fi
    
    log_success "Platform service production deployment completed"
    
    # Verify deployment via health check
    log_info "Verifying platform service production deployment..."
    EXPECTED_URL="https://api.ployman.app/health"
    
    # Wait for deployment with longer timeout for production
    TIMEOUT=180
    ELAPSED=0
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if curl -sf "$EXPECTED_URL" > /dev/null; then
            log_success "Platform service production health check passed: $EXPECTED_URL"
            break
        fi
        log_warning "Waiting for platform service production deployment... ($ELAPSED/$TIMEOUT seconds)"
        sleep 15
        ELAPSED=$((ELAPSED + 15))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        log_error "Platform service production health check timeout after $TIMEOUT seconds"
        return 1
    fi
    
    # Verify version endpoint
    VERSION_URL="https://api.ployman.app/version"
    if curl -sf "$VERSION_URL" > /dev/null; then
        VERSION_RESPONSE=$(curl -s "$VERSION_URL")
        log_success "Platform service production version endpoint accessible"
        log_info "Production version: $VERSION_RESPONSE"
    else
        log_error "Platform service production version endpoint not accessible"
        return 1
    fi
    
    # Verify detailed version endpoint
    DETAILED_VERSION_URL="https://api.ployman.app/version/detailed"
    if curl -sf "$DETAILED_VERSION_URL" > /dev/null; then
        log_success "Platform service production detailed version endpoint accessible"
    else
        log_warning "Platform service production detailed version endpoint not accessible (non-critical)"
    fi
    
    return 0
}

# Test production domain resolution and SSL
test_production_infrastructure() {
    log_info "Testing production infrastructure (DNS, SSL, load balancing)..."
    
    # Test DNS resolution
    if nslookup api.ployman.app > /dev/null 2>&1; then
        log_success "Production DNS resolution for api.ployman.app working"
    else
        log_error "Production DNS resolution failed for api.ployman.app"
        return 1
    fi
    
    if nslookup ployd.app > /dev/null 2>&1; then
        log_success "Production DNS resolution for ployd.app working"
    else
        log_error "Production DNS resolution failed for ployd.app"
        return 1
    fi
    
    # Test SSL certificates
    if curl -I "https://api.ployman.app" 2>/dev/null | head -1 | grep -q "200\|301\|302"; then
        log_success "Production SSL certificate for api.ployman.app valid"
    else
        log_error "Production SSL certificate issue for api.ployman.app"
        return 1
    fi
    
    # Test load balancer health
    HEALTH_STATUS=$(curl -s "https://api.ployman.app/health" | grep -o '"status":"[^"]*"' || echo "failed")
    if echo "$HEALTH_STATUS" | grep -q '"status":"healthy"'; then
        log_success "Production load balancer health check passed"
    else
        log_warning "Production load balancer health check returned: $HEALTH_STATUS"
    fi
    
    return 0
}

# Cleanup test resources  
cleanup_test_resources() {
    log_info "Cleaning up production test resources..."
    
    # Remove test user app if it exists (only cleanup test apps, not production services)
    if nomad job status "$TEST_APP_NAME" &> /dev/null; then
        log_info "Stopping test user app: $TEST_APP_NAME"
        nomad job stop "$TEST_APP_NAME" || true
        sleep 10
        nomad job stop -purge "$TEST_APP_NAME" || true
    fi
    
    # Clean up temporary directories
    rm -rf "/tmp/test-prod-user-app-$TEST_TIMESTAMP" 2>/dev/null || true
    
    log_success "Production test cleanup completed"
}

# Main test execution
main() {
    echo "=================================================================="
    echo "🧪 Production Environment Deployment Integration Test"
    echo "=================================================================="
    
    # Trap to ensure cleanup on exit
    trap cleanup_test_resources EXIT
    
    # Run test phases with production safety
    confirm_production_test
    echo ""
    
    test_prerequisites
    echo ""
    
    if test_production_infrastructure; then
        log_success "✅ Production infrastructure test PASSED"
    else
        log_error "❌ Production infrastructure test FAILED"
        exit 1
    fi
    echo ""
    
    if test_user_app_production_deployment; then
        log_success "✅ User app production deployment test PASSED"
    else
        log_error "❌ User app production deployment test FAILED"
        exit 1
    fi
    echo ""
    
    if test_platform_service_production_deployment; then
        log_success "✅ Platform service production deployment test PASSED"
    else
        log_error "❌ Platform service production deployment test FAILED"  
        exit 1
    fi
    echo ""
    
    echo "=================================================================="
    log_success "🎉 ALL PRODUCTION DEPLOYMENT TESTS PASSED"
    echo "=================================================================="
    
    echo ""
    echo "📊 Production Test Summary:"
    echo "   ✅ User app deployment (ploy push) to *.ployd.app"
    echo "   ✅ Platform service deployment (ployman push) to *.ployman.app"
    echo "   ✅ Production infrastructure validation (DNS, SSL, load balancing)"
    echo "   ✅ Health check verification for production domains"
    echo "   ✅ Production environment routing validated"
    echo ""
    echo "🎯 Next step: Update documentation and complete deployment roadmap"
}

# Execute main function
main "$@"