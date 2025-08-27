#!/bin/bash
# Emscripten C/C++ WebAssembly build script
# This script is used as a reference - actual builds use the integrated builder in api/builders/wasm.go

set -euo pipefail

APP_NAME="${1:-}"
SOURCE_FILE="${2:-main.cpp}"
OUTPUT_FILE="${3:-${APP_NAME}.wasm}"

if [ -z "$APP_NAME" ]; then
    echo "Usage: $0 <app-name> [source-file] [output-file]"
    exit 1
fi

echo "Building C++ WASM application with Emscripten: $APP_NAME"

# Check for source file
if [ ! -f "$SOURCE_FILE" ]; then
    echo "Error: Source file $SOURCE_FILE not found"
    exit 1
fi

# Check if Emscripten is available
if ! command -v emcc >/dev/null 2>&1; then
    echo "Error: Emscripten compiler (emcc) not found"
    exit 1
fi

# Build with Emscripten
echo "Compiling with Emscripten compiler..."
emcc -O3 -s WASM=1 \
     -s EXPORTED_FUNCTIONS='["_main"]' \
     -s EXPORTED_RUNTIME_METHODS='["ccall","cwrap"]' \
     "$SOURCE_FILE" -o "$OUTPUT_FILE"

if [ -f "$OUTPUT_FILE" ]; then
    echo "✓ Built successfully: $OUTPUT_FILE"
    ls -lh "$OUTPUT_FILE"
    
    # Also show any accompanying JS file
    JS_FILE="${OUTPUT_FILE%.wasm}.js"
    if [ -f "$JS_FILE" ]; then
        echo "✓ JavaScript glue code: $JS_FILE"
        ls -lh "$JS_FILE"
    fi
else
    echo "✗ Build failed: WASM file not found"
    exit 1
fi

echo "Emscripten C++ WASM build completed successfully"