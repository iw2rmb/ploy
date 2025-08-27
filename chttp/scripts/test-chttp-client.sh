#!/bin/bash
# Test script for CHTTP client container
set -euo pipefail

# Configuration
CHTTP_SERVER_URL="${CHTTP_SERVER_URL:-http://pylint-chttp:8080}"
CHTTP_CLIENT_ID="${CHTTP_CLIENT_ID:-dev-client}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" >&2
    exit 1
}

# Wait for CHTTP server to be ready
wait_for_server() {
    local max_attempts=30
    local attempt=1
    
    log "Waiting for CHTTP server at $CHTTP_SERVER_URL..."
    
    while [[ $attempt -le $max_attempts ]]; do
        if curl -s -f "$CHTTP_SERVER_URL/health" >/dev/null 2>&1; then
            log "CHTTP server is ready!"
            return 0
        fi
        
        log "Attempt $attempt/$max_attempts: Server not ready, waiting..."
        sleep 2
        ((attempt++))
    done
    
    error "CHTTP server did not become ready after $max_attempts attempts"
}

# Test health endpoint
test_health_endpoint() {
    log "Testing health endpoint..."
    
    local response
    if response=$(curl -s -w "HTTP_CODE:%{http_code}" "$CHTTP_SERVER_URL/health"); then
        local http_code="${response##*HTTP_CODE:}"
        local body="${response%HTTP_CODE:*}"
        
        if [[ "$http_code" == "200" ]]; then
            log "Health check passed: $body"
            return 0
        else
            warn "Health check returned HTTP $http_code: $body"
            return 1
        fi
    else
        error "Failed to connect to health endpoint"
    fi
}

# Create test Python archive
create_test_archive() {
    log "Creating test Python archive..."
    
    mkdir -p /tmp/test-python
    
    # Create Python file with intentional Pylint issues
    cat > /tmp/test-python/sample.py << 'EOF'
import os  # unused import
import sys

def hello_world():
    # missing docstring
    print("Hello World")
    unused_var = 42
    return "success"

class TestClass:
    def __init__(self):
        self.name = "test"
    
    def method_without_docstring(self):
        return self.name.upper()

if __name__ == "__main__":
    hello_world()
EOF

    # Create tar.gz archive
    cd /tmp
    tar -czf test-python.tar.gz test-python/
    log "Test archive created: /tmp/test-python.tar.gz"
}

# Test analysis endpoint (without auth for development)
test_analysis_endpoint() {
    log "Testing analysis endpoint..."
    
    create_test_archive
    
    local response
    if response=$(curl -s -w "HTTP_CODE:%{http_code}" \
        -X POST \
        -H "Content-Type: application/gzip" \
        --data-binary @/tmp/test-python.tar.gz \
        "$CHTTP_SERVER_URL/analyze"); then
        
        local http_code="${response##*HTTP_CODE:}"
        local body="${response%HTTP_CODE:*}"
        
        if [[ "$http_code" == "200" ]]; then
            log "Analysis request successful!"
            echo "Response: $body" | jq . 2>/dev/null || echo "Response: $body"
            return 0
        elif [[ "$http_code" == "401" ]]; then
            log "Analysis endpoint requires authentication (expected in production)"
            return 0
        else
            warn "Analysis request returned HTTP $http_code: $body"
            return 1
        fi
    else
        error "Failed to connect to analysis endpoint"
    fi
}

# Main test execution
main() {
    log "Starting CHTTP client tests..."
    log "Server URL: $CHTTP_SERVER_URL"
    log "Client ID: $CHTTP_CLIENT_ID"
    
    # Wait for server
    wait_for_server
    
    # Run tests
    local failed_tests=0
    
    if ! test_health_endpoint; then
        ((failed_tests++))
    fi
    
    if ! test_analysis_endpoint; then
        ((failed_tests++))
    fi
    
    # Report results
    if [[ $failed_tests -eq 0 ]]; then
        log "All tests passed successfully!"
        exit 0
    else
        error "$failed_tests test(s) failed"
    fi
}

# Run if executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi