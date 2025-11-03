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

	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/server/events"
	httpserver "github.com/iw2rmb/ploy/internal/server/http"
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

	// Step 3: Initialize events service for SSE fanout (required by handlers).
	eventsService, err := events.New(events.Options{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      nil,
		Store:       db,
	})
	if err != nil {
		t.Fatalf("events.New() failed: %v", err)
	}
	if err := eventsService.Start(ctx); err != nil {
		t.Fatalf("eventsService.Start() failed: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = eventsService.Stop(shutdownCtx)
	}()

	// Step 4: Configure and start HTTP server without TLS.
	httpCfg := config.HTTPConfig{
		Listen: "127.0.0.1:0", // Use port 0 to let OS assign a free port.
		TLS: config.TLSConfig{
			Enabled: false, // Disable TLS for this test.
		},
	}
	httpSrv, err := httpserver.New(httpserver.Options{
		Config:     httpCfg,
		Authorizer: authorizer,
	})
	if err != nil {
		t.Fatalf("httpserver.New() failed: %v", err)
	}

	// Register a simple health handler to verify the server is responding.
	httpSrv.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// Register a basic repos list handler (requires control-plane role).
	httpSrv.HandleFunc("GET /v1/repos", func(w http.ResponseWriter, r *http.Request) {
		repos, err := db.ListRepos(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("list repos failed: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"repos": repos})
	}, auth.RoleControlPlane)

	// Start the server.
	if err := httpSrv.Start(ctx); err != nil {
		t.Fatalf("httpSrv.Start() failed: %v", err)
	}

	// Capture the actual listening address.
	serverAddr := httpSrv.Addr()
	t.Logf("Server started at %s", serverAddr)

	// Step 5: Make an HTTP request to the health endpoint (no TLS required).
	// Wait a brief moment for the server to fully start.
	time.Sleep(100 * time.Millisecond)

	healthURL := fmt.Sprintf("http://%s/health", serverAddr)
	resp, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET %s failed: %v", healthURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, body)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	t.Logf("Health check response: %s", bodyBytes)

	// Step 6: Make an HTTP request to a protected endpoint (repos list).
	// Since AllowInsecure is true with DefaultRole=RoleControlPlane,
	// requests without TLS should be allowed and assigned the control-plane role.
	reposURL := fmt.Sprintf("http://%s/v1/repos", serverAddr)
	resp2, err := http.Get(reposURL)
	if err != nil {
		t.Fatalf("GET %s failed: %v", reposURL, err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("Expected status 200 for repos endpoint, got %d: %s", resp2.StatusCode, body)
	}

	var reposResp map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&reposResp); err != nil {
		t.Fatalf("Failed to decode repos response: %v", err)
	}
	t.Logf("Repos list response: %v", reposResp)

	// Step 7: Stop the server gracefully.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpSrv.Stop(shutdownCtx); err != nil {
		t.Fatalf("httpSrv.Stop() failed: %v", err)
	}

	t.Log("Server stopped successfully")

	// Step 8: Verify the server is no longer responding.
	// We expect a connection error since the server is stopped.
	time.Sleep(100 * time.Millisecond)
	_, err = http.Get(healthURL)
	if err == nil {
		t.Fatalf("Expected connection error after server stop, but request succeeded")
	}
	t.Logf("Verified server stopped (connection error as expected): %v", err)
}
