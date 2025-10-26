package etcdutil

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var defaultEndpoints = []string{"http://127.0.0.1:2379"}

// LocalEndpoints returns a copy of the default localhost etcd endpoints.
func LocalEndpoints() []string {
	out := make([]string, len(defaultEndpoints))
	copy(out, defaultEndpoints)
	return out
}

// ConfigFromEnv builds an etcd client configuration honoring the standard env vars.
func ConfigFromEnv() (clientv3.Config, error) {
	cfg := clientv3.Config{
		Endpoints:   LocalEndpoints(),
		DialTimeout: 5 * time.Second,
	}
	if env := strings.TrimSpace(os.Getenv("PLOY_ETCD_ENDPOINTS")); env != "" {
		parts := strings.Split(env, ",")
		endpoints := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				endpoints = append(endpoints, trimmed)
			}
		}
		if len(endpoints) > 0 {
			cfg.Endpoints = endpoints
		}
	}
	if user := strings.TrimSpace(os.Getenv("PLOY_ETCD_USERNAME")); user != "" {
		cfg.Username = user
		cfg.Password = os.Getenv("PLOY_ETCD_PASSWORD")
	}
	tlsCfg, err := BuildTLSConfigFromEnv()
	if err != nil {
		return cfg, err
	}
	if tlsCfg != nil {
		cfg.TLS = tlsCfg
	}
	return cfg, nil
}

// BuildTLSConfigFromEnv constructs a TLS config for etcd connections based on env vars.
func BuildTLSConfigFromEnv() (*tls.Config, error) {
	caPath := strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_CA"))
	certPath := strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_CERT"))
	keyPath := strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_KEY"))
	skipVerify := strings.EqualFold(strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_SKIP_VERIFY")), "true") ||
		strings.TrimSpace(os.Getenv("PLOY_ETCD_TLS_SKIP_VERIFY")) == "1"

	if caPath == "" && certPath == "" && keyPath == "" && !skipVerify {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: skipVerify, // #nosec G402 — allow operator override for debugging.
	}

	if caPath != "" {
		data, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("load etcd ca: %w", err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(data); !ok {
			return nil, errors.New("control-plane: parse etcd ca")
		}
		tlsCfg.RootCAs = pool
	}

	if certPath != "" || keyPath != "" {
		if certPath == "" || keyPath == "" {
			return nil, errors.New("control-plane: both etcd client cert and key required")
		}
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("control-plane: load etcd client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}
