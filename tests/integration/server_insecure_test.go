package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
)

// TestServerStartStop_InsecureMode verifies that the server can start and stop
// with mTLS disabled using AllowInsecure authorizer (test-only configuration).
//
// This test requires a test database accessible via PLOY_TEST_PG_DSN.
func TestServerStartStop_InsecureMode(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Initialize store.
	db, err := store.NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Step 2: Create an AllowInsecure authorizer (test-only).
	// In production, AllowInsecure should always be false.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: true,
		DefaultRole:   auth.RoleControlPlane,
	})

	// Step 3: Configure and start HTTP server without TLS.
	httpCfg := config.HTTPConfig{
		Listen: "127.0.0.1:0", // Use port 0 to let OS assign a free port.
	}
	httpSrv, err := server.NewHTTPServer(server.HTTPOptions{
		Config:     httpCfg,
		Authorizer: authorizer,
	})
	if err != nil {
		t.Fatalf("server.NewHTTPServer() failed: %v", err)
	}

	// Register a simple health handler to verify the server is responding.
	httpSrv.RegisterRouteFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Register a basic runs list handler (requires control-plane role).
	httpSrv.RegisterRouteFunc("GET /v1/runs", func(w http.ResponseWriter, r *http.Request) {
		runs, err := db.ListRuns(r.Context(), store.ListRunsParams{Limit: 10, Offset: 0})
		if err != nil {
			http.Error(w, fmt.Sprintf("list runs failed: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"runs": runs})
	}, auth.RoleControlPlane)

	// Start the server.
	if err := httpSrv.Start(ctx); err != nil {
		t.Fatalf("httpSrv.Start() failed: %v", err)
	}

	// Capture the actual listening address.
	serverAddr := httpSrv.Addr()
	t.Logf("Server started at %s", serverAddr)

	// Step 4: Make an HTTP request to the health endpoint (no TLS required).
	// Wait a brief moment for the server to fully start.
	time.Sleep(100 * time.Millisecond)

	healthURL := fmt.Sprintf("http://%s/health", serverAddr)
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET %s failed: %v", healthURL, err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, body)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	t.Logf("Health check response: %s", bodyBytes)

	// Step 5: Make an HTTP request to a protected endpoint (runs list).
	// Since AllowInsecure is true with DefaultRole=RoleControlPlane,
	// requests without TLS should be allowed and assigned the control-plane role.
	runsURL := fmt.Sprintf("http://%s/v1/runs", serverAddr)
	resp2, err := http.Get(runsURL)
	if err != nil {
		t.Fatalf("GET %s failed: %v", runsURL, err)
	}
	defer func() {
		_ = resp2.Body.Close()
	}()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("Expected status 200 for runs endpoint, got %d: %s", resp2.StatusCode, body)
	}

	var runsResp map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&runsResp); err != nil {
		t.Fatalf("Failed to decode runs response: %v", err)
	}
	t.Logf("Runs list response: %v", runsResp)

	// Step 6: Stop the server gracefully.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Stop(shutdownCtx); err != nil {
		t.Fatalf("httpSrv.Stop() failed: %v", err)
	}

	t.Log("Server stopped successfully")

	// Step 7: Verify the server is no longer responding.
	// We expect a connection error since the server is stopped.
	time.Sleep(100 * time.Millisecond)
	resp3, err := http.Get(healthURL)
	if err == nil && resp3 != nil {
		_ = resp3.Body.Close()
		t.Fatalf("Expected connection error after server stop, but request succeeded")
	}
	t.Logf("Verified server stopped (connection error as expected): %v", err)
}
