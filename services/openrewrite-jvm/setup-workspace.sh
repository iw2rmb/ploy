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

# Check common Nomad artifact locations
if [ -d "local" ]; then
    echo "[SETUP] Found 'local' directory (Nomad default artifact location)"
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

# Copy extracted files to workspace
echo "[SETUP] Copying files from $ARTIFACT_DIR to /workspace/project..."
if [ "$ARTIFACT_DIR" = "." ]; then
    # Copy all files except known Nomad directories
    find . -maxdepth 1 -type f -exec cp {} /workspace/project/ \; 2>/dev/null || true
    find . -maxdepth 1 -type d ! -name . ! -name tmp ! -name secrets -exec cp -r {} /workspace/project/ \; 2>/dev/null || true
else
    cp -r "$ARTIFACT_DIR"/* /workspace/project/ 2>/dev/null || true
fi

# Verify workspace contents
echo "[SETUP] Workspace contents:"
ls -la /workspace/project/ | head -20

# Check if we have any files
FILE_COUNT=$(find /workspace/project -type f | wc -l)
echo "[SETUP] Total files in workspace: $FILE_COUNT"

if [ "$FILE_COUNT" -eq 0 ]; then
    echo "[SETUP] WARNING: No files found in workspace!"
    echo "[SETUP] This might indicate an issue with artifact extraction"
    
    # Additional debugging
    echo "[SETUP] Full filesystem exploration:"
    find /alloc -name "*.tar" 2>/dev/null | head -5 || true
    find . -name "*.tar" 2>/dev/null | head -5 || true
fi

# Set proper permissions
chown -R $(whoami):$(whoami) /workspace/project/ 2>/dev/null || true
chmod -R 755 /workspace/project/ 2>/dev/null || true

echo "[SETUP] Workspace setup complete!"
echo "[SETUP] Starting OpenRewrite transformation..."

# Execute the original OpenRewrite entrypoint
exec /usr/local/bin/openrewrite