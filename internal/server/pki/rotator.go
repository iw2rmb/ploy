package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// DefaultRotator performs simple on-disk certificate rotation for the server/node
// certificate described by config.PKIConfig. It renews when the current certificate
// is within the RenewBefore window. If the cluster CA material is available via
// PLOY_SERVER_CA_CERT and PLOY_SERVER_CA_KEY, it reissues a certificate with the
// same subject and SANs using the existing private key. Otherwise, it logs a
// structured warning so operators can rotate out-of-band.
type DefaultRotator struct {
	logger *slog.Logger
}

// NewDefaultRotator creates a new DefaultRotator instance.
func NewDefaultRotator(logger *slog.Logger) *DefaultRotator {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultRotator{logger: logger}
}

// Renew checks the active certificate and, if needed, reissues it using the
// cluster CA from environment variables. It preserves subject, SANs, and reuses
// the existing private key.
func (r *DefaultRotator) Renew(ctx context.Context, cfg config.PKIConfig) error {
	_ = ctx
	// Load current certificate
	certPEM, err := os.ReadFile(strings.TrimSpace(cfg.Certificate))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Nothing to renew yet.
			return nil
		}
		return fmt.Errorf("pki: read cert: %w", err)
	}
	cert, err := parseCert(certPEM)
	if err != nil {
		return fmt.Errorf("pki: parse cert: %w", err)
	}
	// If not within renewal window, exit quietly.
	renewBefore := cfg.RenewBefore
	if renewBefore <= 0 {
		renewBefore = time.Hour
	}
	until := time.Until(cert.NotAfter)
	if until > renewBefore {
		return nil
	}

	// Attempt self-renew using cluster CA from env, else log a warning.
	caCertPEM := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_CERT"))
	caKeyPEM := strings.TrimSpace(os.Getenv("PLOY_SERVER_CA_KEY"))
	if caCertPEM == "" || caKeyPEM == "" {
		r.logger.Warn("pki renewal window reached; CA not configured in env, skipping self-issue",
			"expires_in", until.String(), "cert", cfg.Certificate)
		return nil
	}

	// Parse CA bundle
	caCert, caKey, err := parseCA([]byte(caCertPEM), []byte(caKeyPEM))
	if err != nil {
		return fmt.Errorf("pki: parse CA: %w", err)
	}

	// Load existing private key
	keyPEM, err := os.ReadFile(strings.TrimSpace(cfg.Key))
	if err != nil {
		return fmt.Errorf("pki: read key: %w", err)
	}
	priv, err := parseECPrivateKey(keyPEM)
	if err != nil {
		return fmt.Errorf("pki: parse key: %w", err)
	}

	// Build a new certificate template preserving identity
	now := time.Now().UTC()
	validity := cert.NotAfter.Sub(cert.NotBefore)
	if validity <= 0 {
		validity = 365 * 24 * time.Hour
	}
	serial, _ := rand.Int(rand.Reader, newBigInt())
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               cert.Subject,
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(validity),
		DNSNames:              cert.DNSNames,
		IPAddresses:           cert.IPAddresses,
		URIs:                  cert.URIs,
		KeyUsage:              cert.KeyUsage,
		ExtKeyUsage:           cert.ExtKeyUsage,
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("pki: create certificate: %w", err)
	}
	newCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(cfg.Certificate, newCertPEM, 0o644); err != nil {
		return fmt.Errorf("pki: write cert: %w", err)
	}
	r.logger.Info("pki certificate renewed",
		"cert", cfg.Certificate,
		"not_after", now.Add(validity).Format(time.RFC3339))
	return nil
}

func parseCert(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("invalid CERTIFICATE PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func parseECPrivateKey(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("invalid key PEM")
	}
	// Try EC first
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	// Try PKCS8
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := keyAny.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("unsupported private key type (want ECDSA)")
	}
	return ec, nil
}

func parseCA(certPEM, keyPEM []byte) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	cert, err := parseCert(certPEM)
	if err != nil {
		return nil, nil, err
	}
	// Require CA basic constraint
	if !cert.IsCA {
		return nil, nil, errors.New("provided CA cert is not a CA")
	}
	key, err := parseECPrivateKey(keyPEM)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

// hostFromURL best-effort extracts host or IP from a URL for SAN preservation.
func hostFromURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return ""
	}
	h := parsed.Hostname()
	if ip := net.ParseIP(h); ip != nil {
		return ip.String()
	}
	return h
}

// newBigInt returns a large max for random serial generation (~128-bit).
func newBigInt() *big.Int {
	// 2^128 - 1
	var (
		one = new(big.Int).SetInt64(1)
		b   = new(big.Int).Lsh(one, 128)
	)
	return b.Sub(b, one)
}
