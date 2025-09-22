package certificates

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

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
		NotAfter:              time.Now().Add(30 * 24 * time.Hour),
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

func TestStoreRequiresConnectionDetails(t *testing.T) {
	ctx := context.Background()

	_, err := NewStore(ctx, StoreConfig{MetadataBucket: "certs_metadata", BundleBucket: "certs_bundle", EventsStream: "certs_events", RenewedSubject: "certs.renewed"})
	require.Error(t, err)
}

func TestStoreRoundTrip(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()

	cfg := StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_roundtrip",
		RenewedSubject: "certs.roundtrip",
		Replicas:       1,
	}

	store, err := NewStore(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, store)

	domain := "example.dev"
	certPEM, keyPEM := generateCertificatePair(t, domain)

	meta, err := store.Save(ctx, BundleInput{
		Domain:         domain,
		App:            "app-1",
		Provider:       "letsencrypt",
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
		IssuerPEM:      certPEM,
		AutoRenew:      true,
		IssuedAt:       time.Now().Add(-time.Minute).UTC(),
		ExpiresAt:      time.Now().Add(30 * 24 * time.Hour).UTC(),
	})
	require.NoError(t, err)
	require.Equal(t, domain, meta.Domain)
	require.Equal(t, "app-1", meta.App)
	require.NotEmpty(t, meta.Revision)
	require.Contains(t, meta.BundleObject, domain)
	require.NotEmpty(t, meta.FingerprintSHA256)
	require.NotEmpty(t, meta.SerialNumber)

	fetched, err := store.Get(ctx, domain)
	require.NoError(t, err)
	require.Equal(t, meta.BundleObject, fetched.BundleObject)
	require.WithinDuration(t, meta.NotAfter, fetched.NotAfter, time.Second)
	require.True(t, fetched.AutoRenew)

	bundle, err := store.DownloadBundle(ctx, fetched.BundleObject)
	require.NoError(t, err)
	require.NotEmpty(t, bundle)

	tr := tar.NewReader(bytes.NewReader(bundle))
	files := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		files[hdr.Name] = true
	}
	require.Contains(t, files, "cert.pem")
	require.Contains(t, files, "key.pem")
	require.Contains(t, files, "issuer.pem")
	require.Contains(t, files, "metadata.json")
}

func TestStorePublishesRenewalEvent(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()

	cfg := StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_pub",
		RenewedSubject: "certs.renewed.pub",
	}

	store, err := NewStore(ctx, cfg)
	require.NoError(t, err)
	require.NotNil(t, store)

	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)

	js, err := nc.JetStream()
	require.NoError(t, err)

	sub, err := js.PullSubscribe(cfg.RenewedSubject, "test-consumer", nats.BindStream(cfg.EventsStream))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	domain := "pub.dev"
	certPEM, keyPEM := generateCertificatePair(t, domain)

	meta, err := store.Save(ctx, BundleInput{
		Domain:         domain,
		App:            "app-2",
		Provider:       "custom",
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
	})
	require.NoError(t, err)
	require.NotNil(t, meta)

	msgs, err := sub.Fetch(1, nats.MaxWait(2*time.Second))
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	msg := msgs[0]
	require.Equal(t, cfg.RenewedSubject, msg.Subject)
	revision := msg.Header.Get("X-Ploy-Revision")
	require.NotEmpty(t, revision)
	require.Contains(t, string(msg.Data), domain)

	var payload struct {
		Domain   string `json:"domain"`
		Revision string `json:"revision"`
	}
	require.NoError(t, json.Unmarshal(msg.Data, &payload))
	t.Logf("meta revision=%s payload revision=%s", meta.Revision, payload.Revision)
	require.Equal(t, domain, payload.Domain)
	require.Equal(t, meta.Revision, payload.Revision)
}

func TestStoreSetAutoRenew(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()
	store, err := NewStore(ctx, StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_auto",
		RenewedSubject: "certs.auto",
	})
	require.NoError(t, err)

	domain := "toggle.dev"
	certPEM, keyPEM := generateCertificatePair(t, domain)
	_, err = store.Save(ctx, BundleInput{
		Domain:         domain,
		App:            "app-toggle",
		Provider:       "letsencrypt",
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
		AutoRenew:      true,
	})
	require.NoError(t, err)

	require.NoError(t, store.SetAutoRenew(ctx, domain, false))
	meta, err := store.Get(ctx, domain)
	require.NoError(t, err)
	require.False(t, meta.AutoRenew)
}

func TestStoreExpiringSoon(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()
	store, err := NewStore(ctx, StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_expire",
		RenewedSubject: "certs.expire",
	})
	require.NoError(t, err)

	domain1 := "soon.dev"
	cert1, key1 := generateCertificatePair(t, domain1)
	_, err = store.Save(ctx, BundleInput{
		Domain:         domain1,
		App:            "app-expire",
		Provider:       "letsencrypt",
		CertificatePEM: cert1,
		PrivateKeyPEM:  key1,
		AutoRenew:      true,
		ExpiresAt:      time.Now().Add(12 * time.Hour).UTC(),
	})
	require.NoError(t, err)

	domain2 := "later.dev"
	cert2, key2 := generateCertificatePair(t, domain2)
	_, err = store.Save(ctx, BundleInput{
		Domain:         domain2,
		App:            "app-expire",
		Provider:       "letsencrypt",
		CertificatePEM: cert2,
		PrivateKeyPEM:  key2,
		AutoRenew:      true,
		ExpiresAt:      time.Now().Add(72 * time.Hour).UTC(),
	})
	require.NoError(t, err)

	soon, err := store.ExpiringSoon(ctx, 24*time.Hour)
	require.NoError(t, err)
	require.Len(t, soon, 1)
	require.Equal(t, domain1, soon[0].Domain)
}

func TestStoreDelete(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()
	store, err := NewStore(ctx, StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_delete",
		RenewedSubject: "certs.delete",
	})
	require.NoError(t, err)

	domain := "remove.dev"
	certPEM, keyPEM := generateCertificatePair(t, domain)
	_, err = store.Save(ctx, BundleInput{
		Domain:         domain,
		App:            "app-remove",
		Provider:       "letsencrypt",
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
	})
	require.NoError(t, err)

	require.NoError(t, store.Delete(ctx, domain))

	_, err = store.Get(ctx, domain)
	require.Error(t, err)
}

func TestStoreRecordRenewal(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()
	store, err := NewStore(ctx, StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_renew",
		RenewedSubject: "certs.renew",
	})
	require.NoError(t, err)

	domain := "renew.dev"
	certPEM, keyPEM := generateCertificatePair(t, domain)
	_, err = store.Save(ctx, BundleInput{
		Domain:         domain,
		App:            "app-renew",
		Provider:       "letsencrypt",
		CertificatePEM: certPEM,
		PrivateKeyPEM:  keyPEM,
	})
	require.NoError(t, err)

	require.NoError(t, store.RecordRenewal(ctx, domain))
	meta, err := store.Get(ctx, domain)
	require.NoError(t, err)
	require.Equal(t, 1, meta.RenewalCount)
	require.WithinDuration(t, time.Now(), meta.RenewedAt, time.Second)
}

func TestStoreList(t *testing.T) {
	url, shutdown := startTestJetStream(t)
	defer shutdown()

	ctx := context.Background()
	store, err := NewStore(ctx, StoreConfig{
		URL:            url,
		MetadataBucket: "certs_metadata",
		BundleBucket:   "certs_bundle",
		EventsStream:   "certs_events_list",
		RenewedSubject: "certs.list",
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		domain := fmt.Sprintf("list-%d.dev", i)
		cert, key := generateCertificatePair(t, domain)
		_, err := store.Save(ctx, BundleInput{
			Domain:         domain,
			App:            "app-list",
			Provider:       "letsencrypt",
			CertificatePEM: cert,
			PrivateKeyPEM:  key,
		})
		require.NoError(t, err)
	}

	entries, err := store.List(ctx)
	require.NoError(t, err)
	require.Len(t, entries, 3)
}
