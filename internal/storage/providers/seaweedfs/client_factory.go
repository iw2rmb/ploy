package seaweedfs

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"
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

	// Tuned HTTP transport for high-concurrency uploads with good connection reuse
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 60 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          512,
		MaxIdleConnsPerHost:   256,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{Timeout: timeout, Transport: transport}

	provider := &Provider{
		masterURL:   ensureHTTPScheme(cfg.Master),
		filerURL:    ensureHTTPScheme(cfg.Filer),
		collection:  collection,
		replication: replication,
		timeout:     timeout,
		httpClient:  httpClient,
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
