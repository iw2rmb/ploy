package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	consulapi "github.com/hashicorp/consul/api"

	certstore "github.com/iw2rmb/ploy/internal/certificates"
)

type config struct {
	ConsulAddr        string
	LegacyPrefix      string
	LegacyAppsPrefix  string
	SeaweedFiler      string
	SeaweedCollection string
	JetStreamURL      string
	JetStreamCreds    string
	JetStreamUser     string
	JetStreamPassword string
	MetadataBucket    string
	BundleBucket      string
	EventsStream      string
	RenewedSubject    string
	Replicas          int
	ChunkSize         int
	DryRun            bool
	ManifestPath      string
	Timeout           time.Duration
}

type kvLister interface {
	List(prefix string, q *consulapi.QueryOptions) (consulapi.KVPairs, *consulapi.QueryMeta, error)
}

type manifest struct {
	GeneratedAt time.Time       `json:"generated_at"`
	DryRun      bool            `json:"dry_run"`
	Domains     []domainSummary `json:"domains"`
}

type domainSummary struct {
	Domain      string `json:"domain"`
	App         string `json:"app,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Status      string `json:"status,omitempty"`
	Migrated    bool   `json:"migrated"`
	DryRun      bool   `json:"dry_run"`
	Error       string `json:"error,omitempty"`
	BundleKey   string `json:"bundle_object,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Fingerprint string `json:"fingerprint_sha256,omitempty"`
}

type legacyMetadata struct {
	Domain       string    `json:"domain"`
	CertPath     string    `json:"cert_path"`
	KeyPath      string    `json:"key_path"`
	IssuerPath   string    `json:"issuer_path"`
	CertURL      string    `json:"cert_url"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	IsWildcard   bool      `json:"is_wildcard"`
	AutoRenew    bool      `json:"auto_renew"`
	LastRenewal  time.Time `json:"last_renewal"`
	RenewalCount int       `json:"renewal_count"`
}

type legacyDomainCertificate struct {
	Domain    string    `json:"domain"`
	AppName   string    `json:"app_name"`
	Status    string    `json:"status"`
	Provider  string    `json:"provider"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	AutoRenew bool      `json:"auto_renew"`
	LastError string    `json:"last_error"`
}

type legacyCertificate struct {
	metadata  legacyMetadata
	record    *legacyDomainCertificate
	certPEM   []byte
	keyPEM    []byte
	issuerPEM []byte
}

func main() {
	cfg := parseFlags()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	if err := run(ctx, cfg); err != nil {
		log.Fatalf("certificate migration failed: %v", err)
	}
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.ConsulAddr, "consul-addr", getenv("CONSUL_HTTP_ADDR", "http://127.0.0.1:8500"), "Consul HTTP address")
	flag.StringVar(&cfg.LegacyPrefix, "legacy-prefix", "ploy/certificates/", "Consul prefix containing certificate metadata")
	flag.StringVar(&cfg.LegacyAppsPrefix, "legacy-apps-prefix", "ploy/certificates/apps/", "Consul prefix containing app certificate records")
	flag.StringVar(&cfg.SeaweedFiler, "seaweed-filer", getenv("SEAWEEDFS_FILER", "http://127.0.0.1:8888"), "SeaweedFS filer base URL")
	flag.StringVar(&cfg.SeaweedCollection, "seaweed-collection", getenv("SEAWEEDFS_COLLECTION", "artifacts"), "SeaweedFS collection name")
	flag.StringVar(&cfg.JetStreamURL, "jetstream-url", getenv("PLOY_CERTS_JETSTREAM_URL", getenv("PLOY_JETSTREAM_URL", "")), "JetStream endpoint for certificates")
	flag.StringVar(&cfg.JetStreamCreds, "jetstream-creds", getenv("PLOY_CERTS_JETSTREAM_CREDS", getenv("PLOY_JETSTREAM_CREDS", "")), "NATS credentials file for JetStream")
	flag.StringVar(&cfg.JetStreamUser, "jetstream-user", getenv("PLOY_CERTS_JETSTREAM_USER", getenv("PLOY_JETSTREAM_USER", "")), "NATS user for JetStream")
	flag.StringVar(&cfg.JetStreamPassword, "jetstream-password", getenv("PLOY_CERTS_JETSTREAM_PASSWORD", getenv("PLOY_JETSTREAM_PASSWORD", "")), "NATS password for JetStream")
	flag.StringVar(&cfg.MetadataBucket, "metadata-bucket", getenv("PLOY_CERTS_METADATA_BUCKET", "certs_metadata"), "JetStream KV bucket for metadata")
	flag.StringVar(&cfg.BundleBucket, "bundle-bucket", getenv("PLOY_CERTS_BUNDLE_BUCKET", "certs_bundle"), "JetStream object store bucket for bundles")
	flag.StringVar(&cfg.EventsStream, "events-stream", getenv("PLOY_CERTS_EVENTS_STREAM", "certs_events"), "JetStream stream for certificate events")
	flag.StringVar(&cfg.RenewedSubject, "renewed-subject", getenv("PLOY_CERTS_RENEWED_SUBJECT", "certs.renewed"), "JetStream subject for renewal notifications")
	flag.IntVar(&cfg.Replicas, "replicas", atoi(getenv("PLOY_CERTS_JETSTREAM_REPLICAS", "3"), 3), "JetStream replication factor")
	flag.IntVar(&cfg.ChunkSize, "chunk-size", atoi(getenv("PLOY_CERTS_OBJECT_CHUNK_SIZE", fmt.Sprint(128*1024)), 128*1024), "Object store chunk size in bytes")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Preview actions without writing to JetStream")
	flag.StringVar(&cfg.ManifestPath, "manifest", "", "Optional path to write migration manifest JSON")
	flag.DurationVar(&cfg.Timeout, "timeout", 5*time.Minute, "Total migration timeout")
	flag.Parse()
	return cfg
}

func run(ctx context.Context, cfg config) error {
	if cfg.JetStreamURL == "" {
		return errors.New("jetstream-url must be provided (PLOY_CERTS_JETSTREAM_URL or flag)")
	}

	consulClient, err := consulapi.NewClient(&consulapi.Config{Address: cfg.ConsulAddr})
	if err != nil {
		return fmt.Errorf("create consul client: %w", err)
	}

	legacy, err := loadLegacyCertificates(ctx, consulClient.KV(), cfg)
	if err != nil {
		return err
	}

	if len(legacy) == 0 {
		log.Printf("no legacy certificates found under %s", cfg.LegacyPrefix)
	}

	store, err := certstore.NewStore(ctx, certstore.StoreConfig{
		URL:            cfg.JetStreamURL,
		UserCreds:      cfg.JetStreamCreds,
		User:           cfg.JetStreamUser,
		Password:       cfg.JetStreamPassword,
		MetadataBucket: cfg.MetadataBucket,
		BundleBucket:   cfg.BundleBucket,
		EventsStream:   cfg.EventsStream,
		RenewedSubject: cfg.RenewedSubject,
		Replicas:       cfg.Replicas,
		ChunkSize:      cfg.ChunkSize,
	})
	if err != nil {
		return fmt.Errorf("initialize certificate store: %w", err)
	}
	defer store.Close()

	summaries := make([]domainSummary, 0, len(legacy))
	for _, item := range legacy {
		summary := migrateCertificate(ctx, store, item, cfg.DryRun)
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Domain < summaries[j].Domain })

	for _, s := range summaries {
		if s.Error != "" {
			log.Printf("[ERROR] %s: %s", s.Domain, s.Error)
			continue
		}
		if cfg.DryRun {
			log.Printf("[DRY-RUN] %s app=%s provider=%s status=%s", s.Domain, s.App, s.Provider, s.Status)
		} else if s.Migrated {
			log.Printf("[MIGRATED] %s app=%s provider=%s revision=%s", s.Domain, s.App, s.Provider, s.Revision)
		} else {
			log.Printf("[SKIP] %s app=%s provider=%s", s.Domain, s.App, s.Provider)
		}
	}

	if cfg.ManifestPath != "" {
		if err := writeManifest(cfg.ManifestPath, manifest{
			GeneratedAt: time.Now().UTC(),
			DryRun:      cfg.DryRun,
			Domains:     summaries,
		}); err != nil {
			return err
		}
		log.Printf("manifest written to %s", cfg.ManifestPath)
	}

	return nil
}

func migrateCertificate(ctx context.Context, store *certstore.Store, item legacyCertificate, dryRun bool) domainSummary {
	record := item.record
	app := ""
	provider := ""
	status := ""
	lastError := ""
	if record != nil {
		app = record.AppName
		provider = record.Provider
		status = record.Status
		lastError = record.LastError
	}
	if provider == "" {
		provider = "letsencrypt"
	}
	if status == "" {
		status = "active"
	}

	summary := domainSummary{
		Domain:   item.metadata.Domain,
		App:      app,
		Provider: provider,
		Status:   status,
		DryRun:   dryRun,
	}

	if dryRun {
		summary.Migrated = false
		return summary
	}

	meta := item.metadata
	bundle := certstore.BundleInput{
		Domain:         meta.Domain,
		App:            app,
		Provider:       provider,
		CertificatePEM: item.certPEM,
		PrivateKeyPEM:  item.keyPEM,
		IssuerPEM:      item.issuerPEM,
		AutoRenew:      meta.AutoRenew,
		IssuedAt:       meta.IssuedAt,
		ExpiresAt:      meta.ExpiresAt,
		Status:         status,
		CertURL:        meta.CertURL,
		LastError:      lastError,
		RenewalCount:   meta.RenewalCount,
	}

	metadata, err := store.Save(ctx, bundle)
	if err != nil {
		summary.Error = fmt.Sprintf("store: %v", err)
		return summary
	}

	summary.Migrated = true
	summary.BundleKey = metadata.BundleObject
	summary.Revision = metadata.Revision
	summary.Fingerprint = metadata.FingerprintSHA256
	return summary
}

func loadLegacyCertificates(ctx context.Context, kv kvLister, cfg config) ([]legacyCertificate, error) {
	prefix := ensureTrailingSlash(cfg.LegacyPrefix)
	metaPairs, _, err := kv.List(prefix, nil)
	if err != nil {
		return nil, fmt.Errorf("list legacy metadata: %w", err)
	}

	metas := make(map[string]legacyMetadata)
	for _, pair := range metaPairs {
		if pair == nil || len(pair.Value) == 0 {
			continue
		}
		key := pair.Key
		if strings.HasPrefix(key, ensureTrailingSlash(cfg.LegacyAppsPrefix)) {
			continue
		}
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		var meta legacyMetadata
		if err := json.Unmarshal(pair.Value, &meta); err != nil {
			log.Printf("[WARN] skipping %s: %v", key, err)
			continue
		}
		if meta.Domain == "" {
			meta.Domain = strings.TrimPrefix(key, prefix)
		}
		metas[meta.Domain] = meta
	}

	appPrefix := ensureTrailingSlash(cfg.LegacyAppsPrefix)
	appPairs, _, err := kv.List(appPrefix, nil)
	if err != nil {
		return nil, fmt.Errorf("list legacy app certificates: %w", err)
	}
	records := make(map[string]*legacyDomainCertificate)
	for _, pair := range appPairs {
		if pair == nil || len(pair.Value) == 0 {
			continue
		}
		var record legacyDomainCertificate
		if err := json.Unmarshal(pair.Value, &record); err != nil {
			log.Printf("[WARN] skipping app record %s: %v", pair.Key, err)
			continue
		}
		if record.Domain != "" {
			records[record.Domain] = &record
		}
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	var result []legacyCertificate
	for domain, meta := range metas {
		certPEM, err := fetchSeaweed(ctx, httpClient, cfg.SeaweedFiler, cfg.SeaweedCollection, meta.CertPath)
		if err != nil {
			log.Printf("[ERROR] fetch cert for %s: %v", domain, err)
			result = append(result, legacyCertificate{metadata: meta, record: records[domain], certPEM: nil, keyPEM: nil, issuerPEM: nil})
			continue
		}
		keyPEM, err := fetchSeaweed(ctx, httpClient, cfg.SeaweedFiler, cfg.SeaweedCollection, meta.KeyPath)
		if err != nil {
			log.Printf("[ERROR] fetch key for %s: %v", domain, err)
			result = append(result, legacyCertificate{metadata: meta, record: records[domain], certPEM: nil, keyPEM: nil, issuerPEM: nil})
			continue
		}
		var issuerPEM []byte
		if strings.TrimSpace(meta.IssuerPath) != "" {
			issuerPEM, err = fetchSeaweed(ctx, httpClient, cfg.SeaweedFiler, cfg.SeaweedCollection, meta.IssuerPath)
			if err != nil {
				log.Printf("[WARN] issuer fetch failed for %s: %v", domain, err)
			}
		}
		result = append(result, legacyCertificate{
			metadata:  meta,
			record:    records[domain],
			certPEM:   certPEM,
			keyPEM:    keyPEM,
			issuerPEM: issuerPEM,
		})
	}

	sort.Slice(result, func(i, j int) bool { return result[i].metadata.Domain < result[j].metadata.Domain })
	return result, nil
}

func fetchSeaweed(ctx context.Context, client *http.Client, filer, collection, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("empty key")
	}
	cleanKey := strings.TrimPrefix(key, "/")
	url := fmt.Sprintf("%s/%s/%s", strings.TrimSuffix(filer, "/"), strings.TrimPrefix(collection, "/"), cleanKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("fetch %s failed: %s %s", key, resp.Status, string(body))
	}
	return io.ReadAll(resp.Body)
}

func writeManifest(path string, m manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

func getenv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func atoi(value string, fallback int) int {
	if v, err := strconv.Atoi(value); err == nil {
		return v
	}
	return fallback
}

func ensureTrailingSlash(prefix string) string {
	if prefix == "" {
		return ""
	}
	if !strings.HasSuffix(prefix, "/") {
		return prefix + "/"
	}
	return prefix
}
