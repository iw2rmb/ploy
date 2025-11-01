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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

	// Register mods endpoints (control plane).
	httpSrv.HandleFunc("POST /v1/mods/crud", createModHandler(st), auth.RoleControlPlane)
	httpSrv.HandleFunc("GET /v1/mods/crud", listModsHandler(st), auth.RoleControlPlane)

    // Register runs endpoints (control plane).
    httpSrv.HandleFunc("POST /v1/runs", createRunHandler(st), auth.RoleControlPlane)
    // Support both query (?id=) and RESTful path (/v1/runs/{id}) for basic run view.
    httpSrv.HandleFunc("GET /v1/runs", getRunHandler(st), auth.RoleControlPlane)
    httpSrv.HandleFunc("GET /v1/runs/{id}", getRunHandler(st), auth.RoleControlPlane)
    httpSrv.HandleFunc("DELETE /v1/runs/{id}", deleteRunHandler(st), auth.RoleControlPlane)

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
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
				http.Error(w, "repository with this url already exists", http.StatusConflict)
				return
			}
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

		wrapper := struct {
			Repos []repoResponse `json:"repos"`
		}{
			Repos: make([]repoResponse, len(repos)),
		}

		for i, repo := range repos {
			wrapper.Repos[i] = repoResponse{
				ID:        uuid.UUID(repo.ID.Bytes).String(),
				URL:       repo.Url,
				Branch:    repo.Branch,
				CommitSha: repo.CommitSha,
				CreatedAt: repo.CreatedAt.Time.Format(time.RFC3339),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(wrapper); err != nil {
			slog.Error("list repos: encode response failed", "err", err)
		}

		slog.Debug("repositories listed", "count", len(repos))
	}
}

// createModHandler returns an HTTP handler that creates a new mod.
func createModHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.RepoID) == "" {
			http.Error(w, "repo_id field is required", http.StatusBadRequest)
			return
		}

		// Validate repo_id format.
		repoUUID, err := uuid.Parse(req.RepoID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid repo_id: %v", err), http.StatusBadRequest)
			return
		}

		// Validate spec is not empty.
		if len(req.Spec) == 0 {
			http.Error(w, "spec field is required", http.StatusBadRequest)
			return
		}

		// Validate spec is valid JSON.
		if !json.Valid(req.Spec) {
			http.Error(w, "spec must be valid JSON", http.StatusBadRequest)
			return
		}

		// Create the mod.
		mod, err := st.CreateMod(r.Context(), store.CreateModParams{
			RepoID: pgtype.UUID{
				Bytes: repoUUID,
				Valid: true,
			},
			Spec:      req.Spec,
			CreatedBy: req.CreatedBy,
		})
		if err != nil {
			// Check if this is a foreign key violation (repo does not exist).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
				http.Error(w, "repository not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create mod: %v", err), http.StatusInternalServerError)
			slog.Error("create mod: database error", "repo_id", req.RepoID, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		}{
			ID:        uuid.UUID(mod.ID.Bytes).String(),
			RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
			Spec:      mod.Spec,
			CreatedBy: mod.CreatedBy,
			CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create mod: encode response failed", "err", err)
		}

		slog.Info("mod created",
			"id", resp.ID,
			"repo_id", resp.RepoID,
		)
	}
}

// listModsHandler returns an HTTP handler that lists mods, optionally filtered by repo_id.
func listModsHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for repo_id query parameter.
		repoIDStr := strings.TrimSpace(r.URL.Query().Get("repo_id"))

		type modResponse struct {
			ID        string          `json:"id"`
			RepoID    string          `json:"repo_id"`
			Spec      json.RawMessage `json:"spec"`
			CreatedBy *string         `json:"created_by,omitempty"`
			CreatedAt string          `json:"created_at"`
		}

		wrapper := struct {
			Mods []modResponse `json:"mods"`
		}{}

		if repoIDStr != "" {
			// Parse and validate repo_id.
			repoUUID, err := uuid.Parse(repoIDStr)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid repo_id: %v", err), http.StatusBadRequest)
				return
			}

			// List mods for the specified repository.
			mods, err := st.ListModsByRepo(r.Context(), pgtype.UUID{
				Bytes: repoUUID,
				Valid: true,
			})
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
				slog.Error("list mods: database error", "repo_id", repoIDStr, "err", err)
				return
			}

			wrapper.Mods = make([]modResponse, len(mods))
			for i, mod := range mods {
				wrapper.Mods[i] = modResponse{
					ID:        uuid.UUID(mod.ID.Bytes).String(),
					RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
					Spec:      mod.Spec,
					CreatedBy: mod.CreatedBy,
					CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
				}
			}

			slog.Debug("mods listed by repo", "repo_id", repoIDStr, "count", len(mods))
		} else {
			// List all mods.
			mods, err := st.ListMods(r.Context())
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to list mods: %v", err), http.StatusInternalServerError)
				slog.Error("list mods: database error", "err", err)
				return
			}

			wrapper.Mods = make([]modResponse, len(mods))
			for i, mod := range mods {
				wrapper.Mods[i] = modResponse{
					ID:        uuid.UUID(mod.ID.Bytes).String(),
					RepoID:    uuid.UUID(mod.RepoID.Bytes).String(),
					Spec:      mod.Spec,
					CreatedBy: mod.CreatedBy,
					CreatedAt: mod.CreatedAt.Time.Format(time.RFC3339),
				}
			}

			slog.Debug("mods listed", "count", len(mods))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(wrapper); err != nil {
			slog.Error("list mods: encode response failed", "err", err)
		}
	}
}

// createRunHandler returns an HTTP handler that creates a new run.
func createRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Decode request body.
		var req struct {
			ModID     string  `json:"mod_id"`
			BaseRef   string  `json:"base_ref"`
			TargetRef string  `json:"target_ref"`
			CommitSha *string `json:"commit_sha,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields.
		if strings.TrimSpace(req.ModID) == "" {
			http.Error(w, "mod_id field is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.BaseRef) == "" {
			http.Error(w, "base_ref field is required", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.TargetRef) == "" {
			http.Error(w, "target_ref field is required", http.StatusBadRequest)
			return
		}

		// Validate mod_id format.
		modUUID, err := uuid.Parse(req.ModID)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid mod_id: %v", err), http.StatusBadRequest)
			return
		}

		// Create the run with status=queued.
		run, err := st.CreateRun(r.Context(), store.CreateRunParams{
			ModID: pgtype.UUID{
				Bytes: modUUID,
				Valid: true,
			},
			Status:    store.RunStatusQueued,
			BaseRef:   req.BaseRef,
			TargetRef: req.TargetRef,
			CommitSha: req.CommitSha,
		})
		if err != nil {
			// Check if this is a foreign key violation (mod does not exist).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
				http.Error(w, "mod not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to create run: %v", err), http.StatusInternalServerError)
			slog.Error("create run: database error", "mod_id", req.ModID, "err", err)
			return
		}

		// Build response with run_id.
		resp := struct {
			RunID string `json:"run_id"`
		}{
			RunID: uuid.UUID(run.ID.Bytes).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("create run: encode response failed", "err", err)
		}

		slog.Info("run created",
			"run_id", resp.RunID,
			"mod_id", req.ModID,
			"status", "queued",
		)
	}
}

// getRunHandler returns an HTTP handler that retrieves a run by id query parameter.
// Supports view=timing query parameter to retrieve timing data from runs_timing view.
func getRunHandler(st store.Store) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Check if view=timing is requested.
        view := strings.TrimSpace(r.URL.Query().Get("view"))
        if view == "timing" {
            getRunTimingHandler(st).ServeHTTP(w, r)
            return
        }

        // Accept id from path parameter first, then fallback to query parameter.
        runIDStr := strings.TrimSpace(r.PathValue("id"))
        if runIDStr == "" {
            runIDStr = strings.TrimSpace(r.URL.Query().Get("id"))
        }
        if runIDStr == "" {
            http.Error(w, "id query parameter is required", http.StatusBadRequest)
            return
        }

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Get the run from the database.
		run, err := st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run: %v", err), http.StatusInternalServerError)
			slog.Error("get run: database error", "run_id", runIDStr, "err", err)
			return
		}

		// Build response.
        resp := struct {
            ID         string          `json:"id"`
            ModID      string          `json:"mod_id"`
            Status     string          `json:"status"`
            Reason     *string         `json:"reason,omitempty"`
            CreatedAt  string          `json:"created_at"`
            StartedAt  *string         `json:"started_at,omitempty"`
            FinishedAt *string         `json:"finished_at,omitempty"`
            NodeID     *string         `json:"node_id,omitempty"`
            BaseRef    string          `json:"base_ref"`
            TargetRef  string          `json:"target_ref"`
            CommitSha  *string         `json:"commit_sha,omitempty"`
            Stats      json.RawMessage `json:"stats,omitempty"`
        }{
            ID:        uuid.UUID(run.ID.Bytes).String(),
            ModID:     uuid.UUID(run.ModID.Bytes).String(),
            Status:    string(run.Status),
            Reason:    run.Reason,
            CreatedAt: run.CreatedAt.Time.Format(time.RFC3339),
            BaseRef:   run.BaseRef,
            TargetRef: run.TargetRef,
            CommitSha: run.CommitSha,
        }

		// Handle optional timestamp fields.
		if run.StartedAt.Valid {
			startedAt := run.StartedAt.Time.Format(time.RFC3339)
			resp.StartedAt = &startedAt
		}
		if run.FinishedAt.Valid {
			finishedAt := run.FinishedAt.Time.Format(time.RFC3339)
			resp.FinishedAt = &finishedAt
		}

		// Handle optional node_id.
		if run.NodeID.Valid {
			nodeID := uuid.UUID(run.NodeID.Bytes).String()
			resp.NodeID = &nodeID
		}

        // Handle stats (JSONB): return as raw JSON if not empty object.
        if len(run.Stats) > 0 && string(run.Stats) != "{}" {
            resp.Stats = json.RawMessage(run.Stats)
        }

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get run: encode response failed", "err", err)
		}

		slog.Info("run retrieved", "run_id", resp.ID)
	}
}

// getRunTimingHandler returns an HTTP handler that retrieves timing data for a run.
func getRunTimingHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Accept id from path parameter first, then fallback to query parameter.
		runIDStr := strings.TrimSpace(r.PathValue("id"))
		if runIDStr == "" {
			runIDStr = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if runIDStr == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Get timing data from the database.
		timing, err := st.GetRunTiming(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to get run timing: %v", err), http.StatusInternalServerError)
			slog.Error("get run timing: database error", "run_id", runIDStr, "err", err)
			return
		}

		// Build response.
		resp := struct {
			ID      string `json:"id"`
			QueueMs int64  `json:"queue_ms"`
			RunMs   int64  `json:"run_ms"`
		}{
			ID:      uuid.UUID(timing.ID.Bytes).String(),
			QueueMs: timing.QueueMs,
			RunMs:   timing.RunMs,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("get run timing: encode response failed", "err", err)
		}

		slog.Info("run timing retrieved", "run_id", resp.ID, "queue_ms", resp.QueueMs, "run_ms", resp.RunMs)
	}
}

// deleteRunHandler returns an HTTP handler that deletes a run by id.
func deleteRunHandler(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract id from path parameter.
		runIDStr := r.PathValue("id")
		if strings.TrimSpace(runIDStr) == "" {
			http.Error(w, "id path parameter is required", http.StatusBadRequest)
			return
		}

		// Parse and validate run_id.
		runUUID, err := uuid.Parse(runIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid id: %v", err), http.StatusBadRequest)
			return
		}

		// Check if the run exists before attempting to delete.
		_, err = st.GetRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			http.Error(w, fmt.Sprintf("failed to check run: %v", err), http.StatusInternalServerError)
			slog.Error("delete run: check failed", "run_id", runIDStr, "err", err)
			return
		}

		// Delete the run.
		err = st.DeleteRun(r.Context(), pgtype.UUID{
			Bytes: runUUID,
			Valid: true,
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to delete run: %v", err), http.StatusInternalServerError)
			slog.Error("delete run: database error", "run_id", runIDStr, "err", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		slog.Info("run deleted", "run_id", runIDStr)
	}
}
