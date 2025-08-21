#!/bin/bash
# AssemblyScript WebAssembly build script
# This script is used as a reference - actual builds use the integrated builder in controller/builders/wasm.go

set -euo pipefail

APP_NAME="${1:-}"
OUTPUT_DIR="${2:-build}"

if [ -z "$APP_NAME" ]; then
    echo "Usage: $0 <app-name> [output-dir]"
    exit 1
fi

echo "Building AssemblyScript WASM application: $APP_NAME"

# Check for package.json and AssemblyScript configuration
if [ ! -f "package.json" ]; then
    echo "Error: package.json not found"
    exit 1
fi

if ! grep -q "assemblyscript" package.json; then
    echo "Error: AssemblyScript not found in dependencies"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Build with AssemblyScript compiler
echo "Compiling with AssemblyScript compiler..."
if command -v asc >/dev/null 2>&1; then
    asc assembly/index.ts --target release --outFile "$OUTPUT_DIR/${APP_NAME}.wasm"
elif npx asc --version >/dev/null 2>&1; then
    npx asc assembly/index.ts --target release --outFile "$OUTPUT_DIR/${APP_NAME}.wasm"
else
    echo "Error: AssemblyScript compiler not found"
    exit 1
fi

WASM_FILE="$OUTPUT_DIR/${APP_NAME}.wasm"
if [ -f "$WASM_FILE" ]; then
    echo "✓ Built successfully: $WASM_FILE"
    ls -lh "$WASM_FILE"
else
    echo "✗ Build failed: WASM file not found"
    exit 1
fi

echo "AssemblyScript WASM build completed successfully"