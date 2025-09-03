#!/bin/bash

# OpenRewrite Workspace Setup Script
# This script handles the workspace setup for Nomad-deployed OpenRewrite transformations

set -euo pipefail

echo "[SETUP] Starting OpenRewrite workspace setup..."
echo "[SETUP] Current directory: $(pwd)"
echo "[SETUP] User: $(whoami)"

# Debug: Show what Nomad provided
echo "[SETUP] Directory contents:"
ls -la

echo "[SETUP] Checking for Nomad artifact locations..."

# First, let's search for input.tar anywhere in the filesystem
echo "[SETUP] Searching for input.tar file..."
INPUT_TAR_FOUND=$(find / -name "input.tar" -type f 2>/dev/null | head -1)

if [ -n "$INPUT_TAR_FOUND" ]; then
    echo "[SETUP] Found input.tar at: $INPUT_TAR_FOUND"
    # Copy the tar file to workspace where runner script expects it
    cp "$INPUT_TAR_FOUND" "/workspace/input.tar"
    echo "[SETUP] Copied input.tar to /workspace/input.tar"
    ls -la /workspace/input.tar
    
    # We're done - runner script will extract it
    echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
    echo "[SETUP] Starting OpenRewrite transformation..."
    exec /usr/local/bin/openrewrite
fi

# If not found by search, check common locations
if [ -f "/local/input.tar" ]; then
    echo "[SETUP] Found input.tar at /local/input.tar (Nomad artifact location)"
    cp "/local/input.tar" "/workspace/input.tar"
    echo "[SETUP] Copied input.tar to /workspace/input.tar"
    ls -la /workspace/input.tar
    echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
    echo "[SETUP] Starting OpenRewrite transformation..."
    exec /usr/local/bin/openrewrite
elif [ -f "local/input.tar" ]; then
    echo "[SETUP] Found input.tar at local/input.tar"
    cp "local/input.tar" "/workspace/input.tar"
    echo "[SETUP] Copied input.tar to /workspace/input.tar"
    ls -la /workspace/input.tar
    echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
    echo "[SETUP] Starting OpenRewrite transformation..."
    exec /usr/local/bin/openrewrite
elif [ -d "local" ]; then
    echo "[SETUP] Found 'local' directory but no input.tar"
    ls -la local/
    ARTIFACT_DIR="local"
elif [ -d "artifacts" ]; then
    echo "[SETUP] Found 'artifacts' directory"
    ls -la artifacts/
    ARTIFACT_DIR="artifacts"
else
    echo "[SETUP] No standard artifact directory found, using current directory"
    ARTIFACT_DIR="."
fi

# Create OpenRewrite expected workspace structure
echo "[SETUP] Creating workspace structure..."
mkdir -p /workspace/project

# Copy extracted files to workspace and create input.tar that runner script expects
echo "[SETUP] Copying files from $ARTIFACT_DIR to /workspace/project..."
if [ "$ARTIFACT_DIR" = "." ]; then
    # Copy all files except known Nomad directories
    find . -maxdepth 1 -type f -exec cp {} /workspace/project/ \; 2>/dev/null || true
    find . -maxdepth 1 -type d ! -name . ! -name tmp ! -name secrets -exec cp -r {} /workspace/project/ \; 2>/dev/null || true
else
    cp -r "$ARTIFACT_DIR"/* /workspace/project/ 2>/dev/null || true
fi

# Create input.tar that the OpenRewrite runner script expects
echo "[SETUP] Creating input.tar for OpenRewrite runner..."
cd /workspace/project
if [ $(find . -type f | wc -l) -gt 0 ]; then
    tar -cf /workspace/input.tar . 2>/dev/null || {
        echo "[SETUP] Failed to create input.tar"
        exit 1
    }
    echo "[SETUP] Created input.tar successfully"
    ls -la /workspace/input.tar
else
    echo "[SETUP] ERROR: No files found to tar in /workspace/project"
    exit 1
fi

# Verify workspace contents
echo "[SETUP] Workspace contents:"
ls -la /workspace/project/ | head -20

# Check if we have any files
FILE_COUNT=$(find /workspace/project -type f | wc -l)
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
chown -R $(whoami):$(whoami) /workspace/ 2>/dev/null || true
chmod -R 755 /workspace/ 2>/dev/null || true

echo "[SETUP] Workspace setup complete!"
echo "[SETUP] Starting OpenRewrite transformation..."

# Execute the original OpenRewrite entrypoint
exec /usr/local/bin/openrewrite