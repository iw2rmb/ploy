package acme

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	certstore "github.com/iw2rmb/ploy/internal/certificates"
)

// CertificateStorage manages JetStream-backed certificate persistence.
type CertificateStorage struct {
	store *certstore.Store
}

// StoreOptions controls how certificates are persisted.
type StoreOptions struct {
	App          string
	Provider     string
	AutoRenew    bool
	Status       string
	CertURL      string
	LastError    string
	RenewalCount int
}

// NewCertificateStorage creates a JetStream-backed certificate storage manager.
func NewCertificateStorage(store *certstore.Store) *CertificateStorage {
	return &CertificateStorage{store: store}
}

// StoreCertificate persists certificate material and metadata, returning the resulting JetStream metadata entry.
func (cs *CertificateStorage) StoreCertificate(ctx context.Context, cert *Certificate, opts StoreOptions) (*certstore.Metadata, error) {
	if cs == nil || cs.store == nil {
		return nil, fmt.Errorf("certificate store not configured")
	}
	if cert == nil {
		return nil, fmt.Errorf("certificate cannot be nil")
	}
	input := certstore.BundleInput{
		Domain:         cert.Domain,
		App:            opts.App,
		Provider:       opts.Provider,
		CertificatePEM: cert.Certificate,
		PrivateKeyPEM:  cert.PrivateKey,
		IssuerPEM:      cert.IssuerCert,
		AutoRenew:      opts.AutoRenew,
		IssuedAt:       cert.IssuedAt,
		ExpiresAt:      cert.ExpiresAt,
		Status:         opts.Status,
		CertURL:        opts.CertURL,
		LastError:      opts.LastError,
		RenewalCount:   opts.RenewalCount,
	}
	return cs.store.Save(ctx, input)
}

// GetCertificate retrieves certificate material and metadata for a domain.
func (cs *CertificateStorage) GetCertificate(ctx context.Context, domain string) (*Certificate, *certstore.Metadata, error) {
	if cs == nil || cs.store == nil {
		return nil, nil, fmt.Errorf("certificate store not configured")
	}
	meta, err := cs.store.Get(ctx, domain)
	if err != nil {
		return nil, nil, err
	}
	bundle, err := cs.store.DownloadBundle(ctx, meta.BundleObject)
	if err != nil {
		return nil, nil, err
	}
	files, err := untarBundle(bundle)
	if err != nil {
		return nil, nil, err
	}

	certificate := &Certificate{
		Domain:      meta.Domain,
		Certificate: files["cert.pem"],
		PrivateKey:  files["key.pem"],
		IssuerCert:  files["issuer.pem"],
		CertURL:     meta.CertURL,
		IssuedAt:    meta.IssuedAt,
		ExpiresAt:   meta.NotAfter,
		IsWildcard:  isWildcardDomain(meta.Domain),
	}

	return certificate, meta, nil
}

// ListCertificates returns all stored certificate metadata entries.
func (cs *CertificateStorage) ListCertificates(ctx context.Context) ([]*certstore.Metadata, error) {
	if cs == nil || cs.store == nil {
		return nil, fmt.Errorf("certificate store not configured")
	}
	return cs.store.List(ctx)
}

// DeleteCertificate removes certificate metadata and bundle for a domain.
func (cs *CertificateStorage) DeleteCertificate(ctx context.Context, domain string) error {
	if cs == nil || cs.store == nil {
		return fmt.Errorf("certificate store not configured")
	}
	return cs.store.Delete(ctx, domain)
}

// UpdateRenewalInfo increments renewal counters for the domain.
func (cs *CertificateStorage) UpdateRenewalInfo(ctx context.Context, domain string, renewed bool) error {
	if cs == nil || cs.store == nil {
		return fmt.Errorf("certificate store not configured")
	}
	if !renewed {
		return nil
	}
	return cs.store.RecordRenewal(ctx, domain)
}

// GetExpiringSoon returns certificates that expire within the provided threshold.
func (cs *CertificateStorage) GetExpiringSoon(ctx context.Context, threshold time.Duration) ([]*certstore.Metadata, error) {
	if cs == nil || cs.store == nil {
		return nil, fmt.Errorf("certificate store not configured")
	}
	return cs.store.ExpiringSoon(ctx, threshold)
}

// SetAutoRenewal toggles auto-renewal for the domain.
func (cs *CertificateStorage) SetAutoRenewal(ctx context.Context, domain string, autoRenew bool) error {
	if cs == nil || cs.store == nil {
		return fmt.Errorf("certificate store not configured")
	}
	return cs.store.SetAutoRenew(ctx, domain, autoRenew)
}

func untarBundle(data []byte) (map[string][]byte, error) {
	files := make(map[string][]byte)
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		buf := bytes.NewBuffer(nil)
		if _, err := io.Copy(buf, reader); err != nil {
			return nil, fmt.Errorf("copy tar entry: %w", err)
		}
		files[header.Name] = buf.Bytes()
	}
	return files, nil
}

func isWildcardDomain(domain string) bool {
	return len(domain) > 0 && domain[0] == '*'
}
