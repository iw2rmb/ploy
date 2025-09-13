package acme

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/internal/storage"
)

// CertificateStorage manages certificate storage operations
type CertificateStorage struct {
	consulClient *consulapi.Client
	storage      storage.Storage
	keyPrefix    string
}

// CertificateMetadata represents certificate metadata stored in Consul
type CertificateMetadata struct {
	Domain       string    `json:"domain"`
	CertPath     string    `json:"cert_path"`   // Path in storage system
	KeyPath      string    `json:"key_path"`    // Path in storage system
	IssuerPath   string    `json:"issuer_path"` // Path in storage system
	CertURL      string    `json:"cert_url"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	IsWildcard   bool      `json:"is_wildcard"`
	AutoRenew    bool      `json:"auto_renew"`
	LastRenewal  time.Time `json:"last_renewal,omitempty"`
	RenewalCount int       `json:"renewal_count"`
}

// NewCertificateStorage creates a new certificate storage manager
func NewCertificateStorage(consulClient *consulapi.Client, storage storage.Storage) *CertificateStorage {
	return &CertificateStorage{
		consulClient: consulClient,
		storage:      storage,
		keyPrefix:    "ploy/certificates",
	}
}

// StoreCertificate stores a certificate and its metadata
func (cs *CertificateStorage) StoreCertificate(ctx context.Context, cert *Certificate) error {
	domain := cert.Domain
	log.Printf("Storing certificate for domain: %s", domain)

	// Generate storage paths
	certPath := fmt.Sprintf("certificates/%s/cert.pem", domain)
	keyPath := fmt.Sprintf("certificates/%s/key.pem", domain)
	issuerPath := fmt.Sprintf("certificates/%s/issuer.pem", domain)

	// Store certificate files in storage system
	if err := cs.uploadData(certPath, cert.Certificate); err != nil {
		return fmt.Errorf("failed to store certificate: %w", err)
	}

	if err := cs.uploadData(keyPath, cert.PrivateKey); err != nil {
		return fmt.Errorf("failed to store private key: %w", err)
	}

	if len(cert.IssuerCert) > 0 {
		if err := cs.uploadData(issuerPath, cert.IssuerCert); err != nil {
			return fmt.Errorf("failed to store issuer certificate: %w", err)
		}
	}

	// Create metadata
	metadata := &CertificateMetadata{
		Domain:       domain,
		CertPath:     certPath,
		KeyPath:      keyPath,
		IssuerPath:   issuerPath,
		CertURL:      cert.CertURL,
		IssuedAt:     cert.IssuedAt,
		ExpiresAt:    cert.ExpiresAt,
		IsWildcard:   cert.IsWildcard,
		AutoRenew:    true, // Enable auto-renewal by default
		LastRenewal:  time.Time{},
		RenewalCount: 0,
	}

	// Store metadata in Consul
	if err := cs.storeMetadata(ctx, domain, metadata); err != nil {
		return fmt.Errorf("failed to store certificate metadata: %w", err)
	}

	log.Printf("Certificate stored successfully for domain: %s", domain)
	return nil
}

// GetCertificate retrieves a certificate and its metadata
func (cs *CertificateStorage) GetCertificate(ctx context.Context, domain string) (*Certificate, *CertificateMetadata, error) {
	// Get metadata from Consul
	metadata, err := cs.getMetadata(ctx, domain)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get certificate metadata: %w", err)
	}

	// Download certificate files from storage
	certData, err := cs.downloadData(metadata.CertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download certificate: %w", err)
	}

	keyData, err := cs.downloadData(metadata.KeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download private key: %w", err)
	}

	var issuerData []byte
	if metadata.IssuerPath != "" {
		issuerData, err = cs.downloadData(metadata.IssuerPath)
		if err != nil {
			log.Printf("Warning: failed to download issuer certificate: %v", err)
		}
	}

	// Reconstruct certificate
	cert := &Certificate{
		Domain:      metadata.Domain,
		Certificate: certData,
		PrivateKey:  keyData,
		IssuerCert:  issuerData,
		CertURL:     metadata.CertURL,
		IssuedAt:    metadata.IssuedAt,
		ExpiresAt:   metadata.ExpiresAt,
		IsWildcard:  metadata.IsWildcard,
	}

	return cert, metadata, nil
}

// ListCertificates lists all stored certificates
func (cs *CertificateStorage) ListCertificates(ctx context.Context) ([]*CertificateMetadata, error) {
	kv := cs.consulClient.KV()

	// List all certificate keys
	keys, _, err := kv.Keys(cs.keyPrefix+"/", "/", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list certificate keys: %w", err)
	}

	var certificates []*CertificateMetadata
	for _, key := range keys {
		// Extract domain from key
		domain := filepath.Base(key)

		metadata, err := cs.getMetadata(ctx, domain)
		if err != nil {
			log.Printf("Warning: failed to get metadata for domain %s: %v", domain, err)
			continue
		}

		certificates = append(certificates, metadata)
	}

	return certificates, nil
}

// DeleteCertificate removes a certificate and its metadata
func (cs *CertificateStorage) DeleteCertificate(ctx context.Context, domain string) error {
	log.Printf("Deleting certificate for domain: %s", domain)

	// Get metadata first
	metadata, err := cs.getMetadata(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get certificate metadata: %w", err)
	}

	// Delete files from storage
	if err := cs.deleteData(metadata.CertPath); err != nil {
		log.Printf("Warning: failed to delete certificate file: %v", err)
	}

	if err := cs.deleteData(metadata.KeyPath); err != nil {
		log.Printf("Warning: failed to delete private key file: %v", err)
	}

	if metadata.IssuerPath != "" {
		if err := cs.deleteData(metadata.IssuerPath); err != nil {
			log.Printf("Warning: failed to delete issuer certificate file: %v", err)
		}
	}

	// Delete metadata from Consul
	kv := cs.consulClient.KV()
	key := fmt.Sprintf("%s/%s", cs.keyPrefix, domain)
	_, err = kv.Delete(key, nil)
	if err != nil {
		return fmt.Errorf("failed to delete certificate metadata: %w", err)
	}

	log.Printf("Certificate deleted successfully for domain: %s", domain)
	return nil
}

// UpdateRenewalInfo updates the renewal information for a certificate
func (cs *CertificateStorage) UpdateRenewalInfo(ctx context.Context, domain string, renewed bool) error {
	metadata, err := cs.getMetadata(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get certificate metadata: %w", err)
	}

	if renewed {
		metadata.LastRenewal = time.Now()
		metadata.RenewalCount++
	}

	return cs.storeMetadata(ctx, domain, metadata)
}

// GetExpiringSoon returns certificates that need renewal soon
func (cs *CertificateStorage) GetExpiringSoon(ctx context.Context, threshold time.Duration) ([]*CertificateMetadata, error) {
	allCerts, err := cs.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}

	var expiring []*CertificateMetadata
	now := time.Now()

	for _, cert := range allCerts {
		if cert.AutoRenew && now.Add(threshold).After(cert.ExpiresAt) {
			expiring = append(expiring, cert)
		}
	}

	return expiring, nil
}

// SetAutoRenewal enables or disables auto-renewal for a certificate
func (cs *CertificateStorage) SetAutoRenewal(ctx context.Context, domain string, autoRenew bool) error {
	metadata, err := cs.getMetadata(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get certificate metadata: %w", err)
	}

	metadata.AutoRenew = autoRenew
	return cs.storeMetadata(ctx, domain, metadata)
}

// storeMetadata stores certificate metadata in Consul
func (cs *CertificateStorage) storeMetadata(ctx context.Context, domain string, metadata *CertificateMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	kv := cs.consulClient.KV()
	key := fmt.Sprintf("%s/%s", cs.keyPrefix, domain)

	pair := &consulapi.KVPair{
		Key:   key,
		Value: data,
	}

	_, err = kv.Put(pair, nil)
	if err != nil {
		return fmt.Errorf("failed to store metadata in Consul: %w", err)
	}

	return nil
}

// getMetadata retrieves certificate metadata from Consul
func (cs *CertificateStorage) getMetadata(ctx context.Context, domain string) (*CertificateMetadata, error) {
	kv := cs.consulClient.KV()
	key := fmt.Sprintf("%s/%s", cs.keyPrefix, domain)

	pair, _, err := kv.Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata from Consul: %w", err)
	}

	if pair == nil {
		return nil, fmt.Errorf("certificate not found for domain: %s", domain)
	}

	var metadata CertificateMetadata
	if err := json.Unmarshal(pair.Value, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

// uploadData uploads data to storage using the Storage interface
func (cs *CertificateStorage) uploadData(key string, data []byte) error {
	reader := bytes.NewReader(data)
	ctx := context.Background()
	fullKey := fmt.Sprintf("certificates/%s", key)
	return cs.storage.Put(ctx, fullKey, reader, storage.WithContentType("application/octet-stream"))
}

// downloadData downloads data from storage using the Storage interface
func (cs *CertificateStorage) downloadData(key string) ([]byte, error) {
	ctx := context.Background()
	fullKey := fmt.Sprintf("certificates/%s", key)
	reader, err := cs.storage.Get(ctx, fullKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return io.ReadAll(reader)
}

// deleteData deletes data from storage (placeholder - StorageProvider doesn't have Delete method)
func (cs *CertificateStorage) deleteData(key string) error {
	// Note: StorageProvider interface doesn't have a Delete method
	// In a production implementation, this would need to be added to the interface
	log.Printf("Warning: Cannot delete storage key %s - Delete method not available in StorageProvider", key)
	return nil
}
