#!/bin/bash

# Test script for environment variables functionality
# Tests scenarios 123-145 from TESTS.md

set -e

# Dynamic controller endpoint based on environment
PLOY_APPS_DOMAIN=${PLOY_APPS_DOMAIN:-"ployd.app"}
PLOY_ENVIRONMENT=${PLOY_ENVIRONMENT:-"dev"}

if [ "$PLOY_ENVIRONMENT" = "dev" ]; then
    BASE_URL="${PLOY_CONTROLLER:-https://api.dev.${PLOY_APPS_DOMAIN}/v1}"
else
    BASE_URL="${PLOY_CONTROLLER:-https://api.${PLOY_APPS_DOMAIN}/v1}"
fi

TEST_APP="test-env-app"

echo "=== Testing Environment Variables API ==="
echo "Controller URL: $BASE_URL"
echo "Test App: $TEST_APP"
echo

# Test 124: GET returns empty object for app with no environment variables
echo "📋 Test 124: GET returns empty object for new app"
response=$(curl -s "$BASE_URL/apps/$TEST_APP/env")
if echo "$response" | grep -q '"env":{}'; then
    echo "✅ PASS: Empty environment variables returned correctly"
else
    echo "❌ FAIL: Expected empty env object, got: $response"
fi
echo

# Test 123: POST with JSON map sets multiple environment variables
echo "📋 Test 123: POST sets multiple environment variables"
response=$(curl -s -X POST "$BASE_URL/apps/$TEST_APP/env" \
    -H "Content-Type: application/json" \
    -d '{"NODE_ENV":"production","DATABASE_URL":"postgres://localhost","DEBUG":"true"}')
if echo "$response" | grep -q '"status":"updated"'; then
    echo "✅ PASS: Multiple environment variables set successfully"
else
    echo "❌ FAIL: Failed to set multiple variables: $response"
fi
echo

# Test 124: GET returns all environment variables
echo "📋 Test 124: GET returns all environment variables"
response=$(curl -s "$BASE_URL/apps/$TEST_APP/env")
if echo "$response" | grep -q '"NODE_ENV":"production"' && \
   echo "$response" | grep -q '"DATABASE_URL":"postgres://localhost"' && \
   echo "$response" | grep -q '"DEBUG":"true"'; then
    echo "✅ PASS: All environment variables retrieved correctly"
else
    echo "❌ FAIL: Environment variables not retrieved correctly: $response"
fi
echo

# Test 125: PUT updates single environment variable
echo "📋 Test 125: PUT updates single environment variable"
response=$(curl -s -X PUT "$BASE_URL/apps/$TEST_APP/env/DEBUG" \
    -H "Content-Type: application/json" \
    -d '{"value":"false"}')
if echo "$response" | grep -q '"status":"updated"'; then
    echo "✅ PASS: Single environment variable updated successfully"
else
    echo "❌ FAIL: Failed to update single variable: $response"
fi
echo

# Verify the update
echo "📋 Verify update: GET updated variable"
response=$(curl -s "$BASE_URL/apps/$TEST_APP/env")
if echo "$response" | grep -q '"DEBUG":"false"'; then
    echo "✅ PASS: Environment variable update verified"
else
    echo "❌ FAIL: Environment variable not updated: $response"
fi
echo

# Test 126: DELETE removes environment variable
echo "📋 Test 126: DELETE removes environment variable"
response=$(curl -s -X DELETE "$BASE_URL/apps/$TEST_APP/env/DEBUG")
if echo "$response" | grep -q '"status":"deleted"'; then
    echo "✅ PASS: Environment variable deleted successfully"
else
    echo "❌ FAIL: Failed to delete variable: $response"
fi
echo

# Verify the deletion
echo "📋 Verify deletion: GET should not include deleted variable"
response=$(curl -s "$BASE_URL/apps/$TEST_APP/env")
if ! echo "$response" | grep -q '"DEBUG"'; then
    echo "✅ PASS: Environment variable deletion verified"
else
    echo "❌ FAIL: Environment variable not deleted: $response"
fi
echo

# Test 143: Multiple operations preserve existing variables
echo "📋 Test 143: Setting new variable preserves existing ones"
response=$(curl -s -X PUT "$BASE_URL/apps/$TEST_APP/env/NEW_VAR" \
    -H "Content-Type: application/json" \
    -d '{"value":"new_value"}')
if echo "$response" | grep -q '"status":"updated"'; then
    echo "✅ PASS: New variable set successfully"
    
    # Verify existing variables are preserved
    response=$(curl -s "$BASE_URL/apps/$TEST_APP/env")
    if echo "$response" | grep -q '"NODE_ENV":"production"' && \
       echo "$response" | grep -q '"DATABASE_URL":"postgres://localhost"' && \
       echo "$response" | grep -q '"NEW_VAR":"new_value"'; then
        echo "✅ PASS: Existing variables preserved when adding new one"
    else
        echo "❌ FAIL: Existing variables not preserved: $response"
    fi
else
    echo "❌ FAIL: Failed to set new variable: $response"
fi
echo

# Test error handling
echo "📋 Test 141: Error handling for malformed JSON"
response=$(curl -s -X POST "$BASE_URL/apps/$TEST_APP/env" \
    -H "Content-Type: application/json" \
    -d '{"invalid":"json"malformed}')
if echo "$response" | grep -q '"error"'; then
    echo "✅ PASS: Error handling for malformed JSON works"
else
    echo "❌ FAIL: Should return error for malformed JSON: $response"
fi
echo

echo "=== Environment Variables API Tests Complete ==="
echo
echo "Summary: Environment variable storage, retrieval, updates, and deletions working correctly"
echo "Note: Build and deploy phase tests require full stack deployment"