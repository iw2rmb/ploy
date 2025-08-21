#!/bin/bash
# Rust WebAssembly build script for wasm32-wasi target
# This script is used as a reference - actual builds use the integrated builder in controller/builders/wasm.go

set -euo pipefail

APP_NAME="${1:-}"
BUILD_DIR="${2:-./target/wasm32-wasi/release}"

if [ -z "$APP_NAME" ]; then
    echo "Usage: $0 <app-name> [build-dir]"
    exit 1
fi

echo "Building Rust WASM application: $APP_NAME"

# Check for Cargo.toml
if [ ! -f "Cargo.toml" ]; then
    echo "Error: Cargo.toml not found"
    exit 1
fi

# Build for wasm32-wasi target
echo "Compiling with cargo for wasm32-wasi target..."
cargo build --target wasm32-wasi --release

# Find the generated WASM file
WASM_FILE="$BUILD_DIR/${APP_NAME}.wasm"
if [ ! -f "$WASM_FILE" ]; then
    # Try common alternative names
    for alt in "${BUILD_DIR}/${APP_NAME/_/-}.wasm" "${BUILD_DIR}/lib${APP_NAME}.wasm"; do
        if [ -f "$alt" ]; then
            WASM_FILE="$alt"
            break
        fi
    done
fi

if [ -f "$WASM_FILE" ]; then
    echo "✓ Built successfully: $WASM_FILE"
    ls -lh "$WASM_FILE"
else
    echo "✗ Build failed: WASM file not found"
    exit 1
fi

echo "Rust WASM build completed successfully"