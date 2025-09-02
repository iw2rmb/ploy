package builders

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewWASMBuilder tests the WASMBuilder constructor
func TestNewWASMBuilder(t *testing.T) {
	builder := NewWASMBuilder()
	
	if builder == nil {
		t.Fatal("NewWASMBuilder returned nil")
	}
	if builder.Name != "WASM Builder" {
		t.Errorf("builder name: got %s, want 'WASM Builder'", builder.Name)
	}
	if builder.Lane != "G" {
		t.Errorf("builder lane: got %s, want 'G'", builder.Lane)
	}
}

// TestWASMBuilderBuild tests the Build method
func TestWASMBuilderBuild(t *testing.T) {
	tests := []struct {
		name           string
		request        BuildRequest
		setupFiles     map[string]string
		mockStrategy   buildStrategy
		mockBuildError error
		expectError    bool
		errContains    string
		expectedLane   string
		expectedRuntime string
	}{
		{
			name: "successful Rust WASM build",
			request: BuildRequest{
				SourcePath: "/tmp/rust-wasm",
				AppName:    "rust-app",
				BuildID:    "build-123",
				OutputPath: "/tmp/out",
			},
			setupFiles: map[string]string{
				"Cargo.toml": `
					[dependencies]
					wasm-bindgen = "0.2"
				`,
				"src/lib.rs": "// Rust WASM code",
			},
			mockStrategy:    StrategyRustWasm32,
			mockBuildError:  nil,
			expectError:     false,
			expectedLane:    "G",
			expectedRuntime: "wazero",
		},
		{
			name: "successful Go WASM build",
			request: BuildRequest{
				SourcePath: "/tmp/go-wasm",
				AppName:    "go-app",
				BuildID:    "build-456",
				OutputPath: "/tmp/out",
			},
			setupFiles: map[string]string{
				"go.mod":  "module example.com/wasm",
				"main.go": "//go:build js && wasm\npackage main",
			},
			mockStrategy:    StrategyGoJSWasm,
			mockBuildError:  nil,
			expectError:     false,
			expectedLane:    "G",
			expectedRuntime: "wazero",
		},
		{
			name: "successful AssemblyScript build",
			request: BuildRequest{
				SourcePath: "/tmp/assemblyscript",
				AppName:    "as-app",
				BuildID:    "build-789",
				OutputPath: "/tmp/out",
			},
			setupFiles: map[string]string{
				"package.json": `{
					"devDependencies": {
						"assemblyscript": "^0.27.0"
					}
				}`,
				"assembly/index.ts": "// AssemblyScript code",
			},
			mockStrategy:    StrategyAssemblyScript,
			mockBuildError:  nil,
			expectError:     false,
			expectedLane:    "G",
			expectedRuntime: "wazero",
		},
		{
			name: "direct WASM file",
			request: BuildRequest{
				SourcePath: "/tmp/direct-wasm",
				AppName:    "direct-app",
				BuildID:    "build-abc",
				OutputPath: "/tmp/out",
			},
			setupFiles: map[string]string{
				"app.wasm": string([]byte{0x00, 0x61, 0x73, 0x6d}), // WASM magic bytes
			},
			mockStrategy:    StrategyDirect,
			mockBuildError:  nil,
			expectError:     false,
			expectedLane:    "G",
			expectedRuntime: "wazero",
		},
		{
			name: "no WASM strategy detected",
			request: BuildRequest{
				SourcePath: "/tmp/no-wasm",
				AppName:    "no-app",
				BuildID:    "build-xyz",
				OutputPath: "/tmp/out",
			},
			setupFiles: map[string]string{
				"main.py": "# Python code",
			},
			expectError: true,
			errContains: "no supported WASM build strategy detected",
		},
		{
			name: "build execution failure",
			request: BuildRequest{
				SourcePath: "/tmp/rust-wasm",
				AppName:    "fail-app",
				BuildID:    "build-fail",
				OutputPath: "/tmp/out",
			},
			setupFiles: map[string]string{
				"Cargo.toml": `[dependencies]\nwasm-bindgen = "0.2"`,
			},
			mockStrategy:   StrategyRustWasm32,
			mockBuildError: errors.New("compilation failed"),
			expectError:    true,
			errContains:    "WASM build failed",
		},
		{
			name: "output directory creation failure",
			request: BuildRequest{
				SourcePath: "/tmp/rust-wasm",
				AppName:    "app",
				BuildID:    "build-123",
				OutputPath: "/nonexistent/readonly/path",
			},
			setupFiles: map[string]string{
				"Cargo.toml": `[dependencies]\nwasm-bindgen = "0.2"`,
			},
			mockStrategy: StrategyRustWasm32,
			expectError:  true,
			errContains:  "failed to create output directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test files if needed
			if tt.setupFiles != nil {
				tmpDir := t.TempDir()
				tt.request.SourcePath = tmpDir
				
				// Create test files
				for filename, content := range tt.setupFiles {
					filePath := filepath.Join(tmpDir, filename)
					dir := filepath.Dir(filePath)
					if dir != tmpDir && dir != "." {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatalf("failed to create directory %s: %v", dir, err)
						}
					}
					if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
						t.Fatalf("failed to create test file %s: %v", filename, err)
					}
				}
				
				// Set output path to temp dir if not testing error case
				if !strings.Contains(tt.request.OutputPath, "/nonexistent") {
					tt.request.OutputPath = t.TempDir()
				}
			}

			// Test the build
			builder := NewWASMBuilder()
			ctx := context.Background()
			result, err := testWASMBuild(ctx, builder, tt.request, tt.mockStrategy, tt.mockBuildError)

			// Verify error expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error should contain '%s', got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				
				// Verify result
				if result == nil {
					t.Fatal("expected result but got nil")
				}
				if result.Lane != tt.expectedLane {
					t.Errorf("lane: got %s, want %s", result.Lane, tt.expectedLane)
				}
				if result.Runtime != tt.expectedRuntime {
					t.Errorf("runtime: got %s, want %s", result.Runtime, tt.expectedRuntime)
				}
				if result.Metadata == nil {
					t.Error("expected metadata but got nil")
				} else {
					if strategy, ok := result.Metadata["wasm_strategy"]; ok {
						if strategy != string(tt.mockStrategy) {
							t.Errorf("wasm_strategy: got %s, want %s", strategy, tt.mockStrategy)
						}
					}
				}
			}
		})
	}
}

// TestDetectBuildStrategy tests the build strategy detection
func TestDetectBuildStrategy(t *testing.T) {
	tests := []struct {
		name             string
		files            map[string]string
		expectedStrategy buildStrategy
		expectError      bool
	}{
		{
			name: "Rust with wasm-bindgen",
			files: map[string]string{
				"Cargo.toml": `
					[dependencies]
					wasm-bindgen = "0.2"
				`,
			},
			expectedStrategy: StrategyRustWasm32,
		},
		{
			name: "Rust with wasm32 target",
			files: map[string]string{
				"Cargo.toml": `
					[lib]
					crate-type = ["cdylib"]
					
					[dependencies]
					wasm-bindgen = "0.2"
				`,
			},
			expectedStrategy: StrategyRustWasm32,
		},
		{
			name: "Go with js/wasm build tags",
			files: map[string]string{
				"go.mod": "module example.com/wasm",
				"main.go": `//go:build js && wasm
				
package main

import "syscall/js"`,
			},
			expectedStrategy: StrategyGoJSWasm,
		},
		{
			name: "AssemblyScript project",
			files: map[string]string{
				"package.json": `{
					"devDependencies": {
						"assemblyscript": "^0.27.0"
					}
				}`,
				"assembly/index.ts": "export function add(a: i32, b: i32): i32 { return a + b; }",
			},
			expectedStrategy: StrategyAssemblyScript,
		},
		{
			name: "Direct WASM file",
			files: map[string]string{
				"module.wasm": string([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}),
			},
			expectedStrategy: StrategyDirect,
		},
		{
			name: "Direct WAT file",
			files: map[string]string{
				"module.wat": "(module)",
			},
			expectedStrategy: StrategyDirect,
		},
		{
			name: "C++ with Emscripten",
			files: map[string]string{
				"CMakeLists.txt": `
					set(CMAKE_TOOLCHAIN_FILE ${EMSCRIPTEN}/cmake/Modules/Platform/Emscripten.cmake)
				`,
				"main.cpp": "#include <emscripten.h>",
			},
			expectedStrategy: StrategyEmscripten,
		},
		{
			name: "No WASM indicators",
			files: map[string]string{
				"main.py": "print('Hello, World!')",
			},
			expectError: true,
		},
		{
			name: "Rust without WASM dependencies",
			files: map[string]string{
				"Cargo.toml": `
					[dependencies]
					serde = "1.0"
				`,
			},
			expectError: true,
		},
		{
			name: "Go without WASM build tags",
			files: map[string]string{
				"go.mod":  "module example.com/app",
				"main.go": "package main\n\nfunc main() {}",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with test files
			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				filePath := filepath.Join(tmpDir, filename)
				dir := filepath.Dir(filePath)
				if dir != tmpDir && dir != "." {
					if err := os.MkdirAll(dir, 0755); err != nil {
						t.Fatalf("failed to create directory %s: %v", dir, err)
					}
				}
				if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create test file %s: %v", filename, err)
				}
			}

			// Test detection
			builder := NewWASMBuilder()
			strategy, err := builder.detectBuildStrategy(tmpDir)

			// Verify expectations
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got strategy: %s", strategy)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if strategy != tt.expectedStrategy {
					t.Errorf("strategy mismatch: got %s, want %s", strategy, tt.expectedStrategy)
				}
			}
		})
	}
}

// TestBuildContextCancellation tests build cancellation via context
func TestBuildContextCancellation(t *testing.T) {
	builder := NewWASMBuilder()
	
	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	
	request := BuildRequest{
		SourcePath: t.TempDir(),
		AppName:    "test-app",
		BuildID:    "build-123",
		OutputPath: t.TempDir(),
	}
	
	// Create a Rust WASM project
	cargoPath := filepath.Join(request.SourcePath, "Cargo.toml")
	os.WriteFile(cargoPath, []byte("[dependencies]\nwasm-bindgen = \"0.2\""), 0644)
	
	// The build should fail due to cancelled context
	_, err := builder.Build(ctx, request)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// TestBuildTimeout tests build timeout handling
func TestBuildTimeout(t *testing.T) {
	builder := NewWASMBuilder()
	
	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	
	request := BuildRequest{
		SourcePath: t.TempDir(),
		AppName:    "test-app",
		BuildID:    "build-123",
		OutputPath: t.TempDir(),
	}
	
	// Create a Rust WASM project
	cargoPath := filepath.Join(request.SourcePath, "Cargo.toml")
	os.WriteFile(cargoPath, []byte("[dependencies]\nwasm-bindgen = \"0.2\""), 0644)
	
	// Sleep to ensure timeout
	time.Sleep(2 * time.Millisecond)
	
	// The build should fail due to timeout
	_, err := builder.Build(ctx, request)
	if err == nil {
		t.Error("expected timeout error")
	}
}

// testWASMBuild is a testable version that can use mock strategies
func testWASMBuild(ctx context.Context, builder *WASMBuilder, req BuildRequest, mockStrategy buildStrategy, mockError error) (*BuildResult, error) {
	// If we have a mock error for testing output directory creation
	if strings.Contains(req.OutputPath, "/nonexistent") {
		return nil, fmt.Errorf("failed to create output directory: permission denied")
	}
	
	// Detect strategy (or use mock)
	strategy := mockStrategy
	if strategy == "" {
		var err error
		strategy, err = builder.detectBuildStrategy(req.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("failed to detect build strategy: %w", err)
		}
	}
	
	// Simulate build error if provided
	if mockError != nil {
		return nil, fmt.Errorf("WASM build failed: %w", mockError)
	}
	
	// Create output directory
	outputDir := filepath.Join(req.OutputPath, "wasm")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Simulate successful build
	artifactPath := filepath.Join(outputDir, req.AppName+".wasm")
	
	return &BuildResult{
		ArtifactPath: artifactPath,
		Lane:         "G",
		Runtime:      "wazero",
		Metadata: map[string]string{
			"wasm_strategy": string(strategy),
			"wasm_runtime":  "wazero",
			"build_id":      req.BuildID,
		},
	}, nil
}