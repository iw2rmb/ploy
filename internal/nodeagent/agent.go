package nodeagent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/pki"
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
	controller := &runController{
		cfg:  cfg,
		runs: make(map[string]*runContext),
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
			select {
			case errCh <- fmt.Errorf("heartbeat: %w", err):
			default:
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.claimer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errCh <- fmt.Errorf("claim loop: %w", err):
			default:
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

	// Generate private key and CSR
	slog.Info("generating private key and CSR")
	keyBundle, csrPEM, err := pki.GenerateNodeCSR(a.cfg.NodeID, a.cfg.ClusterID, "")
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
func (a *Agent) requestCertificate(ctx context.Context, token string, csrPEM []byte) (cert, caCert string, err error) {
	// Create plain HTTPS client (no mTLS - we don't have certs yet)
	// Only verify server's CA certificate
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS13,
			},
		},
	}

	// Prepare request body
	reqBody := map[string]string{
		"csr": string(csrPEM),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimSuffix(a.cfg.ServerURL, "/") + "/v1/pki/bootstrap"

	// Retry with exponential backoff: 1s, 2s, 4s, 8s, 16s (max 5 attempts)
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Info("retrying certificate request", "attempt", attempt+1, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return "", "", ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", "", fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			slog.Warn("certificate request failed", "error", err, "attempt", attempt+1)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			slog.Warn("certificate request returned non-200 status", "status", resp.StatusCode, "attempt", attempt+1)
			continue
		}

		// Parse response
		var result struct {
			Certificate string `json:"certificate"`
			CABundle    string `json:"ca_bundle"`
			BearerToken string `json:"bearer_token"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return "", "", fmt.Errorf("decode response: %w", err)
		}
		resp.Body.Close()

		// Save bearer token if provided
		if result.BearerToken != "" {
			if err := a.saveBearerToken(result.BearerToken); err != nil {
				slog.Error("failed to save bearer token", "err", err)
				// Continue anyway - cert was obtained successfully
			}
		}

		return result.Certificate, result.CABundle, nil
	}

	return "", "", fmt.Errorf("failed to obtain certificate after 5 attempts")
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
