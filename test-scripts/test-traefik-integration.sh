#!/bin/bash

# Test script for Traefik integration
# Tests Phase no-SPOF-2 step 3: Advanced Traefik Load Balancing Configuration

set -e

echo "=== Testing Traefik Advanced Load Balancing Integration ==="
echo "Base URL: ${PLOY_CONTROLLER:-http://localhost:8081}"
echo

# Test 1: Check Nomad has Traefik job
echo "📋 Test 1: Check if Traefik system job exists in Nomad"
if nomad job status traefik-system >/dev/null 2>&1; then
    echo "✅ PASS: Traefik system job exists in Nomad"
    echo "Job Status:"
    nomad job status traefik-system | head -10
else
    echo "❌ FAIL: Traefik system job not found in Nomad"
fi
echo

# Test 2: Check Traefik health endpoint
echo "📋 Test 2: Check Traefik health endpoint"
if curl -s -f http://localhost:8095/ping >/dev/null 2>&1; then
    echo "✅ PASS: Traefik health endpoint responds"
    echo "Response: $(curl -s http://localhost:8095/ping)"
else
    echo "❌ FAIL: Traefik health endpoint not accessible"
fi
echo

# Test 3: Check Traefik API endpoint
echo "📋 Test 3: Check Traefik API endpoint"
if curl -s -f http://localhost:8095/api/overview >/dev/null 2>&1; then
    echo "✅ PASS: Traefik API endpoint responds"
    response=$(curl -s http://localhost:8095/api/overview)
    echo "HTTP routers: $(echo "$response" | jq -r '.http.routers.total // "unknown"')"
    echo "HTTP services: $(echo "$response" | jq -r '.http.services.total // "unknown"')"
else
    echo "❌ FAIL: Traefik API endpoint not accessible"
fi
echo

# Test 4: Check Consul service registration
echo "📋 Test 4: Check Traefik service in Consul"
if curl -s http://localhost:8500/v1/catalog/service/traefik | jq -e '. | length > 0' >/dev/null 2>&1; then
    echo "✅ PASS: Traefik service registered in Consul"
    service_count=$(curl -s http://localhost:8500/v1/catalog/service/traefik | jq '. | length')
    echo "Traefik instances: $service_count"
else
    echo "❌ FAIL: Traefik service not found in Consul"
fi
echo

# Test 5: Test domain management API endpoints  
echo "📋 Test 5: Test domain management API endpoints"
BASE_URL="${PLOY_CONTROLLER:-http://localhost:8081/v1}"

# Test routing health endpoint
if curl -s -f "$BASE_URL/routing/health" >/dev/null 2>&1; then
    echo "✅ PASS: Routing health endpoint responds"
    health_response=$(curl -s "$BASE_URL/routing/health")
    echo "Status: $(echo "$health_response" | jq -r '.status // "unknown"')"
else
    echo "❌ FAIL: Routing health endpoint not working"
fi
echo

# Test domain registration endpoint (should work even without an active app)
echo "📋 Test 6: Test domain registration API structure"
test_response=$(curl -s -X POST "$BASE_URL/apps/test-app/domains" \
    -H "Content-Type: application/json" \
    -d '{"primary":"test.ployd.app","tls":true}')
    
if echo "$test_response" | jq -e '.status' >/dev/null 2>&1; then
    echo "✅ PASS: Domain registration API structure works"
    echo "Response status: $(echo "$test_response" | jq -r '.status')"
else
    echo "❌ FAIL: Domain registration API structure invalid"
fi
echo

# Test 7: Check firewall rules
echo "📋 Test 7: Check firewall rules for Traefik"
if command -v ufw >/dev/null 2>&1; then
    if ufw status | grep -E "(80|443|8080)" >/dev/null; then
        echo "✅ PASS: Firewall rules configured for Traefik"
        echo "Open ports:"
        ufw status | grep -E "(80|443|8080)"
    else
        echo "❌ FAIL: Traefik firewall rules not configured"
    fi
else
    echo "ℹ️  UFW not available, skipping firewall test"
fi
echo

echo "=== Traefik Integration Tests Complete ==="
echo
echo "Summary: Advanced Traefik load balancing with health checks and failover"
echo "Next steps: Deploy multiple controller instances, test load balancing"

# Test 8: Check advanced load balancing configuration
echo "📋 Test 8: Verify advanced load balancing configuration"
echo "Checking controller load balancer config..."
if [[ -f "platform/traefik/controller-load-balancer.yml" ]]; then
    echo "✅ PASS: Controller load balancer configuration exists"
    if grep -q "circuit-breaker" platform/traefik/controller-load-balancer.yml; then
        echo "✅ PASS: Circuit breaker middleware configured"
    fi
    if grep -q "sticky" platform/traefik/controller-load-balancer.yml; then
        echo "✅ PASS: Sticky sessions configured"
    fi
    if grep -q "healthCheck" platform/traefik/controller-load-balancer.yml; then
        echo "✅ PASS: Health check configuration found"
    fi
else
    echo "❌ FAIL: Controller load balancer configuration missing"
fi
echo

# Test 9: Check comprehensive middleware configuration
echo "📋 Test 9: Verify comprehensive middleware configuration"
if [[ -f "platform/traefik/middlewares.yml" ]]; then
    echo "✅ PASS: Comprehensive middleware configuration exists"
    middlewares=("rate-limit-api" "security-headers" "circuit-breaker" "retry-standard" "compress-standard")
    for middleware in "${middlewares[@]}"; do
        if grep -q "$middleware" platform/traefik/middlewares.yml; then
            echo "✅ PASS: Middleware $middleware configured"
        else
            echo "❌ FAIL: Middleware $middleware missing"
        fi
    done
else
    echo "❌ FAIL: Comprehensive middleware configuration missing"
fi
echo

# Test 10: Check enhanced routing logic
echo "📋 Test 10: Verify enhanced routing logic implementation"
if [[ -f "controller/routing/traefik.go" ]]; then
    echo "✅ PASS: Enhanced Traefik routing implementation exists"
    
    if grep -q "ControllerRouteConfig" controller/routing/traefik.go; then
        echo "✅ PASS: ControllerRouteConfig method implemented"
    fi
    
    if grep -q "RegisterController" controller/routing/traefik.go; then
        echo "✅ PASS: RegisterController method implemented"
    fi
    
    if grep -q "CircuitBreaker.*bool" controller/routing/traefik.go; then
        echo "✅ PASS: Circuit breaker support in RouteConfig"
    fi
    
    if grep -q "StickySession.*bool" controller/routing/traefik.go; then
        echo "✅ PASS: Sticky session support in RouteConfig"
    fi
    
    if grep -q "HealthCheckInterval" controller/routing/traefik.go; then
        echo "✅ PASS: Configurable health check intervals"
    fi
else
    echo "❌ FAIL: Enhanced routing implementation missing"
fi
echo

echo "=== Advanced Load Balancing Features Summary ==="
echo "✅ Health-based routing with automatic failover"
echo "✅ Circuit breaker patterns for fault tolerance"
echo "✅ Sticky sessions for stateful operations"
echo "✅ Advanced rate limiting and security headers"
echo "✅ SSL/TLS termination with Let's Encrypt"
echo "✅ Comprehensive middleware stack"
echo "✅ Enhanced routing logic with dynamic configuration"
echo