# WASM.md — WebAssembly Compilation Detection Analysis

## Overview

This document provides comprehensive analysis for detecting when code MUST, COULD, or COULD NOT be compiled to WebAssembly (WASM) for Ploy's Lane G deployment target. The detection logic determines whether a project should automatically be routed to the WASM runtime lane.

## Detection Categories

### MUST Compile as WASM
Projects that explicitly target WebAssembly and have no alternative compilation path.

### COULD Compile as WASM
Projects that can be compiled to WASM but may have native alternatives or require configuration changes.

### COULD NOT Compile as WASM
Projects that fundamentally cannot be compiled to WASM due to language limitations, system dependencies, or architectural constraints.

## Language-Specific Detection

### Rust

#### MUST Compile as WASM
- **Cargo.toml contains explicit WASM target**:
  ```toml
  [lib]
  crate-type = ["cdylib"]
  
  [dependencies]
  wasm-bindgen = "*"
  ```
- **WASM-specific dependencies present**:
  - `wasm-bindgen` (core WASM binding generator)
  - `js-sys` (JavaScript API bindings)
  - `web-sys` (Web API bindings)
  - `wasi` (WebAssembly System Interface)
  - `wasmtime` (WASM runtime bindings)
- **Build scripts targeting wasm32**:
  ```bash
  cargo build --target wasm32-unknown-unknown
  cargo build --target wasm32-wasi
  ```
- **wasm-pack configuration**:
  ```toml
  [package.metadata.wasm-pack.profile.release]
  wee_alloc = false
  ```

#### COULD Compile as WASM
- **Pure Rust code without system dependencies**
- **#[no_std] crates** (can often compile to WASM)
- **Computational libraries** (math, crypto, data processing)
- **Game logic libraries**
- **Parser/serialization libraries**

#### COULD NOT Compile as WASM
- **Direct system call usage** (`std::os`, `libc` calls)
- **File system operations** beyond WASI capabilities
- **Network sockets** (raw socket operations)
- **Threading with std::thread** (WASM has limited threading)
- **Dynamic library loading** (`libloading`, `dlopen`)
- **Platform-specific code** (`#[cfg(target_os = "linux")]`)

### Go

#### MUST Compile as WASM
- **Explicit WASM build tags**:
  ```go
  // +build js,wasm
  package main
  ```
- **syscall/js imports**:
  ```go
  import "syscall/js"
  ```
- **Build instructions for WASM**:
  ```bash
  GOOS=js GOARCH=wasm go build
  ```

#### COULD Compile as WASM
- **Pure computational code**
- **JSON/XML processing**
- **Cryptographic operations**
- **Data structures and algorithms**
- **HTTP client code** (with WASI HTTP support)

#### COULD NOT Compile as WASM
- **CGO usage** (`import "C"`)
- **OS-specific packages** (`os/exec`, `os/signal`)
- **Network server code** (net/http servers)
- **File system operations** beyond WASI
- **Goroutines with complex synchronization**
- **Reflection-heavy code** (may not work reliably)

### C/C++

#### MUST Compile as WASM
- **Emscripten configuration files**:
  - `.emscripten` config file
  - `CMakeLists.txt` with Emscripten toolchain
  - Makefile with `emcc` compiler
- **Emscripten-specific headers**:
  ```c
  #include <emscripten.h>
  #include <emscripten/bind.h>
  ```
- **WASM export attributes**:
  ```c
  EMSCRIPTEN_KEEPALIVE
  extern "C" {
      int my_function();
  }
  ```

#### COULD Compile as WASM
- **Pure C libraries** without system dependencies
- **Mathematical computation code**
- **Image/audio processing algorithms**
- **Game engines** (with Emscripten port)
- **Compression/decompression libraries**

#### COULD NOT Compile as WASM
- **Direct system calls** (`syscall()`, `ioctl()`)
- **POSIX threading** (`pthread` with complex synchronization)
- **Memory-mapped files** (`mmap`)
- **Dynamic library loading** (`dlopen`, `LoadLibrary`)
- **Assembly code** (inline assembly)
- **Hardware-specific intrinsics** (SIMD beyond WebAssembly support)

### JavaScript/TypeScript

#### MUST Compile as WASM
- **AssemblyScript projects**:
  ```json
  {
    "scripts": {
      "asbuild": "asc assembly/index.ts --target release"
    },
    "devDependencies": {
      "assemblyscript": "*"
    }
  }
  ```
- **AssemblyScript files**: `.asc`, `.as` extensions
- **WebAssembly imports**:
  ```javascript
  import init, { function_name } from './pkg/module.js';
  ```

#### COULD Compile as WASM
- **Pure computational TypeScript** (via AssemblyScript)
- **Mathematical algorithms**
- **Data processing pipelines**
- **Game logic code**

#### COULD NOT Compile as WASM
- **DOM manipulation code**
- **Node.js-specific APIs** (`fs`, `path`, `crypto`)
- **Browser APIs** (except through WASM host bindings)
- **Async/await patterns** (limited WASM support)
- **Dynamic require()** statements

### Python

#### MUST Compile as WASM
- **Pyodide-specific configuration**:
  ```python
  # pyodide_build.py
  from pyodide_build import build_package
  ```
- **Pure Python with scientific stack**:
  - NumPy/SciPy usage (via Pyodide)
  - Matplotlib (WASM version)
  - Pandas (WASM support)

#### COULD Compile as WASM
- **Pure Python algorithms**
- **Data analysis scripts** (via Pyodide)
- **Mathematical computations**
- **Text processing**

#### COULD NOT Compile as WASM
- **C extensions** not ported to WASM
- **File system operations** beyond WASI
- **Network servers** (Flask, Django)
- **Threading with complex locks**
- **subprocess module usage**
- **Platform-specific modules** (`winreg`, `pwd`)

### Java/Scala

#### MUST Compile as WASM
- **TeaVM configuration**:
  ```xml
  <plugin>
    <groupId>org.teavm</groupId>
    <artifactId>teavm-maven-plugin</artifactId>
    <configuration>
      <targetType>WEBASSEMBLY</targetType>
    </configuration>
  </plugin>
  ```
- **CheerpJ WASM target**
- **Explicit WASM bytecode generation**

#### COULD Compile as WASM
- **Pure JVM algorithms**
- **Mathematical computations**
- **Data structures**
- **Business logic code**

#### COULD NOT Compile as WASM
- **JNI (Java Native Interface)**
- **Reflection-heavy frameworks**
- **File I/O beyond WASI**
- **Network servers**
- **Threading with complex synchronization**
- **JVM-specific features** (dynamic class loading)

## Detection Algorithm

### File Pattern Analysis
1. **Primary indicators** (MUST):
   - `.wasm`, `.wat` files present
   - WASM-specific configuration files
   - Explicit WASM build targets

2. **Dependency analysis** (MUST/COULD):
   - Parse package manifests for WASM dependencies
   - Check for WASM runtime libraries
   - Analyze build tool configurations

3. **Source code scanning** (COULD):
   - Language-specific import/include patterns
   - Build tags and conditional compilation
   - Platform-specific code detection

### Build Configuration Detection
1. **Rust**: Parse `Cargo.toml` for targets and dependencies
2. **Go**: Check for build tags and GOOS/GOARCH settings
3. **C/C++**: Scan for Emscripten toolchain usage
4. **JavaScript**: Look for AssemblyScript configuration
5. **Python**: Check for Pyodide or WASM-related imports
6. **Java/Scala**: Parse build files for WASM plugins

### System Dependency Analysis
1. **Incompatible patterns**:
   - Native system calls
   - Platform-specific libraries
   - Complex threading models
   - Hardware dependencies

2. **Compatible patterns**:
   - Pure computation
   - Data processing
   - Algorithms and data structures
   - Stateless operations

## Implementation Priority

### Phase 1: Direct Detection
- `.wasm` and `.wat` file presence
- Explicit WASM build configurations
- WASM-specific dependencies

### Phase 2: Language-Specific Analysis
- Rust with wasm32 targets
- Go with js/wasm build tags
- AssemblyScript projects
- C/C++ with Emscripten

### Phase 3: Heuristic Analysis
- System dependency scanning
- Threading pattern analysis
- Platform-specific code detection
- Performance suitability assessment

## Configuration Examples

### Cargo.toml (Rust WASM)
```toml
[package]
name = "wasm-example"
version = "0.1.0"

[lib]
crate-type = ["cdylib"]

[dependencies]
wasm-bindgen = "0.2"
js-sys = "0.3"

[dependencies.web-sys]
version = "0.3"
features = [
  "console",
  "Document",
  "Element",
  "HtmlElement",
  "Window",
]
```

### package.json (AssemblyScript)
```json
{
  "name": "assemblyscript-example",
  "scripts": {
    "asbuild:debug": "asc assembly/index.ts --target debug",
    "asbuild:release": "asc assembly/index.ts --target release",
    "asbuild": "npm run asbuild:release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0"
  }
}
```

### CMakeLists.txt (C++ Emscripten)
```cmake
cmake_minimum_required(VERSION 3.13)
project(wasm_example)

set(CMAKE_TOOLCHAIN_FILE ${EMSCRIPTEN_ROOT}/cmake/Modules/Platform/Emscripten.cmake)

add_executable(wasm_example main.cpp)
set_target_properties(wasm_example PROPERTIES 
    COMPILE_FLAGS "-s USE_SDL=2"
    LINK_FLAGS "-s USE_SDL=2 -s EXPORTED_FUNCTIONS='[\"_main\"]'"
)
```

This detection system ensures accurate Lane G routing while providing fallback options for edge cases and hybrid projects.