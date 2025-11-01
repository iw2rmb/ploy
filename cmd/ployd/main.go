package main

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/api/config"
	"github.com/iw2rmb/ploy/internal/api/httpserver"
	"github.com/iw2rmb/ploy/internal/api/metrics"
	"github.com/iw2rmb/ploy/internal/api/pki"
	"github.com/iw2rmb/ploy/internal/api/scheduler"
	"github.com/iw2rmb/ploy/internal/controlplane/auth"
	internalPKI "github.com/iw2rmb/ploy/internal/pki"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/store/ttlworker"
)

func main() {
	// Allow env to supply the default config path; CLI flag still has highest precedence.
	defaultConfigPath := strings.TrimSpace(os.Getenv("PLOYD_CONFIG_PATH"))
	if defaultConfigPath == "" {
		defaultConfigPath = "/etc/ploy/ployd.yaml"
	}

	var configPath string
	flag.StringVar(&configPath, "config", defaultConfigPath, "Path to ployd configuration (flag overrides $PLOYD_CONFIG_PATH)")
	flag.Parse()

	// Configure structured logger early (will be reconfigured after loading config).
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})))

	// Load configuration from file.
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config", "err", err, "path", configPath)
		os.Exit(1)
	}

	// Reconfigure logger based on config.
	if err := initLogging(cfg.Logging); err != nil {
		slog.Error("initialize logging", "err", err)
		os.Exit(1)
	}

	// Resolve PostgreSQL DSN from environment or config.
	dsn := resolvePgDSN(cfg)
	if dsn == "" {
		slog.Error("postgresql dsn not configured", "hint", "set PLOY_SERVER_PG_DSN or configure postgres.dsn in config file")
		os.Exit(1)
	}

	// Initialize store.
	ctx := context.Background()
	st, err := store.NewStore(ctx, dsn)
	if err != nil {
		slog.Error("initialize store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	// Initialize Authorizer for mTLS-based authentication.
	// Default role is RoleControlPlane; AllowInsecure is false for production.
	authorizer := auth.NewAuthorizer(auth.Options{
		AllowInsecure: false,
		DefaultRole:   auth.RoleControlPlane,
	})

	// Reflect configured transport settings in startup logs (before listeners come up).
	slog.Info("ployd server starting",
		"config", configPath,
		"tls", cfg.HTTP.TLS.Enabled,
		"mtls", cfg.HTTP.TLS.RequireClientCert,
	)

	// Set up signal handling for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Run server.
	if err := run(ctx, cfg, configPath, st, authorizer); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}

	slog.Info("ployd server stopped")
}

// run executes the main server loop and blocks until the context is canceled.
func run(ctx context.Context, cfg config.Config, configPath string, st store.Store, authorizer *auth.Authorizer) error {
	// Initialize PKI manager for certificate renewal.
	rotator := pki.NewDefaultRotator(slog.Default())
	pkiManager, err := pki.New(pki.Options{
		Config:  cfg.PKI,
		Rotator: rotator,
	})
	if err != nil {
		return fmt.Errorf("create pki manager: %w", err)
	}

	// Initialize config watcher for hot-reload.
	configWatcher, err := config.NewWatcher(config.WatcherOptions{
		Path:   configPath,
		Logger: slog.Default(),
	})
	if err != nil {
		return fmt.Errorf("create config watcher: %w", err)
	}

	// Subscribe PKI manager to config changes.
	configWatcher.Subscribe(pkiManager)

	// Initialize TTL worker.
	ttlWorker, err := ttlworker.New(ttlworker.Options{
		Store:          st,
		TTL:            cfg.Scheduler.TTL,
		Interval:       cfg.Scheduler.TTLInterval,
		Logger:         slog.Default(),
		DropPartitions: cfg.Scheduler.DropPartitions,
	})
	if err != nil {
		return fmt.Errorf("create ttl worker: %w", err)
	}

	// Initialize scheduler and register background tasks.
	sched := scheduler.New()
	if ttlWorker != nil {
		sched.AddTask(ttlWorker)
	}

	// Start PKI manager.
	if err := pkiManager.Start(ctx); err != nil {
		return fmt.Errorf("start pki manager: %w", err)
	}

	// Start config watcher.
	if err := configWatcher.Start(ctx); err != nil {
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start config watcher: %w", err)
	}

	// Start scheduler.
	if err := sched.Start(ctx); err != nil {
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start scheduler: %w", err)
	}

	// Initialize HTTP server for API endpoints.
	httpSrv, err := httpserver.New(httpserver.Options{
		Config:     cfg.HTTP,
		Authorizer: authorizer,
	})
	if err != nil {
		return fmt.Errorf("create http server: %w", err)
	}

	// Register health endpoint.
	httpSrv.HandleFunc("/health", healthHandler)

	// Register PKI sign endpoint (admin-only).
	httpSrv.HandleFunc("POST /v1/pki/sign", pkiSignHandler(st), auth.RoleCLIAdmin)

	// Register repos endpoints (control plane).
	httpSrv.HandleFunc("POST /v1/repos", createRepoHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/repos", listReposHandler(st), auth.RoleControlPlane)

	// Initialize metrics server.
	metricsSrv := metrics.New(metrics.Options{
		Listen: cfg.Metrics.Listen,
	})

	// Start HTTP server.
	if err := httpSrv.Start(ctx); err != nil {
		// Ensure background tasks are stopped on failure.
		_ = sched.Stop(context.Background())
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start http server: %w", err)
	}

	// Start metrics server.
	if err := metricsSrv.Start(ctx); err != nil {
		// Stop HTTP server on failure to start metrics.
		_ = httpSrv.Stop(context.Background())
		// Stop scheduler to avoid leaking background goroutines.
		_ = sched.Stop(context.Background())
		_ = configWatcher.Stop(context.Background())
		_ = pkiManager.Stop(context.Background())
		return fmt.Errorf("start metrics server: %w", err)
	}

	slog.Info("ployd servers started",
		"api", httpSrv.Addr(),
		"metrics", metricsSrv.Addr(),
	)

	// Wait for shutdown signal.
	<-ctx.Done()

	// Create a timeout context for graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("graceful shutdown initiated", "timeout", "10s")

	// Stop scheduler.
	if err := sched.Stop(shutdownCtx); err != nil {
		slog.Error("stop scheduler", "err", err)
	}

	// Stop config watcher.
	if err := configWatcher.Stop(shutdownCtx); err != nil {
		slog.Error("stop config watcher", "err", err)
	}

	// Stop PKI manager.
	if err := pkiManager.Stop(shutdownCtx); err != nil {
		slog.Error("stop pki manager", "err", err)
	}

	// Stop HTTP server.
	if err := httpSrv.Stop(shutdownCtx); err != nil {
		slog.Error("stop http server", "err", err)
	}

	// Stop metrics server.
	if err := metricsSrv.Stop(shutdownCtx); err != nil {
		slog.Error("stop metrics server", "err", err)
	}

	return nil
}

// healthHandler responds to health check requests.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
}

// pkiSignHandler returns an HTTP handler that signs node CSRs with the cluster CA.
// It requires admin role authorization and returns a PEM bundle with the signed certificate.
func pkiSignHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			NodeID string `json:"node_id"`
			CSR    string `json:"csr"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate node_id format.
		nodeUUID, err := uuid.Parse(req.NodeID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid node_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate CSR is not empty.
		if strings.TrimSpace(req.CSR) == "" {
			http.Error(w, "csr field is required", http.StatusBadRequest)
			return
		}

		// Load cluster CA from environment (treat whitespace as unset), but preserve
		// original values for downstream use to avoid altering PEM formatting.
		rawCACert := os.Getenv("PLOY_SERVER_CA_CERT")
		rawCAKey := os.Getenv("PLOY_SERVER_CA_KEY")
		if strings.TrimSpace(rawCACert) == "" || strings.TrimSpace(rawCAKey) == "" {
			http.Error(w, "PKI not configured", http.StatusServiceUnavailable)
			slog.Error("pki sign: CA not configured", "hint", "set PLOY_SERVER_CA_CERT and PLOY_SERVER_CA_KEY")
			return
		}

		ca, err := internalPKI.LoadCA(rawCACert, rawCAKey)
		if err != nil {
			http.Error(w, "failed to load CA", http.StatusInternalServerError)
			slog.Error("pki sign: load CA failed", "err", err)
			return
		}

		// Parse CSR to validate subject common name matches node_id when possible.
		if block, _ := pem.Decode([]byte(req.CSR)); block != nil && block.Type == "CERTIFICATE REQUEST" {
			if parsedCSR, err := x509.ParseCertificateRequest(block.Bytes); err == nil {
				if err := parsedCSR.CheckSignature(); err == nil {
					expectedCN := "node:" + req.NodeID
					if strings.TrimSpace(parsedCSR.Subject.CommonName) != expectedCN {
						http.Error(w, "csr subject common name must match node:<node_id>", http.StatusBadRequest)
						return
					}
				}
			}
			// If parsing/signature fails, fall through to SignNodeCSR for consistent error path.
		}

		// Sign the CSR.
		cert, err := internalPKI.SignNodeCSR(ca, []byte(req.CSR), time.Now())
		if err != nil {
			http.Error(w, fmt.Sprintf("sign failed: %v", err), http.StatusBadRequest)
			slog.Warn("pki sign: sign CSR failed", "node_id", req.NodeID, "err", err)
			return
		}

		// Persist certificate metadata to the database.
		err = st.UpdateNodeCertMetadata(r.Context(), store.UpdateNodeCertMetadataParams{
			ID: pgtype.UUID{
				Bytes: nodeUUID,
				Valid: true,
			},
			CertSerial:      &cert.Serial,
			CertFingerprint: &cert.Fingerprint,
			CertNotBefore: pgtype.Timestamptz{
				Time:  cert.NotBefore,
				Valid: true,
			},
			CertNotAfter: pgtype.Timestamptz{
				Time:  cert.NotAfter,
				Valid: true,
			},
		})
		if err != nil {
			http.Error(w, "failed to persist certificate metadata", http.StatusInternalServerError)
			slog.Error("pki sign: persist metadata failed", "node_id", req.NodeID, "err", err)
			return
		}

		// Build response according to docs/api/components/schemas/pki.yaml.
		resp := struct {
			Certificate string `json:"certificate"`
			CABundle    string `json:"ca_bundle"`
			Serial      string `json:"serial"`
			Fingerprint string `json:"fingerprint"`
			NotBefore   string `json:"not_before"`
			NotAfter    string `json:"not_after"`
		}{
			Certificate: cert.CertPEM,
			CABundle:    rawCACert,
			Serial:      cert.Serial,
			Fingerprint: cert.Fingerprint,
			NotBefore:   cert.NotBefore.Format(time.RFC3339),
			NotAfter:    cert.NotAfter.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("pki sign: encode response failed", "err", err)
		}

		slog.Info("pki sign: certificate issued",
			"node_id", req.NodeID,
			"serial", cert.Serial,
			"fingerprint", cert.Fingerprint,
			"not_before", cert.NotBefore.Format(time.RFC3339),
			"not_after", cert.NotAfter.Format(time.RFC3339),
		)
	}
}

// resolvePgDSN returns the PostgreSQL DSN from environment or config.
// Precedence: PLOY_SERVER_PG_DSN > PLOY_POSTGRES_DSN > config.postgres.dsn
func resolvePgDSN(cfg config.Config) string {
	if dsn := strings.TrimSpace(os.Getenv("PLOY_SERVER_PG_DSN")); dsn != "" {
		return dsn
	}
	if dsn := strings.TrimSpace(os.Getenv("PLOY_POSTGRES_DSN")); dsn != "" {
		return dsn
	}
	return strings.TrimSpace(cfg.Postgres.DSN)
}

// initLogging configures the global slog logger based on the logging config.
func initLogging(cfg config.LoggingConfig) error {
	level := parseLogLevel(cfg.Level)
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var w io.Writer = os.Stderr
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		// Note: file is not closed; it will be closed when the process exits.
		w = f
	}

	var handler slog.Handler
	if cfg.JSON {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	// Add static fields if configured.
	if len(cfg.StaticFields) > 0 {
		attrs := make([]slog.Attr, 0, len(cfg.StaticFields))
		for k, v := range cfg.StaticFields {
			attrs = append(attrs, slog.String(k, v))
		}
		handler = handler.WithAttrs(attrs)
	}

	slog.SetDefault(slog.New(handler))
	return nil
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(levelStr string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// createRepoHandler returns an HTTP handler that creates a new repository.
func createRepoHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.URL) == "" {
			http.Error(w, "url field is required", http.StatusBadRequest)
			return
		}

		// Create the repository.
		repo, err := st.CreateRepo(r.Context(), store.CreateRepoParams{
			Url:       req.URL,
			Branch:    req.Branch,
			CommitSha: req.CommitSha,
		})
		if err != nil {
			// Check if this is a duplicate URL error (UNIQUE constraint violation).
			if strings.Contains(err.Error(), "repos_url_unique") || strings.Contains(err.Error(), "duplicate key") {
				http.Error(w, "repository with this url already exists", http.StatusConflict)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create repository: %v", err), http.StatusInternalServerError)
			slog.Error("create repo: database error", "url", req.URL, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		}{
			ID:        uuid.UUID(repo.ID.Bytes).String(),
			URL:       repo.Url,
			Branch:    repo.Branch,
			CommitSha: repo.CommitSha,
			CreatedAt: repo.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create repo: encode response failed", "err", err)
		}

		slog.Info("repository created",
			"id", resp.ID,
			"url", repo.Url,
		)
	}
}

// listReposHandler returns an HTTP handler that lists all repositories.
func listReposHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// List all repositories.
		repos, err := st.ListRepos(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list repositories: %v", err), http.StatusInternalServerError)
			slog.Error("list repos: database error", "err", err)
			return
		}

		// Build response.
		type repoResponse struct {
			ID        string  `json:"id"`
			URL       string  `json:"url"`
			Branch    *string `json:"branch,omitempty"`
			CommitSha *string `json:"commit_sha,omitempty"`
			CreatedAt string  `json:"created_at"`
		}

		resp := make([]repoResponse, len(repos))
		for i, repo := range repos {
			resp[i] = repoResponse{
				ID:        uuid.UUID(repo.ID.Bytes).String(),
				URL:       repo.Url,
				Branch:    repo.Branch,
				CommitSha: repo.CommitSha,
				CreatedAt: repo.CreatedAt.Time.Format(time.RFC3339),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("list repos: encode response failed", "err", err)
		}

		slog.Debug("repositories listed", "count", len(repos))
	}
}
