#!/bin/bash

# OpenRewrite Test: Remove Unused Imports
# This test verifies that transformations actually modify code and produce tangible results

set -e

API_URL="https://api.dev.ployman.app"
TEST_REPO="https://github.com/iw2rmb/ploy-orw-test-java.git"
RECIPE_ID="org.openrewrite.java.RemoveUnusedImports"

echo "=========================================="
echo "OpenRewrite Test: Remove Unused Imports"
echo "=========================================="
echo

# Step 1: Clone test repository and capture initial state
echo "📥 Cloning test repository..."
TEST_DIR="/tmp/orw-test-$$"
rm -rf "$TEST_DIR"
git clone "$TEST_REPO" "$TEST_DIR" >/dev/null 2>&1

echo "📸 Capturing initial state of Application.java..."
BEFORE_CHECKSUM=$(md5sum "$TEST_DIR/src/main/java/com/example/Application.java" | cut -d' ' -f1)
echo "  Initial checksum: $BEFORE_CHECKSUM"

# Count unused imports in Application.java (should have unused: ArrayList, HashMap, Set, HashSet, IOException, File)
UNUSED_IMPORTS_BEFORE=$(grep -c "^import.*\(ArrayList\|HashMap\|Set\|HashSet\|IOException\|File\)" "$TEST_DIR/src/main/java/com/example/Application.java" || true)
echo "  Unused imports found: $UNUSED_IMPORTS_BEFORE"

# Step 2: Start transformation
echo
echo "🚀 Starting transformation..."
TRANSFORM_RESPONSE=$(curl -s -X POST "$API_URL/v1/arf/transforms" \
  -H "Content-Type: application/json" \
  -d "{
    \"recipe_id\": \"$RECIPE_ID\",
    \"type\": \"openrewrite\",
    \"codebase\": {
      \"repository\": \"$TEST_REPO\",
      \"branch\": \"main\"
    }
  }")

TRANSFORM_ID=$(echo "$TRANSFORM_RESPONSE" | jq -r '.transformation_id // .id')
JOB_NAME=$(echo "$TRANSFORM_RESPONSE" | jq -r '.job_id // ""')

if [ "$TRANSFORM_ID" == "null" ] || [ -z "$TRANSFORM_ID" ]; then
  echo "❌ Failed to start transformation"
  echo "$TRANSFORM_RESPONSE"
  exit 1
fi

# If job_name is not in response, we'll need to get it from status
if [ -z "$JOB_NAME" ] || [ "$JOB_NAME" == "null" ]; then
  sleep 2
  STATUS_RESPONSE=$(curl -s "$API_URL/v1/arf/transforms/$TRANSFORM_ID/status")
  JOB_NAME=$(echo "$STATUS_RESPONSE" | jq -r '.job_id // ""')
fi

echo "  Transformation ID: $TRANSFORM_ID"
echo "  Job Name: $JOB_NAME"

# Step 3: Monitor transformation status
echo
echo "⏳ Monitoring transformation..."
MAX_WAIT=60
ELAPSED=0

while [ $ELAPSED -lt $MAX_WAIT ]; do
  STATUS_RESPONSE=$(curl -s "$API_URL/v1/arf/transforms/$TRANSFORM_ID/status")
  STATUS=$(echo "$STATUS_RESPONSE" | jq -r '.status')
  
  if [ "$STATUS" == "completed" ]; then
    echo "✅ Transformation completed"
    break
  elif [ "$STATUS" == "failed" ]; then
    echo "❌ Transformation failed"
    echo "$STATUS_RESPONSE" | jq '.'
    exit 1
  fi
  
  echo -n "."
  sleep 2
  ELAPSED=$((ELAPSED + 2))
done

if [ "$STATUS" != "completed" ]; then
  echo
  echo "❌ Transformation timed out (status: $STATUS)"
  exit 1
fi

# Step 4: Get transformation details
echo
echo "📊 Getting transformation details..."
DETAILS=$(curl -s "$API_URL/v1/arf/transforms/$TRANSFORM_ID")
CHANGES_APPLIED=$(echo "$DETAILS" | jq -r '.changes_applied // 0')
SUCCESS=$(echo "$DETAILS" | jq -r '.success')

echo "  Success: $SUCCESS"
echo "  Changes applied: $CHANGES_APPLIED"

if [ "$CHANGES_APPLIED" -eq 0 ]; then
  echo "⚠️  Warning: No changes reported by transformation"
fi

# Step 5: Download and verify output
echo
echo "📦 Downloading transformed code..."
OUTPUT_DIR="/tmp/orw-output-$$"
mkdir -p "$OUTPUT_DIR"

# Try to get the output via the API
echo "  Attempting to download output.tar..."
HTTP_CODE=$(curl -s -o "$OUTPUT_DIR/output.tar" -w "%{http_code}" \
  "$API_URL/v1/arf/transforms/$TRANSFORM_ID/download")

if [ "$HTTP_CODE" != "200" ]; then
  echo "  ⚠️ Download via API failed (HTTP $HTTP_CODE)"
  
  # Alternative: Try direct storage access
  echo "  Attempting direct storage access..."
  STORAGE_URL="$API_URL/storage/artifacts/jobs/$JOB_NAME/output.tar"
  HTTP_CODE=$(curl -s -o "$OUTPUT_DIR/output.tar" -w "%{http_code}" "$STORAGE_URL")
  
  if [ "$HTTP_CODE" != "200" ]; then
    echo "  ❌ Failed to download output (HTTP $HTTP_CODE)"
    
    # Debug: Check if file exists in storage
    echo
    echo "🔍 Debug: Checking storage..."
    curl -s "$API_URL/storage/artifacts/jobs/$JOB_NAME/" | head -20
    
    exit 1
  fi
fi

# Check if tar file has content
TAR_SIZE=$(stat -f%z "$OUTPUT_DIR/output.tar" 2>/dev/null || stat -c%s "$OUTPUT_DIR/output.tar" 2>/dev/null || echo "0")
echo "  Downloaded tar size: $TAR_SIZE bytes"

if [ "$TAR_SIZE" -eq 0 ]; then
  echo "  ❌ Downloaded tar file is empty"
  exit 1
fi

# Extract and verify contents
echo
echo "📂 Extracting transformed code..."
cd "$OUTPUT_DIR"
tar -xf output.tar 2>/dev/null || {
  echo "  ❌ Failed to extract tar file"
  echo "  Tar file details:"
  file output.tar
  tar -tvf output.tar 2>&1 | head -20 || true
  exit 1
}

# Find the Application.java file
APP_FILE=$(find . -name "Application.java" -type f | head -1)

if [ -z "$APP_FILE" ]; then
  echo "  ❌ Application.java not found in output"
  echo "  Contents of output directory:"
  ls -la
  find . -type f | head -20
  exit 1
fi

echo "  Found Application.java at: $APP_FILE"

# Step 6: Verify changes
echo
echo "🔍 Verifying changes..."
AFTER_CHECKSUM=$(md5sum "$APP_FILE" | cut -d' ' -f1)
echo "  After checksum: $AFTER_CHECKSUM"

if [ "$BEFORE_CHECKSUM" == "$AFTER_CHECKSUM" ]; then
  echo "  ❌ No changes detected - files are identical!"
  echo
  echo "  Showing first 50 lines of transformed file:"
  head -50 "$APP_FILE"
  exit 1
fi

echo "  ✅ File has been modified"

# Check if unused imports were removed
UNUSED_IMPORTS_AFTER=$(grep -c "^import.*\(ArrayList\|HashMap\|Set\|HashSet\|IOException\|File\)" "$APP_FILE" || true)
echo "  Unused imports after: $UNUSED_IMPORTS_AFTER (was $UNUSED_IMPORTS_BEFORE)"

if [ "$UNUSED_IMPORTS_AFTER" -lt "$UNUSED_IMPORTS_BEFORE" ]; then
  echo "  ✅ Unused imports were removed!"
  REMOVED=$((UNUSED_IMPORTS_BEFORE - UNUSED_IMPORTS_AFTER))
  echo "  Removed $REMOVED unused import(s)"
else
  echo "  ⚠️ Unused imports were not removed as expected"
fi

# Show diff
echo
echo "📝 Changes made:"
echo "---"
diff -u "$TEST_DIR/src/main/java/com/example/Application.java" "$APP_FILE" | head -50 || true
echo "---"

# Step 7: Summary
echo
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo "✅ Transformation completed successfully"
echo "✅ Output.tar downloaded and extracted"
if [ "$BEFORE_CHECKSUM" != "$AFTER_CHECKSUM" ]; then
  echo "✅ Code was modified"
else
  echo "❌ Code was NOT modified"
fi
if [ "$UNUSED_IMPORTS_AFTER" -lt "$UNUSED_IMPORTS_BEFORE" ]; then
  echo "✅ Unused imports were removed"
else
  echo "⚠️ Unused imports not fully removed"
fi
echo

# Cleanup
rm -rf "$TEST_DIR" "$OUTPUT_DIR"

echo "Test complete!"