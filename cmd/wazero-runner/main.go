package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/tetratelabs/wazero"
	wasi "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func main() {
	modulePath := flag.String("module", "", "Path to local WASM module file")
	moduleURL := flag.String("url", "", "URL to fetch WASM module from (optional)")
	port := flag.Int("port", 8080, "HTTP port for health endpoint")
	ignoreErrors := flag.Bool("ignore-errors", false, "Keep process alive and report /healthz=503 on module errors")
	flag.Parse()

	if *modulePath == "" && *moduleURL == "" {
		fmt.Println("wazero-runner: either -module or -url is required")
		os.Exit(2)
	}

	// If URL given, download to a temp file
	if *moduleURL != "" {
		tmp, err := os.CreateTemp("", "module-*.wasm")
		if err != nil {
			fmt.Fprintf(os.Stderr, "download temp create failed: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = tmp.Close() }()
		if err := fetchToFile(*moduleURL, tmp); err != nil {
			fmt.Fprintf(os.Stderr, "download failed: %v\n", err)
			os.Exit(1)
		}
		*modulePath = tmp.Name()
		fmt.Printf("Fetched WASM module to %s\n", *modulePath)
	}

	// Start a simple health server (reports 200 OK when no error; 503 with details if lastErr set)
	var lastErr error
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if lastErr == nil {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("ok"))
			return
		}
		w.WriteHeader(503)
		_, _ = w.Write([]byte("error: " + lastErr.Error()))
	})
	addr := fmt.Sprintf(":%d", *port)
	go func() { _ = http.ListenAndServe(addr, mux) }()

	// Run the module once under WASI and exit; keep process alive for observability
	if err := runOnce(*modulePath); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		lastErr = err
		if !*ignoreErrors {
			os.Exit(1)
		}
		// keep alive to serve /healthz=503
		select {}
	} else {
		fmt.Println("Module executed (_start) successfully or exited")
		// keep alive to serve /healthz=200
		select {}
	}
}

func fetchToFile(url string, out *os.File) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

func runOnce(path string) error {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer func() { _ = rt.Close(ctx) }()
	if _, err := wasi.Instantiate(ctx, rt); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	compiled, err := rt.CompileModule(ctx, b)
	if err != nil {
		return err
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		return err
	}
	// Call _start if present
	if fn := mod.ExportedFunction("_start"); fn != nil {
		if _, err := fn.Call(ctx); err != nil {
			// Ignore traps to avoid failing the whole runner
			if !isExitOK(err) {
				return err
			}
		}
	}
	return nil
}

func isExitOK(err error) bool {
	// Consider all traps as non-fatal for scaffold
	if err == nil {
		return true
	}
	// wazero returns an api.ExitError on module exit; treat as OK
	type exitErr interface{ ExitCode() uint32 }
	if ee, ok := err.(exitErr); ok {
		_ = ee
		return true
	}
	return false
}
