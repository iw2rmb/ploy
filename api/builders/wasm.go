// Package builders provides WASM builder implementation for Lane G
package builders

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// WASMBuilder implements WASM module building for Lane G
type WASMBuilder struct {
	Name string
	Lane string
}

// BuildRequest represents a build request for WASM modules
type BuildRequest struct {
	SourcePath string
	AppName    string
	BuildID    string
	OutputPath string
}

// BuildResult represents the result of a WASM build
type BuildResult struct {
	ArtifactPath string
	Lane         string
	Runtime      string
	Metadata     map[string]string
}

// buildStrategy represents different WASM compilation approaches
type buildStrategy string

const (
	StrategyRustWasm32     buildStrategy = "rust-wasm32"
	StrategyGoJSWasm       buildStrategy = "go-js-wasm"
	StrategyAssemblyScript buildStrategy = "assemblyscript"
	StrategyEmscripten     buildStrategy = "emscripten"
	StrategyDirect         buildStrategy = "direct-wasm"
)

// NewWASMBuilder creates a new WASM builder instance
func NewWASMBuilder() *WASMBuilder {
	return &WASMBuilder{
		Name: "WASM Builder",
		Lane: "G",
	}
}

// Build executes the WASM build process
func (w *WASMBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	// Detect WASM compilation strategy
	strategy, err := w.detectBuildStrategy(req.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to detect build strategy: %w", err)
	}

	// Create output directory
	outputDir := filepath.Join(req.OutputPath, "wasm")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Execute language-specific build
	wasmPath, err := w.executeBuild(ctx, strategy, req.SourcePath, outputDir, req.AppName)
	if err != nil {
		return nil, fmt.Errorf("WASM build failed: %w", err)
	}

	// Package WASM module with metadata
	artifactPath, err := w.packageWASMModule(wasmPath, req.AppName, outputDir)
	if err != nil {
		return nil, fmt.Errorf("failed to package WASM module: %w", err)
	}

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

// detectBuildStrategy determines the appropriate WASM compilation strategy
func (w *WASMBuilder) detectBuildStrategy(sourcePath string) (buildStrategy, error) {
	// Check for direct WASM files
	if hasFiles, _ := hasAnyFiles(sourcePath, ".wasm", ".wat"); hasFiles {
		return StrategyDirect, nil
	}

	// Check for Rust with WASM target
	if fileExists(filepath.Join(sourcePath, "Cargo.toml")) {
		if hasRustWASMDeps(sourcePath) {
			return StrategyRustWasm32, nil
		}
	}

	// Check for Go with js/wasm target
	if fileExists(filepath.Join(sourcePath, "go.mod")) {
		if hasGoWASMBuildTags(sourcePath) {
			return StrategyGoJSWasm, nil
		}
	}

	// Check for AssemblyScript
	if fileExists(filepath.Join(sourcePath, "package.json")) {
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

// executeBuild runs the appropriate build command for the detected strategy
func (w *WASMBuilder) executeBuild(ctx context.Context, strategy buildStrategy, sourcePath, outputDir, appName string) (string, error) {
	switch strategy {
	case StrategyRustWasm32:
		return w.buildRustWasm32(ctx, sourcePath, outputDir, appName)
	case StrategyGoJSWasm:
		return w.buildGoJSWasm(ctx, sourcePath, outputDir, appName)
	case StrategyAssemblyScript:
		return w.buildAssemblyScript(ctx, sourcePath, outputDir, appName)
	case StrategyEmscripten:
		return w.buildEmscripten(ctx, sourcePath, outputDir, appName)
	case StrategyDirect:
		return w.copyDirectWasm(sourcePath, outputDir, appName)
	default:
		return "", fmt.Errorf("unsupported build strategy: %s", strategy)
	}
}

// buildRustWasm32 builds Rust projects targeting wasm32
func (w *WASMBuilder) buildRustWasm32(ctx context.Context, sourcePath, outputDir, appName string) (string, error) {
	// Add wasm32 target if not present
	cmd := exec.CommandContext(ctx, "rustup", "target", "add", "wasm32-unknown-unknown")
	cmd.Dir = sourcePath
	if err := cmd.Run(); err != nil {
		// Continue even if rustup fails (might already be installed)
	}

	// Build with cargo
	cmd = exec.CommandContext(ctx, "cargo", "build", "--target", "wasm32-unknown-unknown", "--release")
	cmd.Dir = sourcePath
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cargo build failed: %s", string(output))
	}

	// Find the built WASM file
	wasmSrc := filepath.Join(sourcePath, "target", "wasm32-unknown-unknown", "release", appName+".wasm")
	if !fileExists(wasmSrc) {
		// Try with underscore instead of dash
		wasmSrc = filepath.Join(sourcePath, "target", "wasm32-unknown-unknown", "release", strings.ReplaceAll(appName, "-", "_")+".wasm")
	}

	if !fileExists(wasmSrc) {
		return "", fmt.Errorf("WASM artifact not found after build")
	}

	// Copy to output directory
	wasmDst := filepath.Join(outputDir, appName+".wasm")
	return wasmDst, copyFile(wasmSrc, wasmDst)
}

// buildGoJSWasm builds Go projects targeting js/wasm
func (w *WASMBuilder) buildGoJSWasm(ctx context.Context, sourcePath, outputDir, appName string) (string, error) {
	wasmPath := filepath.Join(outputDir, appName+".wasm")

	cmd := exec.CommandContext(ctx, "go", "build", "-o", wasmPath)
	cmd.Dir = sourcePath
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build failed: %s", string(output))
	}

	// Copy wasm_exec.js helper
	goRoot := os.Getenv("GOROOT")
	if goRoot == "" {
		// Get GOROOT from go env
		cmd := exec.CommandContext(ctx, "go", "env", "GOROOT")
		out, err := cmd.Output()
		if err == nil {
			goRoot = strings.TrimSpace(string(out))
		}
	}

	if goRoot != "" {
		wasmExecSrc := filepath.Join(goRoot, "misc", "wasm", "wasm_exec.js")
		wasmExecDst := filepath.Join(outputDir, "wasm_exec.js")
		copyFile(wasmExecSrc, wasmExecDst) // Ignore error, not critical
	}

	return wasmPath, nil
}

// buildAssemblyScript builds AssemblyScript projects
func (w *WASMBuilder) buildAssemblyScript(ctx context.Context, sourcePath, outputDir, appName string) (string, error) {
	// Check if npm is available
	if _, err := exec.LookPath("npm"); err != nil {
		return "", fmt.Errorf("npm not found: %w", err)
	}

	// Install dependencies
	cmd := exec.CommandContext(ctx, "npm", "install")
	cmd.Dir = sourcePath
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("npm install failed: %w", err)
	}

	// Build with AssemblyScript
	wasmPath := filepath.Join(outputDir, appName+".wasm")
	cmd = exec.CommandContext(ctx, "npx", "asc", "assembly/index.ts", "--target", "release", "--outFile", wasmPath)
	cmd.Dir = sourcePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("AssemblyScript build failed: %s", string(output))
	}

	return wasmPath, nil
}

// buildEmscripten builds C/C++ projects with Emscripten
func (w *WASMBuilder) buildEmscripten(ctx context.Context, sourcePath, outputDir, appName string) (string, error) {
	// Check if emcc is available
	if _, err := exec.LookPath("emcc"); err != nil {
		return "", fmt.Errorf("emcc not found: %w", err)
	}

	wasmPath := filepath.Join(outputDir, appName+".wasm")

	// Simple emcc build for main.cpp
	cmd := exec.CommandContext(ctx, "emcc", "-O3", "-s", "WASM=1", "-s", "EXPORTED_FUNCTIONS=[\"_main\"]", "main.cpp", "-o", wasmPath)
	cmd.Dir = sourcePath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("emcc build failed: %s", string(output))
	}

	return wasmPath, nil
}

// copyDirectWasm copies pre-built WASM files
func (w *WASMBuilder) copyDirectWasm(sourcePath, outputDir, appName string) (string, error) {
	// Find .wasm files in source
	var wasmFile string
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".wasm") {
			wasmFile = path
			return filepath.SkipDir // Stop after finding first WASM file
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if wasmFile == "" {
		return "", fmt.Errorf("no WASM files found in source")
	}

	// Copy to output
	wasmDst := filepath.Join(outputDir, appName+".wasm")
	return wasmDst, copyFile(wasmFile, wasmDst)
}

// packageWASMModule creates a deployable WASM package
func (w *WASMBuilder) packageWASMModule(wasmPath, appName, outputDir string) (string, error) {
	// For now, just return the WASM path
	// In the future, this could create a tar.gz with metadata, etc.
	return wasmPath, nil
}

// Helper functions
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasAnyFiles(root string, extensions ...string) (bool, error) {
	found := false
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		for _, ext := range extensions {
			if strings.HasSuffix(path, ext) {
				found = true
				return filepath.SkipDir
			}
		}
		return nil
	})
	return found, err
}

func hasRustWASMDeps(root string) bool {
	cargoToml := filepath.Join(root, "Cargo.toml")
	if !fileExists(cargoToml) {
		return false
	}

	content, err := os.ReadFile(cargoToml)
	if err != nil {
		return false
	}

	contentStr := string(content)
	return strings.Contains(contentStr, "wasm-bindgen") ||
		strings.Contains(contentStr, "js-sys") ||
		strings.Contains(contentStr, "web-sys") ||
		strings.Contains(contentStr, "cdylib")
}

func hasGoWASMBuildTags(root string) bool {
	// Check for js,wasm build tags in Go files
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, ".go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		contentStr := string(content)
		if strings.Contains(contentStr, "js,wasm") || strings.Contains(contentStr, "syscall/js") {
			return fmt.Errorf("found") // Use error to break out of walk
		}
		return nil
	}) != nil
}

func hasAssemblyScriptDeps(root string) bool {
	packageJSON := filepath.Join(root, "package.json")
	if !fileExists(packageJSON) {
		return false
	}

	content, err := os.ReadFile(packageJSON)
	if err != nil {
		return false
	}

	return strings.Contains(string(content), "assemblyscript")
}

func hasEmscriptenToolchain(root string) bool {
	// Check for Emscripten configuration files or headers
	return fileExists(filepath.Join(root, ".emscripten")) ||
		fileExists(filepath.Join(root, "CMakeLists.txt")) // Simplified check
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}