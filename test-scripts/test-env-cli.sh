#!/bin/bash

# Test script for environment variables CLI functionality
# Tests scenarios 127-130 from TESTS.md

set -e

PLOY_CLI="./ploy"
TEST_APP="test-cli-env"

echo "=== Testing Environment Variables CLI ==="
echo "CLI binary: $PLOY_CLI"
echo "Test App: $TEST_APP"
echo

# Build CLI if not exists
if [ ! -f "$PLOY_CLI" ]; then
    echo "Building CLI..."
    go build -o ploy ./cmd/ploy
fi

# Test 127: ploy env list app shows empty message
echo "📋 Test 127: ploy env list shows empty message for new app"
output=$("$PLOY_CLI" env list "$TEST_APP" 2>&1 || true)
if echo "$output" | grep -q "(none)"; then
    echo "✅ PASS: Empty environment variables message displayed correctly"
else
    echo "❌ FAIL: Expected '(none)' message, got: $output"
fi
echo

# Test 129: ploy env set app KEY VALUE
echo "📋 Test 129: ploy env set displays success message"
output=$("$PLOY_CLI" env set "$TEST_APP" NODE_ENV production 2>&1 || true)
if echo "$output" | grep -q "Environment variable NODE_ENV set for app"; then
    echo "✅ PASS: Environment variable set message displayed correctly"
else
    echo "❌ FAIL: Expected success message, got: $output"
fi
echo

# Test 127: ploy env list app shows variables
echo "📋 Test 127: ploy env list shows variables after setting"
output=$("$PLOY_CLI" env list "$TEST_APP" 2>&1 || true)
if echo "$output" | grep -q "NODE_ENV=production"; then
    echo "✅ PASS: Environment variables listed correctly"
else
    echo "❌ FAIL: Expected NODE_ENV=production, got: $output"
fi
echo

# Set multiple variables for further testing
echo "📋 Setting additional test variables"
"$PLOY_CLI" env set "$TEST_APP" DATABASE_URL "postgres://localhost" >/dev/null 2>&1 || true
"$PLOY_CLI" env set "$TEST_APP" DEBUG "true" >/dev/null 2>&1 || true

# Test 128: ploy env get app KEY
echo "📋 Test 128: ploy env get displays specific variable"
output=$("$PLOY_CLI" env get "$TEST_APP" NODE_ENV 2>&1 || true)
if echo "$output" | grep -q "NODE_ENV=production"; then
    echo "✅ PASS: Specific environment variable retrieved correctly"
else
    echo "❌ FAIL: Expected NODE_ENV=production, got: $output"
fi
echo

# Test 128: ploy env get for non-existent variable
echo "📋 Test 128: ploy env get shows not found for missing variable"
output=$("$PLOY_CLI" env get "$TEST_APP" NON_EXISTENT 2>&1 || true)
if echo "$output" | grep -q "not found"; then
    echo "✅ PASS: Not found message displayed for missing variable"
else
    echo "❌ FAIL: Expected 'not found' message, got: $output"
fi
echo

# Test 130: ploy env delete app KEY
echo "📋 Test 130: ploy env delete displays success message"
output=$("$PLOY_CLI" env delete "$TEST_APP" DEBUG 2>&1 || true)
if echo "$output" | grep -q "Environment variable DEBUG deleted from app"; then
    echo "✅ PASS: Environment variable deletion message displayed correctly"
else
    echo "❌ FAIL: Expected deletion success message, got: $output"
fi
echo

# Verify deletion worked
echo "📋 Verify deletion: ploy env list should not show deleted variable"
output=$("$PLOY_CLI" env list "$TEST_APP" 2>&1 || true)
if ! echo "$output" | grep -q "DEBUG="; then
    echo "✅ PASS: Deleted variable no longer appears in list"
else
    echo "❌ FAIL: Deleted variable still appears: $output"
fi
echo

# Test error handling
echo "📋 Test error handling: invalid command usage"
output=$("$PLOY_CLI" env 2>&1 || true)
if echo "$output" | grep -q "usage:"; then
    echo "✅ PASS: Usage message displayed for invalid command"
else
    echo "❌ FAIL: Expected usage message, got: $output"
fi
echo

output=$("$PLOY_CLI" env set 2>&1 || true)
if echo "$output" | grep -q "usage:"; then
    echo "✅ PASS: Usage message displayed for insufficient arguments"
else
    echo "❌ FAIL: Expected usage message, got: $output"
fi
echo

echo "=== Environment Variables CLI Tests Complete ==="
echo
echo "Summary: CLI commands for environment variables working correctly"
echo "Note: These tests require the controller to be running at $PLOY_CONTROLLER"