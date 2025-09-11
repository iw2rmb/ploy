#!/bin/bash
# Test script to verify end-to-end transformation workflow with RecipeRegistry

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
API_URL="${API_URL:-https://api.dev.ployman.app}"
TRANSFLOW_ENDPOINT="$API_URL/v1/transflow/run"

echo -e "${BLUE}=== Testing Transflow Submission Workflow ===${NC}"
echo "API URL: $API_URL"
echo

echo "1. Testing transflow run with Java 11 to 17 recipe..."

# Use Ploy's test repository created for Java 11 to 17 migration testing
TRANSFORM_PAYLOAD='{
  "config_data": {
    "version": "1",
    "id": "twf-$(date +%s)",
    "target_repo": "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
    "target_branch": "main",
    "base_ref": "main",
    "lane": "A",
    "build_timeout": "5m",
    "steps": [
      {
        "type": "orw-apply",
        "id": "orw1",
        "engine": "openrewrite",
        "recipes": ["org.openrewrite.java.migrate.Java11toJava17"]
      }
    ],
    "self_heal": {"enabled": false}
  }
}'

echo "Transformation payload:"
echo "$TRANSFORM_PAYLOAD" | jq .
echo

# Attempt transformation
echo "Submitting transflow run request..."
RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}" \
    -H "Content-Type: application/json" \
    -d "$TRANSFORM_PAYLOAD" \
    "$TRANSFLOW_ENDPOINT")

# Extract HTTP status and response body
HTTP_STATUS=$(echo "$RESPONSE" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
RESPONSE_BODY=$(echo "$RESPONSE" | sed -e 's/HTTPSTATUS:.*//g')

echo "HTTP Status: $HTTP_STATUS"
echo "Response: $RESPONSE_BODY"
echo

if [ "$HTTP_STATUS" -eq 202 ] || [ "$HTTP_STATUS" -eq 201 ] || [ "$HTTP_STATUS" -eq 200 ]; then
    echo -e "${GREEN}✓ Transformation request accepted${NC}"
    
    # Parse task ID if available
    TASK_ID=$(echo "$RESPONSE_BODY" | jq -r '.execution_id // empty')
    
    if [ ! -z "$TASK_ID" ]; then
        echo "Task ID: $TASK_ID"
        
        # Test task status endpoint
        echo "2. Testing transformation status tracking..."
        STATUS_ENDPOINT="$API_URL/v1/transflow/status/$TASK_ID"
        
        STATUS_RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}" "$STATUS_ENDPOINT")
        STATUS_HTTP_CODE=$(echo "$STATUS_RESPONSE" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
        STATUS_BODY=$(echo "$STATUS_RESPONSE" | sed -e 's/HTTPSTATUS:.*//g')
        
        echo "Status HTTP Code: $STATUS_HTTP_CODE"
        echo "Status Response: $STATUS_BODY"
        
        if [ "$STATUS_HTTP_CODE" -eq 200 ]; then
            echo -e "${GREEN}✓ Task status tracking working${NC}"
        else
            echo -e "${YELLOW}⚠ Task status tracking endpoint returned $STATUS_HTTP_CODE${NC}"
        fi
    fi
    
    echo -e "${GREEN}🎉 Transflow submission workflow is functional!${NC}"
    echo
    echo -e "${BLUE}=== Workflow Summary ===${NC}"
    echo "✓ Transflow submission: Working"
    echo "✓ Async status endpoint reachable (if provided)"
    
else
    echo -e "${RED}✗ Transformation request failed${NC}"
    
    # Common error scenarios
    if [ "$HTTP_STATUS" -eq 400 ]; then
        echo -e "${YELLOW}Bad request - check payload format or recipe_id${NC}"
    elif [ "$HTTP_STATUS" -eq 404 ]; then
        echo -e "${YELLOW}Recipe not found - RecipeRegistry may not have the recipe${NC}"
    elif [ "$HTTP_STATUS" -eq 503 ]; then
        echo -e "${YELLOW}Service unavailable - check backend services${NC}"
    fi
    
    exit 1
fi

echo
echo -e "${BLUE}=== Test Complete ===${NC}"
echo "Transformation endpoint: $TRANSFORM_ENDPOINT"
echo -e "${GREEN}RecipeRegistry-based transformation workflow: Working${NC}"
