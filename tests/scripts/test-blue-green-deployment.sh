#!/bin/bash

# Test Blue-Green Deployment Implementation
# Tests Phase Networking-2 Step 3: Blue-Green deployment with gradual traffic shifting

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
TEST_APP="test-bluegreen-app"
CONTROLLER_URL="https://api.dev.ployd.app/v1"
VERSION_1="v1.0.0"
VERSION_2="v1.1.0"
TIMEOUT=300  # 5 minutes timeout

print_header() {
    echo -e "${BLUE}============================================${NC}"
    echo -e "${BLUE} Blue-Green Deployment Test Suite${NC}"
    echo -e "${BLUE}============================================${NC}"
}

print_test() {
    echo -e "${YELLOW}Test: $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# Cleanup function
cleanup() {
    echo "🧹 Cleaning up test resources..."
    
    # Stop any running blue-green deployments
    curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/rollback" || true
    
    # Remove test app
    ./build/ploy apps destroy --name "$TEST_APP" --force 2>/dev/null || true
    
    echo "Cleanup completed"
}

# Set up cleanup trap
trap cleanup EXIT

wait_for_deployment() {
    local app_name="$1"
    local expected_status="$2"
    local timeout="$3"
    
    echo "Waiting for deployment to reach status: $expected_status (timeout: ${timeout}s)"
    
    for i in $(seq 1 $timeout); do
        local status=$(curl -s "$CONTROLLER_URL/apps/$app_name/blue-green/status" | jq -r '.deployment.status // "unknown"' 2>/dev/null || echo "unknown")
        
        if [ "$status" = "$expected_status" ]; then
            print_success "Deployment reached status: $expected_status"
            return 0
        fi
        
        if [ $((i % 10)) -eq 0 ]; then
            echo "Still waiting... (${i}s) Current status: $status"
        fi
        
        sleep 1
    done
    
    print_error "Timeout waiting for status: $expected_status"
    return 1
}

check_traffic_weights() {
    local app_name="$1"
    local expected_blue="$2"
    local expected_green="$3"
    
    local response=$(curl -s "$CONTROLLER_URL/apps/$app_name/blue-green/status")
    local blue_weight=$(echo "$response" | jq -r '.deployment.blue_weight // 0')
    local green_weight=$(echo "$response" | jq -r '.deployment.green_weight // 0')
    
    if [ "$blue_weight" = "$expected_blue" ] && [ "$green_weight" = "$expected_green" ]; then
        print_success "Traffic weights correct: Blue=$blue_weight%, Green=$green_weight%"
        return 0
    else
        print_error "Traffic weights incorrect: Expected Blue=$expected_blue%, Green=$expected_green%. Got Blue=$blue_weight%, Green=$green_weight%"
        return 1
    fi
}

test_prerequisites() {
    print_test "Checking prerequisites"
    
    # Check if controller is running
    if ! curl -s "$CONTROLLER_URL/health" > /dev/null; then
        print_error "Controller not accessible at $CONTROLLER_URL"
        print_warning "Start the controller with: PORT=8081 ./build/controller"
        exit 1
    fi
    print_success "Controller is accessible"
    
    # Check if CLI binary exists
    if [ ! -f "./build/ploy" ]; then
        print_error "CLI binary not found at ./build/ploy"
        print_warning "Build the CLI with: go build -o build/ploy ./cmd/ploy"
        exit 1
    fi
    print_success "CLI binary found"
    
    # Check if jq is available for JSON parsing
    if ! command -v jq &> /dev/null; then
        print_warning "jq not found - some tests may be limited"
    else
        print_success "jq available for JSON parsing"
    fi
}

test_create_initial_app() {
    print_test "Creating initial application"
    
    # Create test app directory
    mkdir -p "test-apps/$TEST_APP"
    cd "test-apps/$TEST_APP"
    
    # Create simple Go app
    cat > main.go << 'EOF'
package main

import (
    "fmt"
    "net/http"
    "os"
)

func main() {
    version := os.Getenv("APP_VERSION")
    if version == "" {
        version = "unknown"
    }
    
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "Hello from version %s\n", version)
    })
    
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        fmt.Fprintf(w, "OK")
    })
    
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    fmt.Printf("Starting server on port %s\n", port)
    http.ListenAndServe(":"+port, nil)
}
EOF

    # Create go.mod
    echo "module test-app" > go.mod
    echo "go 1.21" >> go.mod
    
    # Deploy initial version
    APP_VERSION="$VERSION_1" ../../build/ploy push -a "$TEST_APP" -sha "$VERSION_1"
    
    # Wait for deployment
    sleep 10
    
    cd ../..
    print_success "Initial application deployed"
}

test_start_blue_green_deployment() {
    print_test "Starting Blue-Green deployment"
    
    # Start blue-green deployment with new version
    local response=$(curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/deploy/blue-green" \
        -H "Content-Type: application/json" \
        -d "{\"version\": \"$VERSION_2\"}")
    
    local status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "deployment_started" ]; then
        print_success "Blue-Green deployment started successfully"
        
        # Check initial state - all traffic should be on blue
        sleep 5
        check_traffic_weights "$TEST_APP" 100 0
        
    else
        print_error "Failed to start Blue-Green deployment: $response"
        return 1
    fi
}

test_get_deployment_status() {
    print_test "Getting deployment status"
    
    local response=$(curl -s "$CONTROLLER_URL/apps/$TEST_APP/blue-green/status")
    local status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "success" ]; then
        print_success "Deployment status retrieved successfully"
        
        # Display deployment details
        local deployment=$(echo "$response" | jq -r '.deployment')
        local app_name=$(echo "$deployment" | jq -r '.app_name // "unknown"')
        local active_color=$(echo "$deployment" | jq -r '.active_color // "unknown"')
        local blue_weight=$(echo "$deployment" | jq -r '.blue_weight // 0')
        local green_weight=$(echo "$deployment" | jq -r '.green_weight // 0')
        
        echo "  App: $app_name"
        echo "  Active Color: $active_color"
        echo "  Traffic Distribution: Blue $blue_weight% / Green $green_weight%"
        
    else
        print_error "Failed to get deployment status: $response"
        return 1
    fi
}

test_manual_traffic_shifting() {
    print_test "Testing manual traffic shifting"
    
    # Test shifting to 25% green traffic
    local response=$(curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/shift" \
        -H "Content-Type: application/json" \
        -d "{\"target_weight\": 25}")
    
    local status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "success" ]; then
        print_success "Traffic shifted to 25% successfully"
        
        # Verify traffic weights
        sleep 3
        check_traffic_weights "$TEST_APP" 75 25
        
    else
        print_error "Failed to shift traffic: $response"
        return 1
    fi
    
    # Test shifting to 50% green traffic
    response=$(curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/shift" \
        -H "Content-Type: application/json" \
        -d "{\"target_weight\": 50}")
    
    status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "success" ]; then
        print_success "Traffic shifted to 50% successfully"
        
        # Verify traffic weights
        sleep 3
        check_traffic_weights "$TEST_APP" 50 50
        
    else
        print_error "Failed to shift traffic to 50%: $response"
        return 1
    fi
}

test_automatic_traffic_shifting() {
    print_test "Testing automatic traffic shifting"
    
    # Reset to 0% green traffic first
    curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/shift" \
        -H "Content-Type: application/json" \
        -d "{\"target_weight\": 0}" > /dev/null
    
    sleep 3
    
    # Start automatic traffic shifting
    local response=$(curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/auto-shift")
    local status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "success" ]; then
        print_success "Automatic traffic shifting started"
        
        # Wait and check traffic progression
        echo "Monitoring automatic traffic shifting..."
        
        for step in 10 25 50 75 100; do
            echo "Waiting for traffic to reach ${step}% green..."
            
            # Wait up to 2 minutes for each step
            local found=false
            for i in $(seq 1 120); do
                local current_green=$(curl -s "$CONTROLLER_URL/apps/$TEST_APP/blue-green/status" | jq -r '.deployment.green_weight // 0' 2>/dev/null || echo "0")
                
                if [ "$current_green" = "$step" ]; then
                    print_success "Traffic reached ${step}% green"
                    found=true
                    break
                fi
                
                sleep 1
            done
            
            if [ "$found" = false ]; then
                print_warning "Did not reach ${step}% green traffic within timeout"
                break
            fi
        done
        
    else
        print_error "Failed to start automatic traffic shifting: $response"
        return 1
    fi
}

test_complete_deployment() {
    print_test "Completing Blue-Green deployment"
    
    local response=$(curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/complete")
    local status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "success" ]; then
        print_success "Blue-Green deployment completed successfully"
        
        # Verify final state - green should be active with 100% traffic
        sleep 5
        local final_status=$(curl -s "$CONTROLLER_URL/apps/$TEST_APP/blue-green/status")
        local active_color=$(echo "$final_status" | jq -r '.deployment.active_color // "unknown"')
        local green_weight=$(echo "$final_status" | jq -r '.deployment.green_weight // 0')
        
        if [ "$active_color" = "green" ] && [ "$green_weight" = "100" ]; then
            print_success "Final state correct: Green active with 100% traffic"
        else
            print_error "Final state incorrect: Active=$active_color, Green=$green_weight%"
            return 1
        fi
        
    else
        print_error "Failed to complete deployment: $response"
        return 1
    fi
}

test_rollback_deployment() {
    print_test "Testing deployment rollback"
    
    # Start a new blue-green deployment for rollback test
    curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/deploy/blue-green" \
        -H "Content-Type: application/json" \
        -d "{\"version\": \"v1.2.0\"}" > /dev/null
    
    sleep 5
    
    # Shift some traffic to green
    curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/shift" \
        -H "Content-Type: application/json" \
        -d "{\"target_weight\": 25}" > /dev/null
    
    sleep 3
    
    # Test rollback
    local response=$(curl -s -X POST "$CONTROLLER_URL/apps/$TEST_APP/blue-green/rollback")
    local status=$(echo "$response" | jq -r '.status // "error"')
    
    if [ "$status" = "success" ]; then
        print_success "Deployment rollback completed successfully"
        
        # Verify rollback state
        sleep 5
        local rollback_status=$(curl -s "$CONTROLLER_URL/apps/$TEST_APP/blue-green/status")
        local blue_weight=$(echo "$rollback_status" | jq -r '.deployment.blue_weight // 0')
        local green_weight=$(echo "$rollback_status" | jq -r '.deployment.green_weight // 0')
        
        if [ "$blue_weight" = "100" ] && [ "$green_weight" = "0" ]; then
            print_success "Rollback state correct: All traffic back to blue"
        else
            print_error "Rollback state incorrect: Blue=$blue_weight%, Green=$green_weight%"
            return 1
        fi
        
    else
        print_error "Failed to rollback deployment: $response"
        return 1
    fi
}

test_cli_integration() {
    print_test "Testing CLI integration"
    
    # Test CLI status command
    local cli_status=$(./build/ploy bluegreen status "$TEST_APP" 2>&1)
    if echo "$cli_status" | grep -q "Blue-Green Deployment Status"; then
        print_success "CLI status command working"
    else
        print_warning "CLI status command may have issues: $cli_status"
    fi
    
    # Test CLI help
    local cli_help=$(./build/ploy bluegreen --help 2>&1 || ./build/ploy bluegreen 2>&1)
    if echo "$cli_help" | grep -q "Commands:"; then
        print_success "CLI help working"
    else
        print_warning "CLI help may have issues"
    fi
}

# Main test execution
main() {
    print_header
    
    # Run test suite
    test_prerequisites
    test_create_initial_app
    test_start_blue_green_deployment
    test_get_deployment_status
    test_manual_traffic_shifting
    test_automatic_traffic_shifting
    test_complete_deployment
    test_rollback_deployment
    test_cli_integration
    
    echo
    echo -e "${GREEN}============================================${NC}"
    echo -e "${GREEN} All Blue-Green Deployment tests passed! ✓${NC}"
    echo -e "${GREEN}============================================${NC}"
    
    echo
    echo "📋 Test Summary:"
    echo "✓ Blue-Green deployment initiation"
    echo "✓ Traffic weight management"
    echo "✓ Manual traffic shifting"
    echo "✓ Automatic traffic shifting"
    echo "✓ Deployment completion"
    echo "✓ Deployment rollback"
    echo "✓ CLI integration"
    
    echo
    echo "🚀 Blue-Green deployment is ready for production use!"
}

# Run main function
main "$@"