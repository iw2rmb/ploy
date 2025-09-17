package seaweedfs

import (
	"log"
	"net/http"
	"strings"
)

// New creates a new SeaweedFS storage provider
func New(cfg Config) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	timeout := cfg.TimeoutDuration()
	collection := cfg.Collection
	if collection == "" {
		collection = "artifacts"
	}

	replication := cfg.Replication
	if replication == "" {
		replication = "000" // no replication for dev environment
	}

	provider := &Provider{
		masterURL:   ensureHTTPScheme(cfg.Master),
		filerURL:    ensureHTTPScheme(cfg.Filer),
		collection:  collection,
		replication: replication,
		timeout:     timeout,
		httpClient:  &http.Client{Timeout: timeout},
	}

	log.Printf("[SeaweedFS Provider] Initialized with Master: %s, Filer: %s, Collection: %s", provider.masterURL, provider.filerURL, provider.collection)
	return provider, nil
}

func ensureHTTPScheme(addr string) string {
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		return "http://" + addr
	}
	return addr
}
