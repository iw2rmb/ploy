#!/bin/bash

# Simple OpenRewrite Java 17 Test
# Directly monitors Nomad job execution rather than relying on API status endpoint

set -e

echo "=== Simple OpenRewrite Java 17 Test ==="
echo "Target host: ${TARGET_HOST:-45.12.75.241}"
echo

# Configuration
TARGET_HOST="${TARGET_HOST:-45.12.75.241}"
PLOY_CONTROLLER="${PLOY_CONTROLLER:-https://api.dev.ployman.app/v1}"

# Test single repository
echo "Testing with java8-tutorial repository..."

# Submit transformation request with verbose output
echo "Submitting transformation request..."
RESPONSE=$(curl -s -X POST "$PLOY_CONTROLLER/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/winterbe/java8-tutorial.git",
      "branch": "master",
      "language": "java",
      "build_tool": "maven"
    }
  }' \
  --max-time 15 2>&1 || echo '{"error": "timeout"}')

echo "Response: $RESPONSE"

# Extract transformation ID if available
TRANSFORM_ID=$(echo "$RESPONSE" | grep -o '"transformation_id":"[^"]*"' | cut -d'"' -f4 || echo "")

if [ -n "$TRANSFORM_ID" ]; then
    echo "Transformation ID: $TRANSFORM_ID"
    
    # Wait a moment for job to be created
    sleep 5
    
    # Check for OpenRewrite jobs in Nomad
    echo "Checking for OpenRewrite jobs in Nomad..."
    ssh root@$TARGET_HOST "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh status --job openrewrite-*'" 2>/dev/null || echo "No OpenRewrite jobs found"
    
    # List all recent jobs
    echo "Recent Nomad jobs:"
    ssh root@$TARGET_HOST "su - ploy -c 'nomad job status | head -20'" 2>/dev/null || true
else
    echo "No transformation ID received, checking Nomad directly..."
    
    # Check for any OpenRewrite-related activity
    ssh root@$TARGET_HOST "su - ploy -c 'nomad job status | grep -i openrewrite'" 2>/dev/null || echo "No OpenRewrite jobs found"
fi

echo "Test complete"