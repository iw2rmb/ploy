package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// resolveObjectStoreConfig returns the ObjectStoreConfig from environment or config.
// Environment variables take precedence over config file values.
func resolveObjectStoreConfig(cfg config.Config) config.ObjectStoreConfig {
	result := cfg.ObjectStore

	if v := strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_ENDPOINT")); v != "" {
		result.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_BUCKET")); v != "" {
		result.Bucket = v
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_ACCESS_KEY")); v != "" {
		result.AccessKey = v
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_SECRET_KEY")); v != "" {
		result.SecretKey = v
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_SECURE")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			result.Secure = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_REGION")); v != "" {
		result.Region = v
	}

	return result
}
