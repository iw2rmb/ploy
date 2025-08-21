#!/bin/bash
set -e

echo "=== Traefik Integration Test ==="

# Start controller in background
pkill -f "./build/controller" || true
sleep 2
CONSUL_HTTP_ADDR=127.0.0.1:8500 nohup ./build/controller > test-controller.log 2>&1 &
CONTROLLER_PID=$!
sleep 5

echo "Controller started with PID: $CONTROLLER_PID"

# Test basic API health
echo "Testing basic API health..."
curl -f http://127.0.0.1:8081/v1/apps || {
    echo "API health check failed"
    exit 1
}

echo "API is responding"

# Test domain listing (should include default domain)
echo "Testing domain listing..."
DOMAINS=$(curl -s http://127.0.0.1:8081/v1/apps/testapp/domains)
echo "Domain response: $DOMAINS"

# Test domain addition
echo "Testing domain addition..."
ADD_RESPONSE=$(curl -s -X POST http://127.0.0.1:8081/v1/apps/testapp/domains -H "Content-Type: application/json" -d {domain:
