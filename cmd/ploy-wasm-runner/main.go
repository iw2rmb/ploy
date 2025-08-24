// ploy-wasm-runner - WebAssembly Runtime Engine for Lane G Deployments
//
// This is a DEPLOYMENT RUNTIME COMPONENT, not a CLI tool.
// It runs INSIDE deployed containers to execute WASM modules.
//
// Architecture Role:
//   - Deployed as the main process in Lane G containers
//   - Executes compiled WASM modules via HTTP interface
//   - Similar to Node.js runtime, JVM, or Python interpreter
//
// Deployment Flow:
//   1. Controller compiles user code to WASM
//   2. Controller builds this binary for target architecture
//   3. Both get packaged into deployment artifact
//   4. Nomad deploys container with this as entrypoint
//   5. This serves HTTP by executing the WASM module
//
// See README.md for detailed documentation.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iw2rmb/ploy/controller/runtime"
)

func main() {
	var (
		modulePath    = flag.String("module", "", "Path to WASM module")
		port          = flag.String("port", "8080", "HTTP port")
		maxMemory     = flag.String("max-memory", "32MB", "Maximum memory for WASM module")
		timeout       = flag.String("timeout", "30s", "Execution timeout")
		envVars       = flag.String("env", "", "Environment variables (comma-separated key=value pairs)")
		wasiRoot      = flag.String("wasi-root", "/tmp/wasm-sandbox", "WASI filesystem root")
		logLevel      = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	if *modulePath == "" {
		log.Fatal("--module flag is required")
	}

	// Configure logging
	setupLogging(*logLevel)

	// Parse configuration
	config, err := parseConfig(*maxMemory, *timeout, *envVars, *wasiRoot)
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Initialize WASM runtime
	ctx := context.Background()
	wasmRuntime, err := runtime.NewWASMRuntime(ctx, config)
	if err != nil {
		log.Fatalf("Failed to initialize WASM runtime: %v", err)
	}
	defer wasmRuntime.Close(ctx)

	// Load WASM module
	wasmBytes, err := os.ReadFile(*modulePath)
	if err != nil {
		log.Fatalf("Failed to read WASM module: %v", err)
	}

	compiledModule, err := wasmRuntime.LoadModule(ctx, wasmBytes, "app")
	if err != nil {
		log.Fatalf("Failed to load WASM module: %v", err)
	}

	log.Printf("WASM module loaded successfully: %s", *modulePath)

	// Setup HTTP server
	mux := http.NewServeMux()
	
	// Main application handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		requestCtx, cancel := context.WithTimeout(r.Context(), config.MaxExecTime)
		defer cancel()

		// Execute WASM module for each request
		err := wasmRuntime.ExecuteModule(requestCtx, compiledModule, []string{})
		if err != nil {
			log.Printf("WASM execution error: %v", err)
			http.Error(w, fmt.Sprintf("WASM execution failed: %v", err), http.StatusInternalServerError)
			return
		}

		// For now, return success message
		// In production, WASM modules would handle HTTP requests directly
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
  "status": "success",
  "message": "WASM module executed successfully",
  "runtime": "wazero",
  "timestamp": "%s"
}`, time.Now().UTC().Format(time.RFC3339))
	})

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
  "status": "healthy",
  "service": "ploy-wasm-runner",
  "runtime": "wazero",
  "module": "%s",
  "uptime": "%s"
}`, *modulePath, time.Since(startTime).String())
	})

	// WASM runtime health check
	mux.HandleFunc("/wasm-health", func(w http.ResponseWriter, r *http.Request) {
		// Quick WASM module validation
		testCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Try to instantiate module (lightweight operation)
		module, err := wasmRuntime.Runtime().InstantiateModule(testCtx, compiledModule, runtime.DefaultModuleConfig())
		if err != nil {
			http.Error(w, fmt.Sprintf("WASM runtime unhealthy: %v", err), http.StatusServiceUnavailable)
			return
		}
		module.Close(testCtx)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{
  "status": "healthy",
  "wasm_runtime": "wazero",
  "module_loaded": true,
  "max_memory_mb": %d,
  "timeout": "%s"
}`, config.MaxMemoryPages*64/1024, config.MaxExecTime.String())
	})

	// Metrics endpoint for monitoring
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// Basic Prometheus-style metrics
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintf(w, "# HELP wasm_executions_total Total number of WASM executions\n")
		fmt.Fprintf(w, "# TYPE wasm_executions_total counter\n")
		fmt.Fprintf(w, "wasm_executions_total %d\n", executionCount)
		
		fmt.Fprintf(w, "# HELP wasm_execution_duration_seconds Duration of WASM executions\n")
		fmt.Fprintf(w, "# TYPE wasm_execution_duration_seconds histogram\n")
		fmt.Fprintf(w, "wasm_execution_duration_seconds_sum %.3f\n", totalExecutionTime.Seconds())
		fmt.Fprintf(w, "wasm_execution_duration_seconds_count %d\n", executionCount)
		
		fmt.Fprintf(w, "# HELP wasm_memory_pages_max Maximum memory pages configured\n")
		fmt.Fprintf(w, "# TYPE wasm_memory_pages_max gauge\n")
		fmt.Fprintf(w, "wasm_memory_pages_max %d\n", config.MaxMemoryPages)
	})

	// Create HTTP server
	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: config.MaxExecTime + 10*time.Second, // Allow extra time for response
		IdleTimeout:  120 * time.Second,
	}

	// Setup graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down WASM runner...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		server.Shutdown(shutdownCtx)
	}()

	// Start server
	log.Printf("Starting WASM HTTP server on port %s", *port)
	log.Printf("WASM module: %s", *modulePath)
	log.Printf("Max memory: %s (%d pages)", *maxMemory, config.MaxMemoryPages)
	log.Printf("Execution timeout: %s", config.MaxExecTime)
	
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("HTTP server error: %v", err)
	}

	log.Println("WASM runner shutdown complete")
}

// Global metrics (simple counters for now)
var (
	startTime           = time.Now()
	executionCount      int64
	totalExecutionTime  time.Duration
)

// parseConfig parses command line configuration into WASMConfig
func parseConfig(maxMemory, timeout, envVars, wasiRoot string) (runtime.WASMConfig, error) {
	config := runtime.DefaultWASMConfig()

	// Parse memory limit
	if maxMemory != "" {
		memoryBytes, err := parseMemorySize(maxMemory)
		if err != nil {
			return config, fmt.Errorf("invalid memory size: %w", err)
		}
		config.MaxMemoryPages = uint32(memoryBytes / (64 * 1024)) // Convert to 64KB pages
		if config.MaxMemoryPages == 0 {
			config.MaxMemoryPages = 1 // Minimum 1 page (64KB)
		}
	}

	// Parse timeout
	if timeout != "" {
		duration, err := time.ParseDuration(timeout)
		if err != nil {
			return config, fmt.Errorf("invalid timeout: %w", err)
		}
		config.MaxExecTime = duration
	}

	// Set WASI root
	config.FilesystemRoot = wasiRoot

	return config, nil
}

// parseMemorySize parses memory size strings like "32MB", "1GB", "512KB"
func parseMemorySize(size string) (int64, error) {
	size = strings.ToUpper(strings.TrimSpace(size))
	
	var multiplier int64 = 1
	var numStr string

	if strings.HasSuffix(size, "KB") {
		multiplier = 1024
		numStr = strings.TrimSuffix(size, "KB")
	} else if strings.HasSuffix(size, "MB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(size, "MB")
	} else if strings.HasSuffix(size, "GB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(size, "GB")
	} else if strings.HasSuffix(size, "B") {
		multiplier = 1
		numStr = strings.TrimSuffix(size, "B")
	} else {
		// Assume bytes if no suffix
		numStr = size
	}

	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return num * multiplier, nil
}

// setupLogging configures logging level and format
func setupLogging(level string) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	switch strings.ToLower(level) {
	case "debug":
		log.SetOutput(os.Stdout)
	case "info":
		log.SetOutput(os.Stdout)
	case "warn", "warning":
		// Could implement different log levels here
		log.SetOutput(os.Stdout)
	case "error":
		log.SetOutput(os.Stderr)
	default:
		log.SetOutput(os.Stdout)
	}
}