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

# Check common Nomad artifact locations and look for tar files
if [ -d "local" ]; then
    echo "[SETUP] Found 'local' directory (Nomad default artifact location)"
    ls -la local/
    ARTIFACT_DIR="local"
    
    # Check if there's a tar file in the artifact directory
    if [ -f "local/input.tar" ]; then
        echo "[SETUP] Found input.tar in local directory, using it directly"
        # Copy the tar file to workspace where runner script expects it
        cp "local/input.tar" "/workspace/input.tar"
        echo "[SETUP] Copied input.tar to /workspace/input.tar"
        ls -la /workspace/input.tar
        
        # We're done - runner script will extract it
        echo "[SETUP] Workspace setup complete - input.tar ready for runner script!"
        echo "[SETUP] Starting OpenRewrite transformation..."
        exec /usr/local/bin/openrewrite
    fi
    
elif [ -d "artifacts" ]; then
    echo "[SETUP] Found 'artifacts' directory"
    ls -la artifacts/
    ARTIFACT_DIR="artifacts"
else
    echo "[SETUP] No standard artifact directory found, using current directory"
    ARTIFACT_DIR="."
fi

# Create workspace directory only (runner.sh will create project subdirectory)
echo "[SETUP] Creating workspace directory..."
mkdir -p /workspace

# Create input.tar directly from the artifact directory
echo "[SETUP] Creating input.tar for OpenRewrite runner from $ARTIFACT_DIR..."

if [ "$ARTIFACT_DIR" = "." ]; then
    # Create tar from current directory, excluding Nomad directories
    echo "[SETUP] Creating tar from current directory (excluding Nomad dirs)..."
    tar -cf /workspace/input.tar \
        --exclude='./tmp' \
        --exclude='./secrets' \
        --exclude='./local' \
        --exclude='./artifacts' \
        --exclude='./alloc' \
        . 2>/dev/null || {
        echo "[SETUP] Failed to create input.tar"
        exit 1
    }
else
    # Create tar from artifact directory contents
    echo "[SETUP] Creating tar from $ARTIFACT_DIR contents..."
    tar -cf /workspace/input.tar -C "$ARTIFACT_DIR" . 2>/dev/null || {
        echo "[SETUP] Failed to create input.tar"
        exit 1
    }
fi

echo "[SETUP] Created input.tar successfully"
ls -la /workspace/input.tar

# Debug: Show what's in the tar
echo "[SETUP] Input tar contents (first 10 files):"
tar -tvf /workspace/input.tar | head -10

# Verify tar was created with content
FILE_COUNT=$(tar -tf /workspace/input.tar 2>/dev/null | wc -l)
echo "[SETUP] Total files in input.tar: $FILE_COUNT"

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