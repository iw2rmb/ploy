// Package runtime provides WASM runtime implementation using wazero
package runtime

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WASMRuntime manages WASM module execution using wazero
type WASMRuntime struct {
	runtime      wazero.Runtime
	config       WASMConfig
	moduleConfig wazero.ModuleConfig
}

// WASMConfig configures the WASM runtime environment
type WASMConfig struct {
	MaxMemoryPages  uint32        // 64KB pages, max 4GB
	MaxExecTime     time.Duration // Maximum execution time
	AllowedSyscalls []string      // Allowed system calls (future use)
	FilesystemRoot  string        // Root directory for filesystem access
}

// DefaultWASMConfig returns a secure default configuration
func DefaultWASMConfig() WASMConfig {
	return WASMConfig{
		MaxMemoryPages: 256,              // 16MB default
		MaxExecTime:    30 * time.Second, // 30 second timeout
		FilesystemRoot: "/tmp/wasm-sandbox",
	}
}

// NewWASMRuntime creates a new WASM runtime instance
func NewWASMRuntime(ctx context.Context, config WASMConfig) (*WASMRuntime, error) {
	// Create wazero runtime
	runtime := wazero.NewRuntime(ctx)

	// Instantiate WASI Preview 1
	_, err := wasi_snapshot_preview1.Instantiate(ctx, runtime)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	// Create default module configuration
	moduleConfig := wazero.NewModuleConfig().WithName("wasm-app")

	// Configure filesystem if specified
	if config.FilesystemRoot != "" {
		// Ensure sandbox directory exists
		if err := os.MkdirAll(config.FilesystemRoot, 0755); err != nil {
			return nil, fmt.Errorf("failed to create filesystem root: %w", err)
		}

		fsConfig := wazero.NewFSConfig().WithDirMount(config.FilesystemRoot, "/")
		moduleConfig = moduleConfig.WithFSConfig(fsConfig)
	}

	return &WASMRuntime{
		runtime:      runtime,
		config:       config,
		moduleConfig: moduleConfig,
	}, nil
}

// LoadModule compiles and loads a WASM module from bytes
func (w *WASMRuntime) LoadModule(ctx context.Context, wasmBytes []byte, moduleName string) (wazero.CompiledModule, error) {
	// Compile the WASM module
	compiledModule, err := w.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to compile WASM module: %w", err)
	}

	return compiledModule, nil
}

// ExecuteModule instantiates and executes a WASM module
func (w *WASMRuntime) ExecuteModule(ctx context.Context, compiledModule wazero.CompiledModule, args []string) error {
	// Create execution context with timeout
	if w.config.MaxExecTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.config.MaxExecTime)
		defer cancel()
	}

	// Configure module with arguments
	moduleConfig := w.moduleConfig
	if len(args) > 0 {
		moduleConfig = moduleConfig.WithArgs(args...)
	}

	// Instantiate the module
	module, err := w.runtime.InstantiateModule(ctx, compiledModule, moduleConfig)
	if err != nil {
		return fmt.Errorf("failed to instantiate WASM module: %w", err)
	}
	defer module.Close(ctx)

	// Execute the main function or _start
	if startFn := module.ExportedFunction("_start"); startFn != nil {
		_, err = startFn.Call(ctx)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}
	} else if mainFn := module.ExportedFunction("main"); mainFn != nil {
		_, err = mainFn.Call(ctx)
		if err != nil {
			return fmt.Errorf("execution failed: %w", err)
		}
	} else {
		return fmt.Errorf("no main or _start function found in WASM module")
	}

	return nil
}

// ExecuteModuleFromFile loads and executes a WASM module from file
func (w *WASMRuntime) ExecuteModuleFromFile(ctx context.Context, wasmPath string, args []string) error {
	// Read WASM module
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("failed to read WASM file: %w", err)
	}

	// Load module
	compiledModule, err := w.LoadModule(ctx, wasmBytes, "app")
	if err != nil {
		return err
	}

	// Execute module
	return w.ExecuteModule(ctx, compiledModule, args)
}

// ConfigureWASI configures WASI with custom settings
func (w *WASMRuntime) ConfigureWASI(config WASIConfig) error {
	// Create filesystem configuration
	fsConfig := wazero.NewFSConfig()

	// Add preopen directories (sandbox filesystem)
	for guestPath, hostPath := range config.Preopens {
		if err := os.MkdirAll(hostPath, 0755); err != nil {
			return fmt.Errorf("failed to create preopen directory %s: %w", hostPath, err)
		}
		fsConfig = fsConfig.WithDirMount(hostPath, guestPath)
	}

	// Configure module with WASI
	moduleConfig := wazero.NewModuleConfig().WithFSConfig(fsConfig)

	// Add arguments
	if len(config.Args) > 0 {
		moduleConfig = moduleConfig.WithArgs(config.Args...)
	}

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

// Close releases all resources
func (w *WASMRuntime) Close(ctx context.Context) error {
	return w.runtime.Close(ctx)
}

// Runtime returns the underlying wazero runtime for advanced operations
func (w *WASMRuntime) Runtime() wazero.Runtime {
	return w.runtime
}

// DefaultModuleConfig returns the default module configuration
func DefaultModuleConfig() wazero.ModuleConfig {
	return wazero.NewModuleConfig().WithName("default")
}

// WASIConfig configures WASI (WebAssembly System Interface) settings
type WASIConfig struct {
	Args     []string          // Command line arguments
	Env      map[string]string // Environment variables
	Preopens map[string]string // guest_path -> host_path mappings
	Stdin    io.Reader         // Standard input
	Stdout   io.Writer         // Standard output
	Stderr   io.Writer         // Standard error
}

// DefaultWebWASIConfig returns a configuration suitable for web applications
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

// ServerWASIConfig returns a configuration suitable for server applications
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
