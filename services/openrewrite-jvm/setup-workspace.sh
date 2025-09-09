#!/bin/bash

# OpenRewrite Workspace Setup Script
# This script handles the workspace setup for Nomad-deployed OpenRewrite transformations

set -euo pipefail

echo "[SETUP] Starting OpenRewrite workspace setup..."
echo "[SETUP] Current directory: $(pwd)"
echo "[SETUP] User: $(whoami)"

# Allow test/runtime overrides
WORKSPACE_DIR="${WORKSPACE_DIR:-/workspace}"
CONTEXT_DIR="${CONTEXT_DIR:-${WORKSPACE_DIR}/context}"
SKIP_EXEC_OPENREWRITE="${SKIP_EXEC_OPENREWRITE:-}"

# Debug: Show what Nomad provided
echo "[SETUP] Directory contents:"
ls -la

echo "[SETUP] Checking for Nomad artifact locations..."

# Definitive behavior: Prefer mounted context early and do not fall back
if [ -d "${CONTEXT_DIR}" ]; then
    echo "[SETUP] Found context directory: ${CONTEXT_DIR}"
    ls -la "${CONTEXT_DIR}" | head -20 || true
    ARTIFACT_DIR="${CONTEXT_DIR}"
else
    # Only if no explicit context, search for provided input.tar
    echo "[SETUP] Searching for input.tar file..."
    INPUT_TAR_FOUND=$(find / -name "input.tar" -type f 2>/dev/null | head -1)

    if [ -n "$INPUT_TAR_FOUND" ]; then
        echo "[SETUP] Found input.tar at: $INPUT_TAR_FOUND"
        # Copy the tar file to workspace where runner script expects it
        cp "$INPUT_TAR_FOUND" "${WORKSPACE_DIR}/input.tar"
        echo "[SETUP] Copied input.tar to ${WORKSPACE_DIR}/input.tar"
        ls -la "${WORKSPACE_DIR}/input.tar"
        
        # We're done - runner script will extract it
        echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
        echo "[SETUP] Starting OpenRewrite transformation..."
        if [ "$SKIP_EXEC_OPENREWRITE" = "1" ]; then exit 0; fi
        exec /usr/local/bin/openrewrite
    fi

    # If not found by search, check common locations
    if [ -f "/local/input.tar" ]; then
        echo "[SETUP] Found input.tar at /local/input.tar (Nomad artifact location)"
        cp "/local/input.tar" "${WORKSPACE_DIR}/input.tar"
        echo "[SETUP] Copied input.tar to ${WORKSPACE_DIR}/input.tar"
        ls -la "${WORKSPACE_DIR}/input.tar"
        echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
        echo "[SETUP] Starting OpenRewrite transformation..."
        if [ "$SKIP_EXEC_OPENREWRITE" = "1" ]; then exit 0; fi
        exec /usr/local/bin/openrewrite
    elif [ -f "local/input.tar" ]; then
        echo "[SETUP] Found input.tar at local/input.tar"
        cp "local/input.tar" "${WORKSPACE_DIR}/input.tar"
        echo "[SETUP] Copied input.tar to ${WORKSPACE_DIR}/input.tar"
        ls -la "${WORKSPACE_DIR}/input.tar"
        echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
        echo "[SETUP] Starting OpenRewrite transformation..."
        if [ "$SKIP_EXEC_OPENREWRITE" = "1" ]; then exit 0; fi
        exec /usr/local/bin/openrewrite
    elif [ -d "local" ]; then
        echo "[SETUP] Found 'local' directory but no input.tar"
        ls -la local/ || true
        ARTIFACT_DIR="local"
    elif [ -d "artifacts" ]; then
        echo "[SETUP] Found 'artifacts' directory"
        ls -la artifacts/ || true
        ARTIFACT_DIR="artifacts"
    else
        echo "[SETUP] No standard artifact directory found, using current directory"
        ARTIFACT_DIR="."
    fi
fi

# Determine build root containing pom.xml or Gradle files
BUILD_DIR="$ARTIFACT_DIR"
if [ -d "$ARTIFACT_DIR" ]; then
  if [ ! -f "$ARTIFACT_DIR/pom.xml" ] && [ ! -f "$ARTIFACT_DIR/build.gradle" ] && [ ! -f "$ARTIFACT_DIR/build.gradle.kts" ]; then
    echo "[SETUP] Searching for build root under $ARTIFACT_DIR..."
    CANDIDATE=$(find "$ARTIFACT_DIR" -maxdepth 3 -type f \( -name pom.xml -o -name build.gradle -o -name build.gradle.kts \) 2>/dev/null | head -1 || true)
    if [ -n "$CANDIDATE" ]; then
      BUILD_DIR=$(dirname "$CANDIDATE")
    fi
  fi
fi

echo "[SETUP] Selected build directory: $BUILD_DIR"

# Create OpenRewrite expected workspace structure and copy build root
echo "[SETUP] Creating workspace structure..."
rm -rf "${WORKSPACE_DIR}/project"
mkdir -p "${WORKSPACE_DIR}/project"
if [ -d "$BUILD_DIR" ]; then
  cp -r "$BUILD_DIR"/* "${WORKSPACE_DIR}/project/" 2>/dev/null || true
fi

# Create input.tar for OpenRewrite runner
echo "[SETUP] Creating input.tar for OpenRewrite runner..."
cd "${WORKSPACE_DIR}/project"
FILE_COUNT=$(find . -type f | wc -l)
if [ "$FILE_COUNT" -gt 0 ]; then
  tar -cf "${WORKSPACE_DIR}/input.tar" . 2>/dev/null || {
    echo "[SETUP] Failed to create input.tar"
    exit 1
  }
  echo "[SETUP] Created input.tar successfully"
  ls -la "${WORKSPACE_DIR}/input.tar"
else
  echo "[SETUP] ERROR: No files found to tar in /workspace/project"
  exit 1
fi

# Verify workspace contents
echo "[SETUP] Workspace contents:"
ls -la "${WORKSPACE_DIR}/project/" | head -20

# Check if we have any files
FILE_COUNT=$(find "${WORKSPACE_DIR}/project" -type f | wc -l)
echo "[SETUP] Total files in workspace: $FILE_COUNT"

if [ "$FILE_COUNT" -eq 0 ]; then
    echo "[SETUP] WARNING: No files found in input.tar!"
    echo "[SETUP] This might indicate an issue with artifact extraction"
    
    # Additional debugging
    echo "[SETUP] Full filesystem exploration:"
    find /alloc -name "*.tar" 2>/dev/null | head -5 || true
    find . -name "*.tar" 2>/dev/null | head -5 || true
fi

# Set proper permissions on workspace
chown -R $(whoami):$(whoami) "${WORKSPACE_DIR}/" 2>/dev/null || true
chmod -R 755 "${WORKSPACE_DIR}/" 2>/dev/null || true

echo "[SETUP] Workspace setup complete!"
echo "[SETUP] Starting OpenRewrite transformation..."

# Execute the original OpenRewrite entrypoint
if [ "$SKIP_EXEC_OPENREWRITE" = "1" ]; then exit 0; fi
exec /usr/local/bin/openrewrite
