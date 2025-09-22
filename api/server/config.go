package server

import (
	"os"
	"strconv"
	"time"

	recipecatalog "github.com/iw2rmb/ploy/internal/recipes/catalog"
	"github.com/iw2rmb/ploy/internal/utils"
)

// JetStreamEnvConfig captures dual-write configuration for the environment store.
type JetStreamEnvConfig struct {
	DualWrite       bool
	URL             string
	Bucket          string
	CredentialsPath string
	User            string
	Password        string
}

// JetStreamRoutingConfig captures routing persistence configuration.
type JetStreamRoutingConfig struct {
	Enabled         bool
	URL             string
	Bucket          string
	Stream          string
	SubjectPrefix   string
	CredentialsPath string
	User            string
	Password        string
	ChunkSize       int
	Replicas        int
}

// JetStreamCertificatesConfig captures certificate persistence configuration.
type JetStreamCertificatesConfig struct {
	Enabled         bool
	URL             string
	MetadataBucket  string
	BundleBucket    string
	EventsStream    string
	RenewedSubject  string
	CredentialsPath string
	User            string
	Password        string
	ChunkSize       int
	Replicas        int
}

// ControllerConfig holds configuration for controller initialization
type ControllerConfig struct {
	Port              string
	ConsulAddr        string
	NomadAddr         string
	StorageConfigPath string
	CleanupConfigPath string
	UseConsulEnv      bool
	EnvStorePath      string
	CleanupAutoStart  bool
	ShutdownTimeout   time.Duration
	EnableCaching     bool
	// Legacy security default packs (e.g., "rewrite-java:8.1.0,rewrite-spring:5.0.0"). If set, indexer runs at startup.
	SecurityDefaultPacks string
	// Optional fetcher for the security indexer (used in tests). When nil, indexing is skipped.
	SecurityFetcher recipecatalog.Fetcher
	// Optional registry URL for HTTPFetcher. Used only if SecurityFetcher is nil.
	SecurityRegistryURL string
	// Optional Maven group for MavenFetcher. If set, MavenFetcher is used.
	SecurityMavenGroup string
	// JetStream dual write configuration for the env store.
	JetStreamEnv JetStreamEnvConfig
	// JetStream routing persistence/event configuration.
	JetStreamRouting JetStreamRoutingConfig
	// JetStream certificate metadata/bundle configuration.
	JetStreamCertificates JetStreamCertificatesConfig
}

// parseIntEnv parses integer from environment variable with fallback
func parseIntEnv(envVar string, defaultVal int) int {
	if val := os.Getenv(envVar); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

func parseBoolEnv(envVar string, defaultVal bool) bool {
	if val := os.Getenv(envVar); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return defaultVal
}

// LoadConfigFromEnv loads controller configuration from environment variables
func LoadConfigFromEnv() *ControllerConfig {
	// Prioritize NOMAD_PORT_http for dynamic port allocation, fall back to PORT, then default
	port := os.Getenv("NOMAD_PORT_http")
	if port == "" {
		port = utils.Getenv("PORT", "8081")
	}

	dualWrite := parseBoolEnv("PLOY_ENVSTORE_JETSTREAM_DUAL_WRITE", false)
	if !dualWrite {
		dualWrite = parseBoolEnv("PLOY_USE_JETSTREAM_KV", false)
	}

	jsURL := utils.Getenv("PLOY_JETSTREAM_URL", "")
	jsBucket := utils.Getenv("PLOY_JETSTREAM_ENV_BUCKET", "")
	if jsBucket == "" {
		jsBucket = utils.Getenv("PLOY_JETSTREAM_KV_BUCKET", "ploy_env")
	}

	jsCreds := utils.Getenv("PLOY_JETSTREAM_CREDS", "")
	jsUser := utils.Getenv("PLOY_JETSTREAM_USER", "")
	jsPassword := utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")

	routingEnabled := parseBoolEnv("PLOY_ROUTING_JETSTREAM_ENABLED", false)
	routingURL := utils.Getenv("PLOY_ROUTING_JETSTREAM_URL", jsURL)
	routingBucket := utils.Getenv("PLOY_ROUTING_OBJECT_BUCKET", "routing_maps")
	routingStream := utils.Getenv("PLOY_ROUTING_EVENT_STREAM", "routing_events")
	routingSubject := utils.Getenv("PLOY_ROUTING_EVENT_SUBJECT_PREFIX", "routing.app")
	routingCreds := utils.Getenv("PLOY_ROUTING_JETSTREAM_CREDS", jsCreds)
	routingUser := utils.Getenv("PLOY_ROUTING_JETSTREAM_USER", jsUser)
	routingPassword := utils.Getenv("PLOY_ROUTING_JETSTREAM_PASSWORD", jsPassword)
	routingChunkSize := parseIntEnv("PLOY_ROUTING_OBJECT_CHUNK_SIZE", 128*1024)
	routingReplicas := parseIntEnv("PLOY_ROUTING_JETSTREAM_REPLICAS", 3)
	if routingURL != "" && !routingEnabled {
		routingEnabled = true
	}

	certsEnabled := parseBoolEnv("PLOY_CERTS_JETSTREAM_ENABLED", true)
	certsURL := utils.Getenv("PLOY_CERTS_JETSTREAM_URL", jsURL)
	certsMetadataBucket := utils.Getenv("PLOY_CERTS_METADATA_BUCKET", "certs_metadata")
	certsBundleBucket := utils.Getenv("PLOY_CERTS_BUNDLE_BUCKET", "certs_bundle")
	certsEventsStream := utils.Getenv("PLOY_CERTS_EVENTS_STREAM", "certs_events")
	certsRenewedSubject := utils.Getenv("PLOY_CERTS_RENEWED_SUBJECT", "certs.renewed")
	certsCreds := utils.Getenv("PLOY_CERTS_JETSTREAM_CREDS", jsCreds)
	certsUser := utils.Getenv("PLOY_CERTS_JETSTREAM_USER", jsUser)
	certsPassword := utils.Getenv("PLOY_CERTS_JETSTREAM_PASSWORD", jsPassword)
	certsChunkSize := parseIntEnv("PLOY_CERTS_OBJECT_CHUNK_SIZE", 128*1024)
	certsReplicas := parseIntEnv("PLOY_CERTS_JETSTREAM_REPLICAS", 3)
	if certsURL != "" && !certsEnabled {
		certsEnabled = true
	}

	return &ControllerConfig{
		Port:                 port,
		ConsulAddr:           utils.Getenv("CONSUL_HTTP_ADDR", "127.0.0.1:8500"),
		NomadAddr:            utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
		StorageConfigPath:    getStorageConfigPath(),
		CleanupConfigPath:    utils.Getenv("PLOY_CLEANUP_CONFIG", ""),
		UseConsulEnv:         utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true",
		EnvStorePath:         utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"),
		CleanupAutoStart:     utils.Getenv("PLOY_CLEANUP_AUTO_START", "true") == "true",
		ShutdownTimeout:      30 * time.Second, // Graceful shutdown timeout
		EnableCaching:        utils.Getenv("PLOY_ENABLE_CACHING", "true") == "true",
		SecurityDefaultPacks: utils.Getenv("PLOY_SECURITY_DEFAULT_PACKS", ""),
		SecurityRegistryURL:  utils.Getenv("PLOY_SECURITY_REGISTRY", "https://registry.dev.ployman.app"),
		SecurityMavenGroup:   utils.Getenv("PLOY_SECURITY_MAVEN_GROUP", ""),
		JetStreamEnv: JetStreamEnvConfig{
			DualWrite:       dualWrite,
			URL:             jsURL,
			Bucket:          jsBucket,
			CredentialsPath: jsCreds,
			User:            jsUser,
			Password:        jsPassword,
		},
		JetStreamRouting: JetStreamRoutingConfig{
			Enabled:         routingEnabled,
			URL:             routingURL,
			Bucket:          routingBucket,
			Stream:          routingStream,
			SubjectPrefix:   routingSubject,
			CredentialsPath: routingCreds,
			User:            routingUser,
			Password:        routingPassword,
			ChunkSize:       routingChunkSize,
			Replicas:        routingReplicas,
		},
		JetStreamCertificates: JetStreamCertificatesConfig{
			Enabled:         certsEnabled,
			URL:             certsURL,
			MetadataBucket:  certsMetadataBucket,
			BundleBucket:    certsBundleBucket,
			EventsStream:    certsEventsStream,
			RenewedSubject:  certsRenewedSubject,
			CredentialsPath: certsCreds,
			User:            certsUser,
			Password:        certsPassword,
			ChunkSize:       certsChunkSize,
			Replicas:        certsReplicas,
		},
	}
}

// getStorageConfigPath resolves the storage configuration path without depending on api/config.
// Order: env override -> common external paths -> embedded default.
func getStorageConfigPath() string {
	if path := os.Getenv("PLOY_STORAGE_CONFIG"); path != "" {
		return path
	}
	externalPaths := []string{
		"/etc/ploy/storage/config.yaml",
		"/etc/ploy/config.yaml",
	}
	for _, p := range externalPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "configs/storage-config.yaml"
}
