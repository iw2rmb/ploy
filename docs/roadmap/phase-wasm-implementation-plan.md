# Phase WASM: WebAssembly Runtime Support - Comprehensive Implementation Plan

## Executive Summary

This document outlines the complete implementation plan for **Phase WASM: WebAssembly Runtime Support**, introducing Lane G as Ploy's universal polyglot deployment target for WebAssembly modules. The implementation leverages wazero (pure Go) runtime, comprehensive WASM detection algorithms, and production-ready deployment infrastructure.

**Priority**: Medium (after no-SPOF Controller completion)
**Complexity**: High (new runtime integration + build pipeline)

---

## Phase Overview

### Strategic Goals
1. **Universal Polyglot Runtime**: Support WASM modules from any language (Rust, Go, C/C++, AssemblyScript, etc.)
2. **Seamless Integration**: Automatic detection and routing to Lane G based on WASM compilation indicators
3. **Production Ready**: Full Nomad integration with resource management, security policies, and monitoring
4. **Developer Experience**: Zero-configuration WASM deployment with comprehensive build pipeline support

### Core Technologies
- **Runtime**: wazero (pure Go WebAssembly runtime)
- **Standards**: WASI Preview 1, WebAssembly Component Model
- **Integration**: Nomad job scheduling, Consul service discovery, Traefik routing
- **Security**: OPA policy enforcement, resource constraints, sandbox isolation

---

## Implementation Architecture

### Lane G Workflow
```
Source Code → WASM Detection → WASM Builder → Runtime Deployment
     ↓              ↓              ↓              ↓
Multi-language  → Lane G Pick  → .wasm Module → wazero Runtime
Project           Algorithm      Generation      (Nomad Job)
```

### Component Breakdown
1. **Detection Engine**: `tools/lane-pick/` WASM detection logic
2. **Build Pipeline**: `controller/builders/wasm.go` with multi-language support
3. **Runtime Integration**: wazero-based execution environment
4. **Deployment**: Nomad job templates with WASI support
5. **Security**: OPA policies for WASM-specific constraints

---

## Phase 1: Foundation & Core Runtime

### Task 1.1: WASM Detection Engine Enhancement 
**Files**: `tools/lane-pick/main.go`

**Implementation Details**:
```go
// Add to detect() function in main.go
func detectWASM(root string) (bool, []string) {
    reasons := []string{}
    
    // Direct WASM files
    if hasAny(root, ".wasm") || hasAny(root, ".wat") {
        reasons = append(reasons, "Direct WASM files detected")
        return true, reasons
    }
    
    // Language-specific WASM targets
    if hasRustWASMTarget(root) {
        reasons = append(reasons, "Rust wasm32 target detected")
        return true, reasons
    }
    
    if hasGoWASMTarget(root) {
        reasons = append(reasons, "Go js/wasm target detected") 
        return true, reasons
    }
    
    if hasAssemblyScriptConfig(root) {
        reasons = append(reasons, "AssemblyScript configuration detected")
        return true, reasons
    }
    
    if hasEmscriptenConfig(root) {
        reasons = append(reasons, "Emscripten configuration detected")
        return true, reasons
    }
    
    return false, reasons
}
```

**Detection Functions to Implement**:
- `hasRustWASMTarget()`: Parse Cargo.toml for wasm-bindgen, wasm32-unknown-unknown
- `hasGoWASMTarget()`: Check for `// +build js,wasm` and `syscall/js` imports
- `hasAssemblyScriptConfig()`: Parse package.json for AssemblyScript dependencies
- `hasEmscriptenConfig()`: Check for .emscripten, CMakeLists.txt with emcc

**Success Criteria**:
- 95%+ accuracy for direct WASM projects
- Support for Rust, Go, C/C++, AssemblyScript detection
- Integration with existing lane picker logic

---

### Task 1.2: wazero Runtime Integration
**Files**: `controller/runtime/wasm.go` (new), `go.mod`

**Implementation Approach**:
```go
// controller/runtime/wasm.go
package runtime

import (
    "context"
    "github.com/tetratelabs/wazero"
    "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WASMRuntime struct {
    runtime wazero.Runtime
    config  WASMConfig
}

type WASMConfig struct {
    MaxMemoryPages uint32 // 64KB pages, max 4GB
    MaxExecTime    time.Duration
    AllowedSyscalls []string
    FilesystemRoot  string
}

func NewWASMRuntime(config WASMConfig) *WASMRuntime {
    ctx := context.Background()
    r := wazero.NewRuntime(ctx)
    
    // Configure WASI Preview 1
    wasi_snapshot_preview1.MustInstantiate(ctx, r)
    
    return &WASMRuntime{
        runtime: r,
        config: config,
    }
}

func (w *WASMRuntime) ExecuteModule(ctx context.Context, wasmBytes []byte, args []string) error {
    // Compile WASM module
    mod, err := w.runtime.Instantiate(ctx, wasmBytes)
    if err != nil {
        return fmt.Errorf("failed to instantiate WASM module: %w", err)
    }
    defer mod.Close(ctx)
    
    // Execute main function or _start
    main := mod.ExportedFunction("_start")
    if main == nil {
        main = mod.ExportedFunction("main")
    }
    
    if main == nil {
        return fmt.Errorf("no main or _start function found")
    }
    
    _, err = main.Call(ctx)
    return err
}
```

**Key Features**:
- WASI Preview 1 support for filesystem and networking
- Resource constraints (memory, execution time)  
- Security sandbox with syscall restrictions
- Error handling and logging integration

**Dependencies**:
- Add `github.com/tetratelabs/wazero v1.5.0` to go.mod
- Optional: `github.com/bytecodealliance/wasmtime-go` as alternative runtime

**Success Criteria**:
- Execute simple WASM modules (Hello World)
- WASI filesystem operations working
- Resource limits enforced
- Integration with existing controller architecture

---

### Task 1.3: WASM Builder Implementation
**Files**: `controller/builders/wasm.go` (new)

**Architecture**:
```go
// controller/builders/wasm.go
package builders

import (
    "archive/tar"
    "context"
    "fmt"
    "path/filepath"
)

type WASMBuilder struct {
    BaseBuilder
}

func NewWASMBuilder() *WASMBuilder {
    return &WASMBuilder{
        BaseBuilder: BaseBuilder{
            Name: "WASM Builder",
            Lane: "G", 
        },
    }
}

func (w *WASMBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
    // Detect WASM compilation strategy
    strategy, err := w.detectBuildStrategy(req.SourcePath)
    if err != nil {
        return nil, fmt.Errorf("failed to detect build strategy: %w", err)
    }
    
    // Execute language-specific build
    wasmPath, err := w.executeBuild(ctx, strategy, req.SourcePath)
    if err != nil {
        return nil, fmt.Errorf("WASM build failed: %w", err)
    }
    
    // Package WASM module with metadata
    artifactPath, err := w.packageWASMModule(wasmPath, req.AppName)
    if err != nil {
        return nil, fmt.Errorf("failed to package WASM module: %w", err)
    }
    
    return &BuildResult{
        ArtifactPath: artifactPath,
        Lane: "G",
        Runtime: "wazero",
        Metadata: map[string]string{
            "wasm_strategy": strategy,
            "wasm_runtime": "wazero",
        },
    }, nil
}

type buildStrategy string

const (
    StrategyRustWasm32     buildStrategy = "rust-wasm32"
    StrategyGoJSWasm       buildStrategy = "go-js-wasm"
    StrategyAssemblyScript buildStrategy = "assemblyscript"
    StrategyEmscripten     buildStrategy = "emscripten"
    StrategyDirect         buildStrategy = "direct-wasm"
)

func (w *WASMBuilder) detectBuildStrategy(sourcePath string) (buildStrategy, error) {
    // Check for direct WASM files
    if hasFiles, _ := hasAnyFiles(sourcePath, ".wasm", ".wat"); hasFiles {
        return StrategyDirect, nil
    }
    
    // Check for Rust with WASM target
    if exists(filepath.Join(sourcePath, "Cargo.toml")) {
        if hasRustWASMDeps(sourcePath) {
            return StrategyRustWasm32, nil
        }
    }
    
    // Check for Go with js/wasm target
    if exists(filepath.Join(sourcePath, "go.mod")) {
        if hasGoWASMBuildTags(sourcePath) {
            return StrategyGoJSWasm, nil
        }
    }
    
    // Check for AssemblyScript
    if exists(filepath.Join(sourcePath, "package.json")) {
        if hasAssemblyScriptDeps(sourcePath) {
            return StrategyAssemblyScript, nil
        }
    }
    
    // Check for Emscripten (C/C++)
    if hasEmscriptenToolchain(sourcePath) {
        return StrategyEmscripten, nil
    }
    
    return "", fmt.Errorf("no supported WASM build strategy detected")
}
```

**Build Strategy Implementations**:

1. **Rust WASM32**:
   ```bash
   cargo build --target wasm32-unknown-unknown --release
   wasm-pack build --target nodejs --out-dir pkg
   ```

2. **Go JS/WASM**:
   ```bash
   GOOS=js GOARCH=wasm go build -o main.wasm
   ```

3. **AssemblyScript**:
   ```bash
   npx asc assembly/index.ts --target release --outFile build/optimized.wasm
   ```

4. **Emscripten (C/C++)**:
   ```bash
   emcc -O3 -s WASM=1 -s EXPORTED_FUNCTIONS='["_main"]' main.c -o main.wasm
   ```

**Success Criteria**:
- Support for 4+ WASM compilation strategies
- Consistent artifact packaging format
- Integration with existing build pipeline
- Comprehensive error handling and logging

---

### Task 1.4: Build Scripts Infrastructure
**Files**: `scripts/build/wasm/` (new directory)

**Directory Structure**:
```
scripts/build/wasm/
├── rust-wasm32.sh      # Rust → WASM32 compilation
├── go-js-wasm.sh       # Go → JS/WASM compilation
├── assemblyscript.sh   # AssemblyScript → WASM
├── emscripten.sh       # C/C++ → WASM via Emscripten
├── common.sh           # Shared utilities
└── validate-wasm.sh    # WASM module validation
```

**Script Templates**:

`rust-wasm32.sh`:
```bash
#!/bin/bash
set -euo pipefail

RUST_VERSION="1.72"
SOURCE_DIR="$1"
OUTPUT_DIR="$2"
APP_NAME="$3"

echo "Building Rust project for WASM32..."

# Install Rust WASM target if not present
rustup target add wasm32-unknown-unknown

# Build with WASM target
cd "$SOURCE_DIR"
cargo build --target wasm32-unknown-unknown --release

# Use wasm-pack if available for optimized builds
if command -v wasm-pack &> /dev/null; then
    wasm-pack build --target nodejs --out-dir "$OUTPUT_DIR"
else
    # Manual copy of WASM artifact
    cp "target/wasm32-unknown-unknown/release/${APP_NAME}.wasm" "$OUTPUT_DIR/"
fi

echo "Rust WASM build completed: $OUTPUT_DIR/${APP_NAME}.wasm"
```

`go-js-wasm.sh`:
```bash
#!/bin/bash
set -euo pipefail

SOURCE_DIR="$1"
OUTPUT_DIR="$2"
APP_NAME="$3"

echo "Building Go project for JS/WASM..."

cd "$SOURCE_DIR"

# Build Go for WebAssembly
GOOS=js GOARCH=wasm go build -o "$OUTPUT_DIR/${APP_NAME}.wasm"

# Copy Go WASM runtime support
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" "$OUTPUT_DIR/"

echo "Go WASM build completed: $OUTPUT_DIR/${APP_NAME}.wasm"
```

**Success Criteria**:
- Automated builds for all supported WASM targets
- Consistent output format and artifact naming
- Error handling and build validation
- Integration with controller build system

---

## Phase 2: Production Integration & Advanced Features

### Task 2.1: Nomad Job Templates for WASM
**Files**: `platform/nomad/templates/wasm-app.hcl.j2` (new)

**Nomad Job Template**:
```hcl
# platform/nomad/templates/wasm-app.hcl.j2
job "wasm-{{ app_name }}" {
  datacenters = ["{{ datacenter }}"]
  type = "service"
  
  constraint {
    attribute = "${attr.kernel.name}"
    value     = "linux"
  }

  group "wasm-runtime" {
    count = {{ replicas | default(1) }}
    
    restart {
      attempts = 3
      interval = "30m"
      delay    = "15s"
      mode     = "fail"
    }

    # WASM-specific resource constraints
    resources {
      cpu    = {{ resources.cpu | default(100) }}    # MHz
      memory = {{ resources.memory | default(128) }}  # MB
      
      # WASM modules are typically small
      disk = {{ resources.disk | default(50) }}     # MB
    }

    # Network configuration for WASM apps
    network {
      mbits = 10
      
      {% if port_mapping %}
      port "http" {
        static = {{ port_mapping.http }}
      }
      {% else %}
      port "http" {}
      {% endif %}
    }

    # WASM module artifact
    artifact {
      source      = "{{ artifact_url }}"
      destination = "local/app.wasm"
      mode        = "file"
    }

    task "wasm-runner" {
      driver = "exec"
      
      config {
        command = "/usr/local/bin/ploy-wasm-runner"
        args = [
          "--module", "local/app.wasm",
          "--port", "${NOMAD_PORT_http}",
          {% if environment_vars %}
          {% for key, value in environment_vars.items() %}
          "--env", "{{ key }}={{ value }}",
          {% endfor %}
          {% endif %}
        ]
      }

      # WASM runtime environment
      env {
        WASM_APP_NAME = "{{ app_name }}"
        WASM_RUNTIME  = "wazero"
        
        {% if wasi_config %}
        # WASI configuration
        WASI_ROOT = "/tmp/wasi-root"
        {% endif %}
        
        {% if environment_vars %}
        {% for key, value in environment_vars.items() %}
        {{ key }} = "{{ value }}"
        {% endfor %}
        {% endif %}
      }

      # WASM module validation
      template {
        data = <<EOF
#!/bin/bash
# Validate WASM module before execution
if ! wasm-validate local/app.wasm; then
    echo "Invalid WASM module"
    exit 1
fi
echo "WASM module validation passed"
EOF
        destination = "local/validate.sh"
        perms = "755"
      }

      # Service registration for WASM apps
      service {
        name = "wasm-{{ app_name }}"
        port = "http"
        
        tags = [
          "wasm",
          "lane-g",
          "app:{{ app_name }}",
          "traefik.enable=true",
          "traefik.http.routers.{{ app_name }}.rule=Host(`{{ app_name }}.{{ domain }}`)"
        ]

        check {
          type     = "http"
          path     = "/health"
          interval = "10s"
          timeout  = "3s"
          
          check_restart {
            limit = 3
            grace = "10s"
          }
        }
      }

      # Resource limits specific to WASM
      resources {
        cpu    = {{ resources.cpu | default(100) }}
        memory = {{ resources.memory | default(128) }}
        
        # WASM has predictable memory usage
        memory_max = {{ resources.memory_max | default(256) }}
      }

      # Logs configuration
      logs {
        max_files     = 3
        max_file_size = 10
      }

      # Kill timeout for graceful WASM shutdown
      kill_timeout = "5s"
    }
  }
}
```

**WASM Runner Implementation** (`cmd/ploy-wasm-runner/main.go`):
```go
package main

import (
    "context"
    "flag"
    "log"
    "net/http"
    "os"
    "time"
    
    "github.com/iw2rmb/ploy/controller/runtime"
)

func main() {
    var (
        modulePath = flag.String("module", "", "Path to WASM module")
        port      = flag.String("port", "8080", "HTTP port")
        envVars   = flag.String("env", "", "Environment variables")
    )
    flag.Parse()

    if *modulePath == "" {
        log.Fatal("--module flag is required")
    }

    // Load WASM module
    wasmBytes, err := os.ReadFile(*modulePath)
    if err != nil {
        log.Fatalf("Failed to read WASM module: %v", err)
    }

    // Initialize WASM runtime
    config := runtime.WASMConfig{
        MaxMemoryPages: 256, // 16MB
        MaxExecTime:   30 * time.Second,
    }
    
    wasmRuntime := runtime.NewWASMRuntime(config)
    
    // Execute WASM module in HTTP server context
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        ctx := context.WithTimeout(r.Context(), 10*time.Second)
        err := wasmRuntime.ExecuteModule(ctx, wasmBytes, []string{})
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        w.Write([]byte("WASM execution completed"))
    })
    
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("OK"))
    })

    log.Printf("Starting WASM HTTP server on port %s", *port)
    log.Fatal(http.ListenAndServe(":"+*port, nil))
}
```

**Success Criteria**:
- Nomad job deploys WASM modules successfully
- Resource constraints properly enforced
- Health checking and service discovery working
- Traefik routing to WASM apps functional

---

### Task 2.2: WASI Support Implementation
**Files**: `controller/runtime/wasi.go` (new)

**WASI Preview 1 Implementation**:
```go
// controller/runtime/wasi.go
package runtime

import (
    "context"
    "io/fs"
    "os"
    "path/filepath"
    
    "github.com/tetratelabs/wazero"
    "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

type WASIConfig struct {
    Args        []string
    Env         map[string]string
    Preopens    map[string]string // guest_path -> host_path
    Stdin       io.Reader
    Stdout      io.Writer
    Stderr      io.Writer
}

func (w *WASMRuntime) ConfigureWASI(ctx context.Context, config WASIConfig) error {
    // Configure WASI with custom filesystem mapping
    fsConfig := wazero.NewFSConfig()
    
    // Add preopen directories (sandbox filesystem)
    for guestPath, hostPath := range config.Preopens {
        if err := os.MkdirAll(hostPath, 0755); err != nil {
            return fmt.Errorf("failed to create preopen directory %s: %w", hostPath, err)
        }
        
        fsConfig = fsConfig.WithDirMount(hostPath, guestPath)
    }
    
    // Configure module with WASI
    moduleConfig := wazero.NewModuleConfig().
        WithArgs(config.Args...).
        WithFSConfig(fsConfig)
    
    // Add environment variables
    for key, value := range config.Env {
        moduleConfig = moduleConfig.WithEnv(key, value)
    }
    
    // Configure stdio
    if config.Stdin != nil {
        moduleConfig = moduleConfig.WithStdin(config.Stdin)
    }
    if config.Stdout != nil {
        moduleConfig = moduleConfig.WithStdout(config.Stdout)
    }
    if config.Stderr != nil {
        moduleConfig = moduleConfig.WithStderr(config.Stderr)
    }
    
    w.moduleConfig = moduleConfig
    return nil
}

// Predefined WASI configurations for common use cases
func DefaultWebWASIConfig() WASIConfig {
    return WASIConfig{
        Args: []string{"app"},
        Env: map[string]string{
            "PATH": "/usr/bin:/bin",
        },
        Preopens: map[string]string{
            "/tmp":  "/tmp/wasm-sandbox",
            "/data": "/opt/wasm-data",
        },
    }
}

func ServerWASIConfig(dataDir string) WASIConfig {
    return WASIConfig{
        Args: []string{"server"},
        Env: map[string]string{
            "PATH": "/usr/bin:/bin",
            "HOME": "/tmp/wasm-home",
        },
        Preopens: map[string]string{
            "/":     "/tmp/wasm-root",
            "/data": dataDir,
            "/tmp":  "/tmp/wasm-tmp",
        },
    }
}
```

**Security Features**:
- Sandboxed filesystem access through preopens
- Environment variable isolation
- Resource constraints (memory, CPU time)
- No network access by default (controlled through WASI)

**Success Criteria**:
- WASM modules can read/write files through WASI
- Environment variables passed correctly
- Filesystem access properly sandboxed
- Integration with Nomad job environment

---

### Task 2.3: OPA Policy Integration for WASM
**Files**: `policies/wasm.rego` (new)

**WASM-Specific OPA Policies**:
```rego
# policies/wasm.rego
package wasm

import rego.v1

# WASM module size limits (typically much smaller than container images)
max_wasm_size_mb := 50

# WASM resource constraints
default allow_wasm_deployment := false

allow_wasm_deployment if {
    input.lane == "G"
    input.artifact_size_mb <= max_wasm_size_mb
    valid_wasm_module
    safe_wasi_config
}

valid_wasm_module if {
    # WASM module must have proper magic bytes
    input.artifact_type == "wasm"
    
    # Must include WASM validation results
    input.wasm_validation.valid == true
    input.wasm_validation.version in ["mvp", "1.0"]
}

safe_wasi_config if {
    # Restrict WASI filesystem access
    count(input.wasi_preopens) <= 5
    
    # Ensure no root filesystem access
    not "/etc" in input.wasi_preopens
    not "/usr" in input.wasi_preopens
    not "/var" in input.wasi_preopens
    
    # Limit environment variables
    count(input.environment_vars) <= 20
}

# WASM security requirements
wasm_security_requirements := {
    "max_memory_pages": 1024,     # 64MB max memory
    "max_execution_time": 300,    # 5 minutes max execution
    "allow_network": false,       # No network by default
    "allow_filesystem": true,     # Limited filesystem through WASI
}

deny_wasm_deployment[msg] {
    input.lane == "G"
    input.wasm_config.max_memory_pages > wasm_security_requirements.max_memory_pages
    msg := "WASM module exceeds memory limit"
}

deny_wasm_deployment[msg] {
    input.lane == "G"
    input.wasm_config.allow_network == true
    not input.network_policy_approved
    msg := "WASM network access requires explicit approval"
}

# Development vs Production policies
allow_wasm_development if {
    input.environment == "development"
    input.lane == "G"
    basic_wasm_validation
}

allow_wasm_production if {
    input.environment == "production"
    input.lane == "G"
    valid_wasm_module
    safe_wasi_config
    input.signed_artifact == true
    input.sbom_present == true
}

basic_wasm_validation if {
    input.artifact_type == "wasm"
    input.artifact_size_mb <= max_wasm_size_mb
}
```

**Policy Integration** in `controller/policies/enforcer.go`:
```go
func (e *PolicyEnforcer) ValidateWASMDeployment(req *DeploymentRequest) error {
    if req.Lane != "G" {
        return nil // Not a WASM deployment
    }
    
    input := map[string]interface{}{
        "lane": req.Lane,
        "artifact_type": "wasm",
        "artifact_size_mb": req.ArtifactSizeMB,
        "wasm_validation": req.WASMValidation,
        "wasi_preopens": req.WASIConfig.Preopens,
        "environment_vars": req.EnvironmentVars,
        "environment": e.config.Environment,
        "signed_artifact": req.SignedArtifact,
        "sbom_present": req.SBOMPresent,
    }
    
    result, err := e.evaluatePolicy("wasm/allow_wasm_deployment", input)
    if err != nil {
        return fmt.Errorf("WASM policy evaluation failed: %w", err)
    }
    
    if !result.Allowed {
        return fmt.Errorf("WASM deployment denied: %v", result.Reasons)
    }
    
    return nil
}
```

**Success Criteria**:
- WASM-specific security policies enforced
- Resource constraints validated before deployment
- Separate policies for development vs production
- Integration with existing OPA infrastructure

---

### Task 2.4: Component Model Integration
**Files**: `controller/wasm/components.go` (new)

**WebAssembly Component Model Support**:
```go
// controller/wasm/components.go
package wasm

import (
    "context"
    "fmt"
    "path/filepath"
    
    "github.com/tetratelabs/wazero"
)

type ComponentManager struct {
    runtime wazero.Runtime
    modules map[string]wazero.CompiledModule
}

type ComponentSpec struct {
    Name        string            `json:"name"`
    MainModule  string            `json:"main_module"`
    Dependencies []string          `json:"dependencies"`
    Interfaces  []InterfaceSpec   `json:"interfaces"`
    Resources   ResourceLimits    `json:"resources"`
}

type InterfaceSpec struct {
    Name    string   `json:"name"`
    Exports []string `json:"exports"`
    Imports []string `json:"imports"`
}

type ResourceLimits struct {
    MaxMemoryMB     int `json:"max_memory_mb"`
    MaxExecutionSec int `json:"max_execution_sec"`
}

func NewComponentManager(ctx context.Context) *ComponentManager {
    return &ComponentManager{
        runtime: wazero.NewRuntime(ctx),
        modules: make(map[string]wazero.CompiledModule),
    }
}

func (cm *ComponentManager) LoadComponent(ctx context.Context, spec ComponentSpec, artifactPath string) error {
    // Load main WASM module
    mainBytes, err := os.ReadFile(filepath.Join(artifactPath, spec.MainModule))
    if err != nil {
        return fmt.Errorf("failed to read main module: %w", err)
    }
    
    mainModule, err := cm.runtime.CompileModule(ctx, mainBytes)
    if err != nil {
        return fmt.Errorf("failed to compile main module: %w", err)
    }
    cm.modules[spec.Name+"_main"] = mainModule
    
    // Load dependency modules
    for _, dep := range spec.Dependencies {
        depBytes, err := os.ReadFile(filepath.Join(artifactPath, dep))
        if err != nil {
            return fmt.Errorf("failed to read dependency %s: %w", dep, err)
        }
        
        depModule, err := cm.runtime.CompileModule(ctx, depBytes)
        if err != nil {
            return fmt.Errorf("failed to compile dependency %s: %w", dep, err)
        }
        cm.modules[spec.Name+"_"+dep] = depModule
    }
    
    return nil
}

func (cm *ComponentManager) InstantiateComponent(ctx context.Context, spec ComponentSpec) error {
    // Instantiate modules with proper linking
    moduleConfig := wazero.NewModuleConfig().WithName(spec.Name)
    
    // Apply resource limits
    if spec.Resources.MaxMemoryMB > 0 {
        // Configure memory limits (wazero handles this internally)
        moduleConfig = moduleConfig.WithMemoryLimitPages(
            uint32(spec.Resources.MaxMemoryMB * 1024 * 1024 / 65536), // Convert MB to pages
        )
    }
    
    // Instantiate main module
    mainModule := cm.modules[spec.Name+"_main"]
    _, err := cm.runtime.InstantiateModule(ctx, mainModule, moduleConfig)
    if err != nil {
        return fmt.Errorf("failed to instantiate main module: %w", err)
    }
    
    return nil
}

// Component specification parsing
func ParseComponentSpec(specPath string) (*ComponentSpec, error) {
    data, err := os.ReadFile(specPath)
    if err != nil {
        return nil, err
    }
    
    var spec ComponentSpec
    if err := json.Unmarshal(data, &spec); err != nil {
        return nil, err
    }
    
    return &spec, nil
}
```

**Component Specification Format** (`wasm-component.json`):
```json
{
  "name": "my-wasm-app",
  "main_module": "app.wasm",
  "dependencies": [
    "math-utils.wasm",
    "http-client.wasm"
  ],
  "interfaces": [
    {
      "name": "http-handler",
      "exports": ["handle_request", "handle_response"],
      "imports": ["log_message", "get_env"]
    }
  ],
  "resources": {
    "max_memory_mb": 32,
    "max_execution_sec": 60
  }
}
```

**Success Criteria**:
- Multi-module WASM applications supported
- Component linking working correctly
- Resource limits enforced per component
- Integration with build pipeline

---

### Task 2.5: Sample Applications & Testing
**Files**: `apps/wasm-*` (new directories)

**Sample WASM Applications**:

1. **Rust WASM App** (`apps/wasm-rust-hello/`):
```
apps/wasm-rust-hello/
├── Cargo.toml
├── src/
│   └── main.rs
└── README.md
```

`Cargo.toml`:
```toml
[package]
name = "wasm-rust-hello"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[dependencies]
wasm-bindgen = "0.2"
js-sys = "0.3"

[dependencies.web-sys]
version = "0.3"
features = [
  "console",
]
```

`src/main.rs`:
```rust
use wasm_bindgen::prelude::*;

#[wasm_bindgen]
extern "C" {
    #[wasm_bindgen(js_namespace = console)]
    fn log(s: &str);
}

#[wasm_bindgen]
pub fn greet(name: &str) {
    log(&format!("Hello, {}!", name));
}

#[wasm_bindgen(start)]
pub fn main() {
    log("Rust WASM module loaded successfully!");
}
```

2. **Go WASM App** (`apps/wasm-go-hello/`):
```
apps/wasm-go-hello/
├── go.mod
├── main.go
└── README.md
```

`main.go`:
```go
//go:build js && wasm

package main

import (
    "fmt"
    "syscall/js"
)

func helloHandler(this js.Value, args []js.Value) interface{} {
    name := args[0].String()
    message := fmt.Sprintf("Hello, %s from Go WASM!", name)
    return js.ValueOf(message)
}

func main() {
    fmt.Println("Go WASM module started")
    
    js.Global().Set("hello", js.FuncOf(helloHandler))
    
    // Keep the program running
    select {}
}
```

3. **AssemblyScript App** (`apps/wasm-assemblyscript-hello/`):
```
apps/wasm-assemblyscript-hello/
├── package.json
├── assembly/
│   └── index.ts
└── README.md
```

`package.json`:
```json
{
  "name": "wasm-assemblyscript-hello",
  "scripts": {
    "asbuild:debug": "asc assembly/index.ts --target debug",
    "asbuild:release": "asc assembly/index.ts --target release",
    "build": "npm run asbuild:release"
  },
  "devDependencies": {
    "assemblyscript": "^0.27.0"
  }
}
```

`assembly/index.ts`:
```typescript
export function add(a: i32, b: i32): i32 {
  return a + b;
}

export function greet(name: string): string {
  return "Hello, " + name + " from AssemblyScript!";
}
```

4. **C++ Emscripten App** (`apps/wasm-cpp-hello/`):
```
apps/wasm-cpp-hello/
├── CMakeLists.txt
├── main.cpp
└── README.md
```

`CMakeLists.txt`:
```cmake
cmake_minimum_required(VERSION 3.13)
project(wasm_cpp_hello)

set(CMAKE_CXX_STANDARD 17)

add_executable(wasm_cpp_hello main.cpp)

set_target_properties(wasm_cpp_hello PROPERTIES 
    COMPILE_FLAGS "-s USE_SDL=0"
    LINK_FLAGS "-s EXPORTED_FUNCTIONS='[\"_main\"]' -s EXPORTED_RUNTIME_METHODS='[\"ccall\",\"cwrap\"]'"
)
```

`main.cpp`:
```cpp
#include <iostream>
#include <emscripten.h>

extern "C" {
    EMSCRIPTEN_KEEPALIVE
    int add_numbers(int a, int b) {
        return a + b;
    }
    
    EMSCRIPTEN_KEEPALIVE
    void hello_world() {
        std::cout << "Hello from C++ WASM!" << std::endl;
    }
}

int main() {
    std::cout << "C++ WASM module initialized" << std::endl;
    return 0;
}
```

**Testing Scripts** (`test-scripts/test-wasm-*.sh`):

`test-scripts/test-wasm-lane-detection.sh`:
```bash
#!/bin/bash
set -euo pipefail

echo "Testing WASM lane detection..."

# Test Rust WASM detection
echo "Testing Rust WASM detection..."
result=$(./build/ploy lane-pick --path apps/wasm-rust-hello)
if echo "$result" | jq -r '.lane' | grep -q "G"; then
    echo "✓ Rust WASM detected as Lane G"
else
    echo "✗ Rust WASM detection failed"
    exit 1
fi

# Test Go WASM detection
echo "Testing Go WASM detection..."
result=$(./build/ploy lane-pick --path apps/wasm-go-hello)
if echo "$result" | jq -r '.lane' | grep -q "G"; then
    echo "✓ Go WASM detected as Lane G"
else
    echo "✗ Go WASM detection failed"
    exit 1
fi

# Test AssemblyScript detection
echo "Testing AssemblyScript detection..."
result=$(./build/ploy lane-pick --path apps/wasm-assemblyscript-hello)
if echo "$result" | jq -r '.lane' | grep -q "G"; then
    echo "✓ AssemblyScript detected as Lane G"
else
    echo "✗ AssemblyScript detection failed"
    exit 1
fi

echo "All WASM detection tests passed!"
```

**Success Criteria**:
- 4+ sample WASM applications working
- Lane detection accurate for all samples
- Build pipeline successful for all targets
- Applications deployable via `ploy push`

---

## Testing & Validation Strategy

### Unit Testing
**Files**: `controller/runtime/wasm_test.go`, `controller/builders/wasm_test.go`

**Test Coverage**:
- WASM module loading and validation
- Build strategy detection accuracy
- Resource constraint enforcement
- WASI configuration validation
- Component model functionality

### Integration Testing
**Files**: `test-scripts/test-wasm-integration.sh`

**Test Scenarios**:
1. End-to-end WASM app deployment
2. Multi-language WASM support verification
3. Nomad job creation and execution
4. Service discovery and routing
5. Policy enforcement validation

### Performance Testing
**Metrics**:
- WASM module startup time (< 100ms)
- Memory usage (within configured limits)
- Build time comparison vs traditional lanes
- Concurrent WASM execution capability

### Security Testing
**Validation**:
- Filesystem sandbox enforcement
- Resource limit adherence
- Network access restrictions
- Malicious WASM module rejection

---

## Documentation Updates

### Task D.1: Update Core Documentation
**Files**: `docs/FEATURES.md`, `docs/STACK.md`, `docs/CONCEPT.md`

**FEATURES.md Additions**:
```markdown
### Lane G: WebAssembly Runtime (Aug 2025)
- ✅ **Multi-Language WASM Support**: Rust, Go, C/C++, AssemblyScript compilation
- ✅ **wazero Runtime Integration**: Pure Go WebAssembly runtime
- ✅ **WASI Preview 1**: Filesystem and environment access for WASM modules
- ✅ **Component Model**: Multi-module WASM applications with linking
- ✅ **Automatic Detection**: Language-specific WASM target detection
- ✅ **Security Policies**: OPA-based WASM deployment validation
- ✅ **Resource Constraints**: Memory and execution time limits
```

### Task D.2: Create WASM Documentation
**Files**: `docs/WASM-RUNTIME.md` (new)

**Comprehensive WASM Guide**:
- Developer guide for WASM deployments
- Build configuration examples
- Performance optimization tips
- Troubleshooting common issues
- Security best practices

### Task D.3: Update CLI Documentation
**Files**: `docs/CLI.md`

**CLI Additions**:
```markdown
## WASM Commands

### Deploy WASM Application
```bash
ploy push -a my-wasm-app -lane G
```

### Check WASM Detection
```bash
ploy lane-pick --path ./wasm-project
```

### WASM Build Options
```bash
ploy push -a my-app --build-strategy rust-wasm32
ploy push -a my-app --build-strategy go-js-wasm
```
```

---

## Success Metrics & KPIs

### Technical Metrics
- **Lane Detection Accuracy**: >95% for WASM projects
- **Build Success Rate**: >90% for supported languages
- **Startup Time**: <100ms for WASM modules
- **Memory Efficiency**: 50-80% reduction vs container deployment
- **Build Time**: Competitive with traditional lanes

### Operational Metrics
- **Deployment Success Rate**: >95%
- **Runtime Stability**: 99.5% uptime
- **Resource Utilization**: Within configured limits
- **Policy Compliance**: 100% enforcement

### Developer Experience
- **Zero Configuration**: Automatic detection and deployment
- **Multi-Language Support**: 4+ languages supported
- **Error Messages**: Clear, actionable feedback
- **Documentation**: Comprehensive guides and examples

---

## Risk Mitigation

### Technical Risks
1. **WASM Runtime Stability**: Mitigated by using mature wazero runtime
2. **Performance Overhead**: Mitigated by benchmarking and optimization
3. **Security Vulnerabilities**: Mitigated by sandbox isolation and policies
4. **Build Complexity**: Mitigated by comprehensive build scripts

### Operational Risks
1. **Deployment Issues**: Mitigated by extensive testing and rollback procedures
2. **Resource Conflicts**: Mitigated by proper resource planning and limits
3. **Integration Problems**: Mitigated by incremental integration approach

### Mitigation Strategies
- Comprehensive testing at each phase
- Feature flags for gradual rollout
- Monitoring and alerting integration
- Documentation and training materials

---

## Implementation Phases

### Phase 1: Foundation
- Lane detection enhancement
- wazero runtime integration
- WASM builder implementation
- Build scripts infrastructure

**Milestone**: Basic WASM module execution working

### Phase 2: Production Integration
- Nomad job templates
- WASI support implementation
- OPA policy integration
- Component model and testing

**Milestone**: Production-ready WASM deployment system

### Documentation & Testing (Continuous)
- Sample applications created throughout implementation
- Documentation updated incrementally
- Testing performed at each milestone

---

## Conclusion

This comprehensive implementation plan provides a roadmap for adding sophisticated WebAssembly support to Ploy through Lane G. The plan balances technical complexity with practical implementation, ensuring a robust, secure, and performant WASM runtime integration.

The phased approach allows for incremental development and testing, reducing risk while maintaining development velocity. Upon completion, Ploy will offer industry-leading WebAssembly deployment capabilities with automatic language detection, comprehensive build pipeline support, and production-ready runtime integration.

**Key Deliverables**:
1. ✅ Multi-language WASM detection and building
2. ✅ Production-ready wazero runtime integration  
3. ✅ Nomad-based deployment with resource management
4. ✅ WASI Preview 1 support for filesystem/environment access
5. ✅ Component Model for complex WASM applications
6. ✅ Security policies and resource constraints
7. ✅ Comprehensive documentation and sample applications

This implementation will establish Ploy as a leading platform for WebAssembly deployment, providing developers with seamless, secure, and efficient WASM application hosting capabilities.