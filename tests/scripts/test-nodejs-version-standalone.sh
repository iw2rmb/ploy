#!/usr/bin/env bash

# Standalone test for Node.js version detection
# Extracts just the version detection function for testing

set -euo pipefail

# Extract the Node.js version detection function
get_nodejs_version_from_package() {
  local app_dir="$1"
  local package_json="$app_dir/package.json"
  
  if [[ ! -f "$package_json" ]]; then
    echo "18"  # Default version
    return 0
  fi
  
  # Extract engines.node field using node to parse JSON safely
  local node_version
  node_version=$(node -p "
    try {
      const pkg = require('$package_json');
      const engines = pkg.engines || {};
      const nodeVersion = engines.node || '';
      // Handle version ranges like '^18.0.0', '>=16.0.0', '18.x'
      // Extract major version number
      const match = nodeVersion.match(/(\d+)/);
      match ? match[1] : '18';
    } catch (e) {
      '18';
    }
  " 2>/dev/null || echo "18")
  
  echo "$node_version"
}

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0

# Helper functions
run_test() {
    local test_name="$1"
    local expected="$2"
    local engines_value="$3"
    
    ((TOTAL_TESTS++))
    
    # Create temporary test directory
    local test_dir="/tmp/nodejs-version-test-$$-$TOTAL_TESTS"
    mkdir -p "$test_dir"
    
    # Create package.json with specific engines value
    if [[ "$engines_value" == "NONE" ]]; then
        cat > "$test_dir/package.json" << 'EOF'
{
  "name": "test-app",
  "version": "1.0.0"
}
EOF
    elif [[ "$engines_value" == "EMPTY" ]]; then
        cat > "$test_dir/package.json" << 'EOF'
{
  "name": "test-app",
  "version": "1.0.0",
  "engines": {}
}
EOF
    else
        cat > "$test_dir/package.json" << EOF
{
  "name": "test-app",
  "version": "1.0.0",
  "engines": {
    "node": "$engines_value"
  }
}
EOF
    fi
    
    local result
    result=$(get_nodejs_version_from_package "$test_dir")
    
    if [[ "$result" == "$expected" ]]; then
        echo "PASS: $test_name → v$result"
        ((PASSED_TESTS++))
    else
        echo "FAIL: $test_name → Expected v$expected, got v$result"
    fi
    
    # Cleanup
    rm -rf "$test_dir"
}

echo "Node.js Version Detection Unit Tests"
echo "===================================="

# Test various version formats
run_test "Simple version number" "18" "18"
run_test "Caret range" "20" "^20.0.0"
run_test "Tilde range" "19" "~19.5.0"
run_test "Greater than or equal" "16" ">=16.0.0"
run_test "Version with x wildcard" "18" "18.x"
run_test "Complex range" "14" ">=14.0.0 <20.0.0"
run_test "Prerelease version" "18" "18.0.0-beta"
run_test "No engines field" "18" "NONE"
run_test "Empty engines object" "18" "EMPTY"

echo ""
echo "Summary: $PASSED_TESTS/$TOTAL_TESTS tests passed"

if [[ $PASSED_TESTS -eq $TOTAL_TESTS ]]; then
    echo "✓ All unit tests passed!"
    exit 0
else
    echo "✗ Some tests failed!"
    exit 1
fi