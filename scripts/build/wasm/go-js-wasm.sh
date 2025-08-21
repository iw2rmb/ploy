#!/bin/bash
# Go WebAssembly build script for js/wasm target
# This script is used as a reference - actual builds use the integrated builder in controller/builders/wasm.go

set -euo pipefail

APP_NAME="${1:-}"
OUTPUT_FILE="${2:-${APP_NAME}.wasm}"

if [ -z "$APP_NAME" ]; then
    echo "Usage: $0 <app-name> [output-file]"
    exit 1
fi

echo "Building Go WASM application: $APP_NAME"

# Check for Go module
if [ ! -f "go.mod" ] && [ ! -f "main.go" ]; then
    echo "Error: No Go module or main.go found"
    exit 1
fi

# Build for js/wasm target
echo "Compiling with Go for js/wasm target..."
GOOS=js GOARCH=wasm go build -o "$OUTPUT_FILE" .

if [ -f "$OUTPUT_FILE" ]; then
    echo "✓ Built successfully: $OUTPUT_FILE"
    ls -lh "$OUTPUT_FILE"
else
    echo "✗ Build failed: WASM file not found"
    exit 1
fi

echo "Go WASM build completed successfully"