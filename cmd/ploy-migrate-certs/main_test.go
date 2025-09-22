package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/stretchr/testify/require"

	consulapi "github.com/hashicorp/consul/api"

	certstore "github.com/iw2rmb/ploy/internal/certificates"
)

type stubKV struct {
	entries map[string][]byte
}

func (s *stubKV) List(prefix string, _ *consulapi.QueryOptions) (consulapi.KVPairs, *consulapi.QueryMeta, error) {
	var keys []string
	for key := range s.entries {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	pairs := make(consulapi.KVPairs, 0, len(keys))
	for _, key := range keys {
		val := s.entries[key]
		copyVal := make([]byte, len(val))
		copy(copyVal, val)
		pairs = append(pairs, &consulapi.KVPair{Key: key, Value: copyVal})
	}
	return pairs, nil, nil
}

func startTestJetStream(t *testing.T) (string, func()) {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	srv := natstest.RunServer(&opts)
	cleanup := func() {
		srv.Shutdown()
	}
	return srv.ClientURL(), cleanup
}

func newTestStore(t *testing.T, url string) *certstore.Store {
	t.Helper()
	ctx := context.Background()
	store, err := certstore.NewStore(ctx, certstore.StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events",
		RenewedSubject: "certs.renewed",
	})
	require.NoError(t, err)
	return store
}

func generateCertificatePair(t *testing.T, domain string) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   domain,
			Organization: []string{"Ploy Test"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour),
		DNSNames:              []string{domain},
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return certPEM, keyPEM
}

func TestLoadLegacyCertificates(t *testing.T) {
	t.Run("loads metadata and fetches pem", func(t *testing.T) {
		meta := legacyMetadata{
			Domain:       "example.dev",
			CertPath:     "certificates/example.dev/cert.pem",
			KeyPath:      "certificates/example.dev/key.pem",
			IssuerPath:   "certificates/example.dev/issuer.pem",
			CertURL:      "https://acme.example/cert",
			IssuedAt:     time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt:    time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			AutoRenew:    true,
			RenewalCount: 2,
		}
		metaJSON, err := json.Marshal(meta)
		require.NoError(t, err)

		record := legacyDomainCertificate{
			Domain:    "example.dev",
			AppName:   "demo",
			Status:    "active",
			Provider:  "letsencrypt",
			AutoRenew: true,
		}
		recordJSON, err := json.Marshal(record)
		require.NoError(t, err)

		kv := &stubKV{entries: map[string][]byte{
			"ploy/certificates/example.dev":           metaJSON,
			"ploy/certificates/apps/demo/example.dev": recordJSON,
		}}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/artifacts/certificates/example.dev/cert.pem":
				_, _ = w.Write([]byte("CERT"))
			case "/artifacts/certificates/example.dev/key.pem":
				_, _ = w.Write([]byte("KEY"))
			case "/artifacts/certificates/example.dev/issuer.pem":
				_, _ = w.Write([]byte("ISSUER"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		cfg := config{
			LegacyPrefix:      "ploy/certificates/",
			LegacyAppsPrefix:  "ploy/certificates/apps/",
			SeaweedFiler:      server.URL,
			SeaweedCollection: "artifacts",
		}

		certs, err := loadLegacyCertificates(context.Background(), kv, cfg)
		require.NoError(t, err)
		require.Len(t, certs, 1)

		entry := certs[0]
		require.Equal(t, "example.dev", entry.metadata.Domain)
		require.NotNil(t, entry.record)
		require.Equal(t, "demo", entry.record.AppName)
		require.Equal(t, []byte("CERT"), entry.certPEM)
		require.Equal(t, []byte("KEY"), entry.keyPEM)
		require.Equal(t, []byte("ISSUER"), entry.issuerPEM)
	})
}

func TestMigrateCertificate(t *testing.T) {
	ctx := context.Background()
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	store := newTestStore(t, url)
	t.Cleanup(store.Close)

	item := legacyCertificate{
		metadata: legacyMetadata{
			Domain:       "example.dev",
			CertURL:      "https://acme.example/cert",
			IssuedAt:     time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt:    time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC),
			AutoRenew:    true,
			RenewalCount: 1,
		},
		record: &legacyDomainCertificate{
			Domain:   "example.dev",
			AppName:  "demo",
			Status:   "active",
			Provider: "letsencrypt",
		},
	}
	certPEM, keyPEM := generateCertificatePair(t, "example.dev")
	item.certPEM = certPEM
	item.keyPEM = keyPEM
	item.issuerPEM = certPEM

	summary := migrateCertificate(ctx, store, item, false)
	t.Logf("summary: %+v", summary)
	require.True(t, summary.Migrated)
	require.Equal(t, "demo", summary.App)
	require.NotEmpty(t, summary.Revision)
	require.Empty(t, summary.Error)

	stored, err := store.Get(ctx, "example.dev")
	require.NoError(t, err)
	require.Equal(t, "demo", stored.App)
	require.Equal(t, "letsencrypt", stored.Provider)
	require.Equal(t, true, stored.AutoRenew)
}

func TestMigrateCertificateDryRun(t *testing.T) {
	ctx := context.Background()
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	store := newTestStore(t, url)
	t.Cleanup(store.Close)

	item := legacyCertificate{
		metadata: legacyMetadata{Domain: "example.dev"},
	}

	summary := migrateCertificate(ctx, store, item, true)
	require.False(t, summary.Migrated)
	require.Empty(t, summary.Error)

	_, err := store.Get(ctx, "example.dev")
	require.Error(t, err)
}
