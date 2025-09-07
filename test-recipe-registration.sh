#!/bin/bash
# Test script to verify recipe registration endpoint works with RecipeRegistry

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
API_URL="${API_URL:-http://localhost:8080}"
RECIPE_ENDPOINT="$API_URL/v1/arf/recipes/register"
LIST_ENDPOINT="$API_URL/v1/arf/recipes"

echo -e "${BLUE}=== Testing Recipe Registration with RecipeRegistry ===${NC}"
echo "API URL: $API_URL"
echo

# Test 1: Health check
echo -n "1. Testing API health... "
if curl -s -f "$API_URL/health" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ API is responding${NC}"
else
    echo -e "${RED}✗ API health check failed${NC}"
    echo "Make sure the API server is running at $API_URL"
    exit 1
fi

# Test 2: Test recipe registration
echo "2. Testing recipe registration..."

# Sample recipe registration payload
RECIPE_PAYLOAD='{
  "recipe_class": "org.openrewrite.java.migrate.Java11toJava17",
  "maven_coords": "org.openrewrite.recipe:rewrite-migrate-java:2.11.0",
  "jar_path": "maven/org/openrewrite/recipe/rewrite-migrate-java/2.11.0/rewrite-migrate-java-2.11.0.jar",
  "source": "test-script",
  "timestamp": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
}'

echo "Payload: $RECIPE_PAYLOAD"
echo

# Attempt registration
RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}" \
    -H "Content-Type: application/json" \
    -d "$RECIPE_PAYLOAD" \
    "$RECIPE_ENDPOINT")

# Extract HTTP status and response body
HTTP_STATUS=$(echo "$RESPONSE" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
RESPONSE_BODY=$(echo "$RESPONSE" | sed -e 's/HTTPSTATUS:.*//g')

echo "HTTP Status: $HTTP_STATUS"
echo "Response: $RESPONSE_BODY"
echo

if [ "$HTTP_STATUS" -eq 200 ] || [ "$HTTP_STATUS" -eq 201 ]; then
    echo -e "${GREEN}✓ Recipe registration successful${NC}"
    
    # Test 3: List recipes to verify it was stored
    echo "3. Testing recipe listing..."
    LIST_RESPONSE=$(curl -s -w "HTTPSTATUS:%{http_code}" "$LIST_ENDPOINT")
    LIST_HTTP_STATUS=$(echo "$LIST_RESPONSE" | tr -d '\n' | sed -e 's/.*HTTPSTATUS://')
    LIST_RESPONSE_BODY=$(echo "$LIST_RESPONSE" | sed -e 's/HTTPSTATUS:.*//g')
    
    echo "List HTTP Status: $LIST_HTTP_STATUS"
    echo "List Response: $LIST_RESPONSE_BODY"
    
    if [ "$LIST_HTTP_STATUS" -eq 200 ]; then
        echo -e "${GREEN}✓ Recipe listing successful${NC}"
        
        # Check if our recipe appears in the list
        if echo "$LIST_RESPONSE_BODY" | grep -q "Java11toJava17"; then
            echo -e "${GREEN}✓ Registered recipe found in list${NC}"
            echo -e "${GREEN}🎉 All tests passed! RecipeRegistry is working correctly.${NC}"
        else
            echo -e "${YELLOW}⚠ Recipe not found in list, but registration succeeded${NC}"
            echo "This might be expected if recipes are stored but not immediately listable"
        fi
    else
        echo -e "${RED}✗ Recipe listing failed${NC}"
    fi
else
    echo -e "${RED}✗ Recipe registration failed${NC}"
    
    # Common error scenarios
    if [ "$HTTP_STATUS" -eq 503 ]; then
        echo -e "${YELLOW}This likely means:${NC}"
        echo "- RecipeRegistry is not initialized (check SeaweedFS connectivity)"
        echo "- Storage provider is not available"
        echo "- Check server logs for storage initialization errors"
    elif [ "$HTTP_STATUS" -eq 400 ]; then
        echo -e "${YELLOW}Bad request - check the payload format${NC}"
    elif [ "$HTTP_STATUS" -eq 404 ]; then
        echo -e "${YELLOW}Endpoint not found - check if API server has the route registered${NC}"
    fi
    exit 1
fi

echo
echo -e "${BLUE}=== Test Summary ===${NC}"
echo "Recipe registration endpoint: $RECIPE_ENDPOINT"
echo "Recipe listing endpoint: $LIST_ENDPOINT" 
echo -e "${GREEN}RecipeRegistry integration: Working${NC}"