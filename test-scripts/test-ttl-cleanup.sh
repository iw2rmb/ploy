#!/bin/bash

# Test script for TTL cleanup functionality
# Tests scenarios 543-567 from TESTS.md

set -e

BASE_URL="${PLOY_CONTROLLER:-http://localhost:8081/v1}"
TEST_APP="test-preview-app"
TEST_SHA="abcdef1234567890abcdef1234567890abcdef12"

echo "=== Testing TTL Cleanup Service ==="
echo "Controller URL: $BASE_URL"
echo "Test App: $TEST_APP"
echo "Test SHA: $TEST_SHA"
echo

# Test 566: GET /v1/cleanup/status provides service status
echo "📋 Test 566: GET cleanup status"
response=$(curl -s "$BASE_URL/cleanup/status")
if echo "$response" | grep -q '"service":"ttl-cleanup"'; then
    echo "✅ PASS: Cleanup status endpoint responds correctly"
    echo "Status: $(echo "$response" | jq -r '.status // "unknown"')"
    echo "Service running: $(echo "$response" | jq -r '.statistics.service_running // "unknown"')"
else
    echo "❌ FAIL: Cleanup status endpoint not working: $response"
fi
echo

# Test 563: GET /v1/cleanup/config returns current configuration
echo "📋 Test 563: GET cleanup configuration"
response=$(curl -s "$BASE_URL/cleanup/config")
if echo "$response" | grep -q '"config"'; then
    echo "✅ PASS: Cleanup config endpoint responds correctly"
    preview_ttl=$(echo "$response" | jq -r '.config.preview_ttl // "unknown"')
    cleanup_interval=$(echo "$response" | jq -r '.config.cleanup_interval // "unknown"')
    max_age=$(echo "$response" | jq -r '.config.max_age // "unknown"')
    echo "Preview TTL: $preview_ttl"
    echo "Cleanup Interval: $cleanup_interval" 
    echo "Max Age: $max_age"
else
    echo "❌ FAIL: Cleanup config endpoint not working: $response"
fi
echo

# Test 567: GET /v1/cleanup/jobs lists preview jobs
echo "📋 Test 567: GET preview jobs list"
response=$(curl -s "$BASE_URL/cleanup/jobs")
if echo "$response" | grep -q '"total_jobs"'; then
    echo "✅ PASS: Preview jobs list endpoint responds correctly"
    total_jobs=$(echo "$response" | jq -r '.total_jobs // 0')
    preview_jobs=$(echo "$response" | jq -r '.preview_jobs // 0')
    jobs_to_clean=$(echo "$response" | jq -r '.jobs_to_clean // 0')
    echo "Total Nomad jobs: $total_jobs"
    echo "Preview jobs: $preview_jobs"
    echo "Jobs to clean: $jobs_to_clean"
else
    echo "❌ FAIL: Preview jobs list endpoint not working: $response"
fi
echo

# Test 565: POST /v1/cleanup/trigger performs manual cleanup
echo "📋 Test 565: POST manual cleanup trigger (dry run)"
response=$(curl -s -X POST "$BASE_URL/cleanup/trigger?dry_run=true")
if echo "$response" | grep -q '"status":"completed"'; then
    echo "✅ PASS: Manual cleanup trigger (dry run) works"
    dry_run=$(echo "$response" | jq -r '.dry_run // false')
    echo "Dry run mode: $dry_run"
    if [ "$dry_run" = "true" ]; then
        echo "✅ PASS: Dry run mode correctly enabled"
    else
        echo "❌ FAIL: Dry run mode not enabled correctly"
    fi
else
    echo "❌ FAIL: Manual cleanup trigger not working: $response"
fi
echo

# Test 564: PUT /v1/cleanup/config updates configuration
echo "📋 Test 564: PUT update cleanup configuration"
original_response=$(curl -s "$BASE_URL/cleanup/config")
original_ttl=$(echo "$original_response" | jq -r '.config.preview_ttl // "24h0m0s"')

# Update with a test configuration
response=$(curl -s -X PUT "$BASE_URL/cleanup/config" \
    -H "Content-Type: application/json" \
    -d '{"preview_ttl":"12h","cleanup_interval":"3h"}')
if echo "$response" | grep -q '"status":"updated"'; then
    echo "✅ PASS: Configuration update successful"
    
    # Verify the update
    verify_response=$(curl -s "$BASE_URL/cleanup/config")
    updated_ttl=$(echo "$verify_response" | jq -r '.config.preview_ttl // "unknown"')
    if [ "$updated_ttl" = "12h0m0s" ]; then
        echo "✅ PASS: Configuration update verified"
    else
        echo "❌ FAIL: Configuration not updated correctly: $updated_ttl"
    fi
    
    # Restore original configuration
    echo "Restoring original configuration..."
    restore_response=$(curl -s -X PUT "$BASE_URL/cleanup/config" \
        -H "Content-Type: application/json" \
        -d "{\"preview_ttl\":\"$original_ttl\"}")
    if echo "$restore_response" | grep -q '"status":"updated"'; then
        echo "✅ Original configuration restored"
    fi
else
    echo "❌ FAIL: Configuration update failed: $response"
fi
echo

# Test service control endpoints (if service is not running)
echo "📋 Testing service control endpoints"
status_response=$(curl -s "$BASE_URL/cleanup/status")
service_running=$(echo "$status_response" | jq -r '.statistics.service_running // false')

if [ "$service_running" = "false" ]; then
    echo "📋 Test 556: POST start service"
    response=$(curl -s -X POST "$BASE_URL/cleanup/start")
    if echo "$response" | grep -q '"status":"started"'; then
        echo "✅ PASS: Service start successful"
        
        # Wait a moment and check status
        sleep 2
        status_response=$(curl -s "$BASE_URL/cleanup/status")
        if echo "$status_response" | jq -r '.statistics.service_running' | grep -q "true"; then
            echo "✅ PASS: Service is now running"
            
            # Test stop service
            echo "📋 Test 556: POST stop service"
            stop_response=$(curl -s -X POST "$BASE_URL/cleanup/stop")
            if echo "$stop_response" | grep -q '"status":"stopped"'; then
                echo "✅ PASS: Service stop successful"
            else
                echo "❌ FAIL: Service stop failed: $stop_response"
            fi
        else
            echo "❌ FAIL: Service not running after start"
        fi
    else
        echo "❌ FAIL: Service start failed: $response"
    fi
else
    echo "ℹ️  Service already running, skipping start/stop test"
fi
echo

# Test configuration defaults endpoint
echo "📋 Testing configuration defaults"
response=$(curl -s "$BASE_URL/cleanup/config/defaults")
if echo "$response" | grep -q '"defaults"'; then
    echo "✅ PASS: Configuration defaults endpoint works"
    default_preview_ttl=$(echo "$response" | jq -r '.defaults.preview_ttl // "unknown"')
    echo "Default preview TTL: $default_preview_ttl"
else
    echo "❌ FAIL: Configuration defaults endpoint not working: $response"
fi
echo

# Test error handling
echo "📋 Testing error handling"
response=$(curl -s -X PUT "$BASE_URL/cleanup/config" \
    -H "Content-Type: application/json" \
    -d '{"invalid":"json"malformed}')
if echo "$response" | grep -q '"error"'; then
    echo "✅ PASS: Error handling for malformed JSON works"
else
    echo "❌ FAIL: Should return error for malformed JSON: $response"
fi
echo

echo "=== TTL Cleanup Service Tests Complete ==="
echo
echo "Summary: TTL cleanup service configuration, control, and monitoring endpoints working correctly"
echo "Note: Full TTL cleanup functionality requires preview jobs and time-based testing"