package server

import (
	"os"
	"strconv"
	"time"

	trecipes "github.com/iw2rmb/ploy/internal/arf/recipes"
	"github.com/iw2rmb/ploy/internal/utils"
)

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
	// Legacy remediation default packs (e.g., "rewrite-java:8.1.0,rewrite-spring:5.0.0"). If set, indexer runs at startup.
	RemediationDefaultPacks string
	// Optional fetcher for the remediation indexer (used in tests). When nil, indexing is skipped.
	RemediationFetcher trecipes.Fetcher
	// Optional registry URL for HTTPFetcher. Used only if RemediationFetcher is nil.
	RemediationRegistryURL string
	// Optional Maven group for MavenFetcher. If set, MavenFetcher is used.
	RemediationMavenGroup string
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

// LoadConfigFromEnv loads controller configuration from environment variables
func LoadConfigFromEnv() *ControllerConfig {
	// Prioritize NOMAD_PORT_http for dynamic port allocation, fall back to PORT, then default
	port := os.Getenv("NOMAD_PORT_http")
	if port == "" {
		port = utils.Getenv("PORT", "8081")
	}

	return &ControllerConfig{
		Port:                    port,
		ConsulAddr:              utils.Getenv("CONSUL_HTTP_ADDR", "127.0.0.1:8500"),
		NomadAddr:               utils.Getenv("NOMAD_ADDR", "http://127.0.0.1:4646"),
		StorageConfigPath:       getStorageConfigPath(),
		CleanupConfigPath:       utils.Getenv("PLOY_CLEANUP_CONFIG", ""),
		UseConsulEnv:            utils.Getenv("PLOY_USE_CONSUL_ENV", "true") == "true",
		EnvStorePath:            utils.Getenv("PLOY_ENV_STORE_PATH", "/tmp/ploy-env-store"),
		CleanupAutoStart:        utils.Getenv("PLOY_CLEANUP_AUTO_START", "true") == "true",
		ShutdownTimeout:         30 * time.Second, // Graceful shutdown timeout
		EnableCaching:           utils.Getenv("PLOY_ENABLE_CACHING", "true") == "true",
		RemediationDefaultPacks: utils.Getenv("PLOY_ARF_DEFAULT_PACKS", ""),
		RemediationRegistryURL:  utils.Getenv("PLOY_ARF_REGISTRY", "https://registry.dev.ployman.app"),
		RemediationMavenGroup:   utils.Getenv("PLOY_ARF_MAVEN_GROUP", ""),
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
