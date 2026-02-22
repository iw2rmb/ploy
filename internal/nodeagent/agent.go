package nodeagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/workflow/backoff"
)

// Agent coordinates the node agent's HTTP server, heartbeat manager, and claim loop.
type Agent struct {
	cfg        Config
	server     *Server
	heartbeat  *HeartbeatManager
	claimer    *ClaimManager
	controller *runController
}

// New constructs a new node agent.
func New(cfg Config) (*Agent, error) {
	// Create shared HTTP uploaders once at initialization.
	// These are reused across all jobs to avoid creating new HTTP clients per
	// upload call, which enables connection pooling and reduces overhead.
	// The underlying http.Client is safe for concurrent use by multiple goroutines.
	diffUploader, err := NewDiffUploader(cfg)
	if err != nil {
		return nil, fmt.Errorf("create diff uploader: %w", err)
	}

	artifactUploader, err := NewArtifactUploader(cfg)
	if err != nil {
		return nil, fmt.Errorf("create artifact uploader: %w", err)
	}

	statusUploader, err := newBaseUploader(cfg)
	if err != nil {
		return nil, fmt.Errorf("create status uploader: %w", err)
	}

	jobImageNameSaver, err := NewJobImageNameSaver(cfg)
	if err != nil {
		return nil, fmt.Errorf("create job image name saver: %w", err)
	}

	// Initialize controller with typed JobID keys for compile-time safety.
	// The shared uploaders are passed in to enable HTTP client reuse.
	controller := &runController{
		cfg:               cfg,
		jobs:              make(map[types.JobID]*jobContext),
		diffUploader:      diffUploader,
		artifactUploader:  artifactUploader,
		statusUploader:    statusUploader,
		jobImageNameSaver: jobImageNameSaver,
	}

	server, err := NewServer(cfg, controller)
	if err != nil {
		return nil, fmt.Errorf("create server: %w", err)
	}

	heartbeat, err := NewHeartbeatManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("create heartbeat manager: %w", err)
	}

	claimer, err := NewClaimManager(cfg, controller)
	if err != nil {
		return nil, fmt.Errorf("create claim manager: %w", err)
	}

	return &Agent{
		cfg:        cfg,
		server:     server,
		heartbeat:  heartbeat,
		claimer:    claimer,
		controller: controller,
	}, nil
}

// Run starts the node agent and blocks until the context is canceled.
func (a *Agent) Run(ctx context.Context) error {
	// Perform bootstrap if needed (exchange token for certificate)
	if a.cfg.HTTP.TLS.Enabled {
		if err := a.bootstrap(ctx); err != nil {
			return fmt.Errorf("bootstrap: %w", err)
		}
	}

	if err := a.server.Start(ctx); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	slog.Info("node http server listening", "addr", a.server.Address())

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.heartbeat.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			wrappedErr := fmt.Errorf("heartbeat: %w", err)
			select {
			case errCh <- wrappedErr:
			default:
				slog.Error("error channel full, dropping error", "component", "heartbeat", "error", err)
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.claimer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			wrappedErr := fmt.Errorf("claim loop: %w", err)
			select {
			case errCh <- wrappedErr:
			default:
				slog.Error("error channel full, dropping error", "component", "claim loop", "error", err)
			}
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := a.server.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("stop server: %w", err)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// bootstrap performs first-time node provisioning by exchanging a bootstrap token
// for a signed certificate. Returns nil if certificates already exist.
func (a *Agent) bootstrap(ctx context.Context) error {
	// Check if certificates already exist
	certExists := fileExists(a.cfg.HTTP.TLS.CertPath) && fileExists(a.cfg.HTTP.TLS.KeyPath)
	if certExists {
		slog.Debug("certificates already exist, skipping bootstrap")
		return nil
	}

	slog.Info("starting bootstrap process")

	// Read bootstrap token from secure location
	tokenPath := "/run/ploy/bootstrap-token"
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read bootstrap token: %w", err)
	}
	bootstrapToken := strings.TrimSpace(string(tokenBytes))

	// Generate private key and CSR (convert domain types to strings for pki package)
	slog.Info("generating private key and CSR")
	keyBundle, csrPEM, err := pki.GenerateNodeCSR(a.cfg.NodeID.String(), a.cfg.ClusterID.String(), "")
	if err != nil {
		return fmt.Errorf("generate CSR: %w", err)
	}

	// Exchange bootstrap token for certificate
	slog.Info("requesting certificate from server")
	cert, caCert, err := a.requestCertificate(ctx, bootstrapToken, csrPEM)
	if err != nil {
		return fmt.Errorf("request certificate: %w", err)
	}

	// Write CA certificate
	slog.Info("writing certificates to disk")
	if err := os.MkdirAll("/etc/ploy/pki", 0755); err != nil {
		return fmt.Errorf("create pki directory: %w", err)
	}

	if err := os.WriteFile(a.cfg.HTTP.TLS.CAPath, []byte(caCert), 0644); err != nil {
		return fmt.Errorf("write CA cert: %w", err)
	}

	// Write node certificate
	if err := os.WriteFile(a.cfg.HTTP.TLS.CertPath, []byte(cert), 0644); err != nil {
		return fmt.Errorf("write node cert: %w", err)
	}

	// Write node private key (restricted permissions)
	if err := os.WriteFile(a.cfg.HTTP.TLS.KeyPath, []byte(keyBundle.KeyPEM), 0600); err != nil {
		return fmt.Errorf("write node key: %w", err)
	}

	// Delete bootstrap token
	if err := os.Remove(tokenPath); err != nil {
		slog.Warn("failed to delete bootstrap token", "error", err)
	}

	slog.Info("bootstrap complete")
	return nil
}

// requestCertificate exchanges a bootstrap token for a signed certificate.
// It retries with exponential backoff to handle temporary network failures.
// Uses shared backoff policy (1s initial, 2x multiplier, 5 attempts total).
func (a *Agent) requestCertificate(ctx context.Context, token string, csrPEM []byte) (cert, caCert string, err error) {
	// Build TLS config for bootstrap: verify server CA using pinned CA bundle.
	// Fallback order:
	//   1. BootstrapCAPath (explicit bootstrap CA)
	//   2. CAPath if file exists (cluster CA from previous bootstrap)
	//   3. System roots (for public PKI / first boot on public infra)
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// Determine which CA bundle to use for server verification.
	bootstrapCAPath := a.cfg.HTTP.TLS.BootstrapCAPath
	if bootstrapCAPath == "" && fileExists(a.cfg.HTTP.TLS.CAPath) {
		// Fall back to cluster CA if it exists (e.g., after previous bootstrap).
		bootstrapCAPath = a.cfg.HTTP.TLS.CAPath
	}

	if bootstrapCAPath != "" {
		// Load pinned CA bundle for server verification.
		caData, readErr := os.ReadFile(bootstrapCAPath)
		if readErr != nil {
			return "", "", fmt.Errorf("read bootstrap CA from %s: %w", bootstrapCAPath, readErr)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caData) {
			return "", "", fmt.Errorf("parse bootstrap CA from %s: no valid certificates found", bootstrapCAPath)
		}
		tlsCfg.RootCAs = pool
		slog.Debug("bootstrap TLS: using pinned CA", "path", bootstrapCAPath)
	} else {
		// No pinned CA available; use system roots (public PKI scenario).
		slog.Debug("bootstrap TLS: using system roots (no pinned CA configured)")
	}

	// Create HTTPS client (no client cert - we don't have one yet).
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	// Prepare request body.
	reqBody := map[string]string{
		"csr": string(csrPEM),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(a.cfg.ServerURL, "/") + "/v1/pki/bootstrap"

	// Variables to capture successful response.
	var certResult, caResult string

	// Track attempt number for structured logging (matches existing log format).
	attemptNum := 0

	// Retry with exponential backoff using shared policy.
	// Policy: 1s initial interval, 2x multiplier, 5 total attempts.
	policy := backoff.CertificateBootstrapPolicy()
	operation := func() error {
		attemptNum++

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		if !a.cfg.NodeID.IsZero() {
			req.Header.Set("PLOY_NODE_UUID", a.cfg.NodeID.String())
		}

		resp, err := client.Do(req)
		if err != nil {
			// Network error - retry with backoff.
			slog.Warn("certificate request failed", "error", err, "attempt", attemptNum)
			return err
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			// Non-200 status - retry with backoff.
			slog.Warn("certificate request returned non-200 status", "status", resp.StatusCode, "attempt", attemptNum)
			return fmt.Errorf("server returned status %d", resp.StatusCode)
		}

		// Parse response.
		var result struct {
			Certificate string `json:"certificate"`
			CABundle    string `json:"ca_bundle"`
			BearerToken string `json:"bearer_token"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}

		// Save bearer token if provided.
		if result.BearerToken != "" {
			if err := a.saveBearerToken(result.BearerToken); err != nil {
				slog.Error("failed to save bearer token", "err", err)
				// Continue anyway - cert was obtained successfully.
			}
		}

		// Capture result for return.
		certResult = result.Certificate
		caResult = result.CABundle
		return nil
	}

	// Run with backoff. The shared backoff helper handles:
	// - Exponential backoff intervals with jitter
	// - Context cancellation
	// - Max attempts enforcement
	// - Structured logging of retry attempts
	err = backoff.RunWithBackoff(ctx, policy, slog.Default(), operation)
	if err != nil {
		return "", "", fmt.Errorf("failed to obtain certificate after %d attempts: %w", attemptNum, err)
	}

	return certResult, caResult, nil
}

// saveBearerToken saves the worker bearer token to a file for use in API requests.
func (a *Agent) saveBearerToken(token string) error {
	tokenPath := bearerTokenPath()
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("write bearer token: %w", err)
	}
	slog.Info("bearer token saved", "path", tokenPath)
	return nil
}

// fileExists checks if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
