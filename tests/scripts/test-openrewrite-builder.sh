#!/bin/bash
# Test script for OpenRewrite dynamic image builder

set -e

echo "Testing OpenRewrite Dynamic Image Builder"
echo "========================================="

# Configuration
RECIPES='["java11to17", "jakarta"]'
PACKAGE_MANAGER="maven"
BASE_JDK="17"
TEST_PROJECT="/tmp/test-java11-migration"

# API endpoint (use local controller on VPS)
API_ENDPOINT="${PLOY_CONTROLLER:-http://localhost:8081/v1}"

echo "Configuration:"
echo "  API Endpoint: $API_ENDPOINT"
echo "  Recipes: $RECIPES"
echo "  Package Manager: $PACKAGE_MANAGER"
echo "  Base JDK: $BASE_JDK"
echo ""

# Step 1: Validate recipes
echo "1. Validating recipes..."
VALIDATE_RESPONSE=$(curl -s -X POST "$API_ENDPOINT/arf/openrewrite/validate" \
  -H "Content-Type: application/json" \
  -d "{\"recipes\": $RECIPES}")

if ! echo "$VALIDATE_RESPONSE" | grep -q '"valid":true'; then
  echo "Recipe validation failed:"
  echo "$VALIDATE_RESPONSE"
  exit 1
fi
echo "✅ Recipes validated successfully"

# Step 2: Generate image name
echo ""
echo "2. Generating image name..."
NAME_RESPONSE=$(curl -s -X POST "$API_ENDPOINT/arf/openrewrite/generate-name" \
  -H "Content-Type: application/json" \
  -d "{
    \"recipes\": $RECIPES,
    \"package_manager\": \"$PACKAGE_MANAGER\"
  }")

IMAGE_NAME=$(echo "$NAME_RESPONSE" | grep -o '"image_name":"[^"]*' | cut -d'"' -f4)
FULL_IMAGE=$(echo "$NAME_RESPONSE" | grep -o '"full_image":"[^"]*' | cut -d'"' -f4)

echo "Generated image name: $IMAGE_NAME"
echo "Full image path: $FULL_IMAGE"

# Step 3: Build the image
echo ""
echo "3. Building custom image..."
BUILD_RESPONSE=$(curl -s -X POST "$API_ENDPOINT/arf/openrewrite/build" \
  -H "Content-Type: application/json" \
  -d "{
    \"recipes\": $RECIPES,
    \"package_manager\": \"$PACKAGE_MANAGER\",
    \"base_jdk\": \"$BASE_JDK\",
    \"force\": false
  }")

if echo "$BUILD_RESPONSE" | grep -q '"error"'; then
  echo "Build failed:"
  echo "$BUILD_RESPONSE"
  exit 1
fi

CACHED=$(echo "$BUILD_RESPONSE" | grep -o '"cached":[^,]*' | cut -d':' -f2)
BUILD_TIME=$(echo "$BUILD_RESPONSE" | grep -o '"build_time":"[^"]*' | cut -d'"' -f4)
SIZE=$(echo "$BUILD_RESPONSE" | grep -o '"size":[^,}]*' | cut -d':' -f2)

echo "✅ Image built successfully"
echo "  Cached: $CACHED"
echo "  Build time: $BUILD_TIME"
if [ "$SIZE" != "null" ] && [ "$SIZE" != "" ]; then
  SIZE_MB=$(echo "scale=2; $SIZE / 1048576" | bc 2>/dev/null || echo "N/A")
  echo "  Size: ${SIZE_MB} MB"
fi

# Step 4: Test transformation (if test project exists)
if [ -d "$TEST_PROJECT" ]; then
  echo ""
  echo "4. Testing transformation..."
  
  # Create tar of test project
  cd $(dirname "$TEST_PROJECT")
  tar -czf /tmp/test-project.tar.gz $(basename "$TEST_PROJECT")
  
  # Upload to storage or use local path
  echo "Project prepared for transformation"
  
  # Submit transformation job
  TRANSFORM_RESPONSE=$(curl -s -X POST "$API_ENDPOINT/arf/openrewrite/transform" \
    -H "Content-Type: application/json" \
    -d "{
      \"project_url\": \"file:///tmp/test-project.tar.gz\",
      \"recipes\": $RECIPES,
      \"package_manager\": \"$PACKAGE_MANAGER\",
      \"base_jdk\": \"$BASE_JDK\"
    }")
  
  JOB_ID=$(echo "$TRANSFORM_RESPONSE" | grep -o '"job_id":"[^"]*' | cut -d'"' -f4)
  
  if [ -n "$JOB_ID" ]; then
    echo "✅ Transformation job submitted: $JOB_ID"
    
    # Wait for job completion (with timeout)
    echo "Waiting for job completion..."
    for i in {1..30}; do
      sleep 2
      STATUS_RESPONSE=$(curl -s "$API_ENDPOINT/arf/openrewrite/status/$JOB_ID")
      STATUS=$(echo "$STATUS_RESPONSE" | grep -o '"status":"[^"]*' | cut -d'"' -f4)
      
      if [ "$STATUS" = "completed" ]; then
        echo "✅ Transformation completed successfully"
        break
      elif [ "$STATUS" = "failed" ]; then
        echo "❌ Transformation failed:"
        echo "$STATUS_RESPONSE"
        exit 1
      fi
      
      echo -n "."
    done
  else
    echo "⚠️ Could not submit transformation job"
  fi
else
  echo ""
  echo "4. Skipping transformation test (test project not found)"
fi

# Step 5: List available recipes
echo ""
echo "5. Listing available recipes..."
RECIPES_LIST=$(curl -s "$API_ENDPOINT/arf/openrewrite/recipes")
TOTAL=$(echo "$RECIPES_LIST" | grep -o '"total":[^,}]*' | cut -d':' -f2)
echo "Total available recipes: $TOTAL"

# Show categories
echo ""
echo "Recipe categories:"
for category in java-migration spring testing logging; do
  COUNT=$(curl -s "$API_ENDPOINT/arf/openrewrite/recipes?category=$category" | \
    grep -o '"total":[^,}]*' | cut -d':' -f2)
  echo "  - $category: $COUNT recipes"
done

echo ""
echo "======================================="
echo "OpenRewrite Builder Test Complete!"
echo "======================================="