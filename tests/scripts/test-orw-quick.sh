#!/bin/bash

# Quick OpenRewrite test to verify transformations work

set -e

API_URL="https://api.dev.ployman.app"

echo "=== Quick OpenRewrite Test ==="
echo ""

# Test Java 11 migration which we know works
echo "Starting transformation: Java8toJava11"
response=$(curl -s -X POST "$API_URL/v1/arf/transforms" \
    -H "Content-Type: application/json" \
    -d '{
        "recipe_id": "org.openrewrite.java.migrate.Java8toJava11",
        "type": "openrewrite",
        "codebase": {
            "repository": "https://github.com/iw2rmb/ploy-orw-test-java.git",
            "branch": "main"
        }
    }')

transform_id=$(echo "$response" | jq -r '.transformation_id')
echo "Transform ID: $transform_id"

# Poll for completion (max 60 seconds)
for i in {1..30}; do
    sleep 2
    status=$(curl -s "$API_URL/v1/arf/transforms/$transform_id/status" | jq -r '.status')
    
    if [ "$status" = "completed" ]; then
        echo "Status: COMPLETED"
        
        # Get transformation details
        details=$(curl -s "$API_URL/v1/arf/transforms/$transform_id")
        
        # Check for diff
        has_diff=$(echo "$details" | jq -r 'has("diff")')
        if [ "$has_diff" = "true" ]; then
            echo "✅ Code changes detected!"
            echo ""
            echo "Diff preview:"
            echo "$details" | jq -r '.diff' | head -30
            echo ""
            echo "SUCCESS: Transformation applied actual code changes"
        else
            echo "⚠️  No diff found in response"
        fi
        
        exit 0
    fi
    
    echo -n "."
done

echo ""
echo "Timeout waiting for transformation"
exit 1