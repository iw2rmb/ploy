package certificates

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	nats "github.com/nats-io/nats.go"
)

const (
	defaultChunkSize = 128 * 1024
)

// StoreConfig describes the JetStream resources required for certificate storage.
type StoreConfig struct {
	Conn           *nats.Conn
	URL            string
	Credentials    nats.Option
	UserCreds      string
	User           string
	Password       string
	MetadataBucket string
	BundleBucket   string
	EventsStream   string
	RenewedSubject string
	ChunkSize      int
	Replicas       int
}

// BundleInput represents the payload required to persist a certificate bundle.
type BundleInput struct {
	Domain         string
	App            string
	Provider       string
	CertificatePEM []byte
	PrivateKeyPEM  []byte
	IssuerPEM      []byte
	AutoRenew      bool
	IssuedAt       time.Time
	ExpiresAt      time.Time
	Status         string
	CertURL        string
	LastError      string
	RenewalCount   int
}

// Metadata captures certificate metadata stored in JetStream.
type Metadata struct {
	Domain            string    `json:"domain"`
	App               string    `json:"app"`
	Provider          string    `json:"provider"`
	Status            string    `json:"status"`
	CertURL           string    `json:"cert_url,omitempty"`
	LastError         string    `json:"last_error,omitempty"`
	BundleObject      string    `json:"bundle_object"`
	BundleDigest      string    `json:"bundle_digest"`
	NotBefore         time.Time `json:"not_before"`
	NotAfter          time.Time `json:"not_after"`
	IssuedAt          time.Time `json:"issued_at"`
	RenewedAt         time.Time `json:"renewed_at,omitempty"`
	AutoRenew         bool      `json:"auto_renew"`
	FingerprintSHA256 string    `json:"fingerprint_sha256"`
	SerialNumber      string    `json:"serial_number"`
	Revision          string    `json:"revision"`
	RenewalCount      int       `json:"renewal_count"`
}

// Store coordinates certificate persistence in JetStream.
type Store struct {
	conn           *nats.Conn
	ownsConn       bool
	js             nats.JetStreamContext
	metadata       nats.KeyValue
	bundles        nats.ObjectStore
	chunkSize      int
	replicas       int
	eventsStream   string
	renewedSubject string
	mu             sync.Mutex
}

// NewStore provisions JetStream resources for certificate storage.
func NewStore(ctx context.Context, cfg StoreConfig) (*Store, error) {
	if cfg.MetadataBucket == "" {
		return nil, fmt.Errorf("metadata bucket is required")
	}
	if cfg.BundleBucket == "" {
		return nil, fmt.Errorf("bundle bucket is required")
	}
	if cfg.EventsStream == "" {
		return nil, fmt.Errorf("events stream is required")
	}
	if cfg.RenewedSubject == "" {
		return nil, fmt.Errorf("renewal subject is required")
	}

	store := &Store{
		chunkSize:      cfg.ChunkSize,
		replicas:       cfg.Replicas,
		eventsStream:   cfg.EventsStream,
		renewedSubject: cfg.RenewedSubject,
	}
	if store.chunkSize <= 0 {
		store.chunkSize = defaultChunkSize
	}
	if store.replicas <= 0 {
		store.replicas = 1
	}

	conn := cfg.Conn
	if conn == nil {
		if cfg.URL == "" {
			return nil, fmt.Errorf("jetstream URL or connection required")
		}

		opts := []nats.Option{nats.Name("ploy-certificates-store")}
		if cfg.Credentials != nil {
			opts = append(opts, cfg.Credentials)
		}
		if cfg.UserCreds != "" {
			opts = append(opts, nats.UserCredentials(cfg.UserCreds))
		}
		if cfg.User != "" {
			opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
		}

		c, err := nats.Connect(cfg.URL, opts...)
		if err != nil {
			return nil, fmt.Errorf("connect to jetstream: %w", err)
		}
		conn = c
		store.ownsConn = true
	}
	store.conn = conn

	js, err := conn.JetStream(nats.Context(ctx))
	if err != nil {
		store.close()
		return nil, fmt.Errorf("jetstream context: %w", err)
	}
	store.js = js

	if err := store.ensureMetadataBucket(cfg.MetadataBucket); err != nil {
		store.close()
		return nil, err
	}
	if err := store.ensureBundleBucket(cfg.BundleBucket); err != nil {
		store.close()
		return nil, err
	}
	if err := store.ensureEventsStream(); err != nil {
		store.close()
		return nil, err
	}

	return store, nil
}

func (s *Store) close() {
	if s.ownsConn && s.conn != nil {
		s.conn.Close()
	}
}

func (s *Store) ensureMetadataBucket(bucket string) error {
	kv, err := s.js.KeyValue(bucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		cfg := &nats.KeyValueConfig{
			Bucket:   bucket,
			History:  5,
			Storage:  nats.FileStorage,
			Replicas: s.replicas,
		}
		kv, err = s.js.CreateKeyValue(cfg)
	}
	if err != nil {
		return fmt.Errorf("ensure certificate metadata bucket: %w", err)
	}
	s.metadata = kv
	return nil
}

func (s *Store) ensureBundleBucket(bucket string) error {
	store, err := s.js.ObjectStore(bucket)
	if errors.Is(err, nats.ErrStreamNotFound) || errors.Is(err, nats.ErrObjectNotFound) {
		cfg := &nats.ObjectStoreConfig{
			Bucket:   bucket,
			Storage:  nats.FileStorage,
			Replicas: s.replicas,
		}
		store, err = s.js.CreateObjectStore(cfg)
	}
	if err != nil {
		return fmt.Errorf("ensure certificate bundle bucket: %w", err)
	}
	s.bundles = store
	return nil
}

func (s *Store) ensureEventsStream() error {
	info, err := s.js.StreamInfo(s.eventsStream)
	if err == nil {
		if !subjectInList(info.Config.Subjects, s.renewedSubject) {
			return fmt.Errorf("events stream %s exists without subject %s", s.eventsStream, s.renewedSubject)
		}
		return nil
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("lookup events stream: %w", err)
	}

	cfg := &nats.StreamConfig{
		Name:       s.eventsStream,
		Subjects:   []string{s.renewedSubject},
		Retention:  nats.LimitsPolicy,
		Duplicates: time.Hour,
		Storage:    nats.FileStorage,
		Replicas:   s.replicas,
	}

	if _, err := s.js.AddStream(cfg); err != nil {
		return fmt.Errorf("create events stream: %w", err)
	}
	return nil
}

func subjectInList(subjects []string, target string) bool {
	for _, subj := range subjects {
		if subj == target {
			return true
		}
	}
	return false
}

// Save stores certificate material and emits a renewal event.
func (s *Store) Save(ctx context.Context, input BundleInput) (*Metadata, error) {
	if err := validateBundleInput(input); err != nil {
		return nil, err
	}

	domainKey := metadataKey(input.Domain)

	s.mu.Lock()
	defer s.mu.Unlock()

	bundleName := bundleObjectName(input.Domain)
	info, err := s.writeBundle(ctx, bundleName, input)
	if err != nil {
		return nil, err
	}

	fingerprint, serial, err := certificateDetails(input.CertificatePEM)
	if err != nil {
		return nil, err
	}

	issuedAt := coalesceTime(input.IssuedAt, time.Now().UTC())
	notBefore := issuedAt
	notAfter := coalesceTime(input.ExpiresAt, issuedAt.Add(90*24*time.Hour))
	status := emptyFallback(input.Status, "active")

	metadata := &Metadata{
		Domain:            input.Domain,
		App:               input.App,
		Provider:          emptyFallback(input.Provider, "unknown"),
		Status:            status,
		CertURL:           input.CertURL,
		LastError:         input.LastError,
		BundleObject:      info.Name,
		BundleDigest:      digestValue(info.Digest),
		NotBefore:         notBefore,
		NotAfter:          notAfter,
		IssuedAt:          issuedAt,
		RenewedAt:         time.Now().UTC(),
		AutoRenew:         input.AutoRenew,
		FingerprintSHA256: fingerprint,
		SerialNumber:      serial,
		RenewalCount:      input.RenewalCount,
	}

	payload, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}

	rev, err := s.metadata.Put(domainKey, payload)
	if err != nil {
		return nil, fmt.Errorf("store metadata: %w", err)
	}
	metadata.Revision = fmt.Sprintf("%d", rev)

	if err := s.publishRenewed(ctx, metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

func validateBundleInput(input BundleInput) error {
	if strings.TrimSpace(input.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if len(input.CertificatePEM) == 0 {
		return fmt.Errorf("certificate PEM is required")
	}
	if len(input.PrivateKeyPEM) == 0 {
		return fmt.Errorf("private key PEM is required")
	}
	return nil
}

func metadataKey(domain string) string {
	return fmt.Sprintf("domains/%s", domain)
}

func bundleObjectName(domain string) string {
	timestamp := time.Now().UTC().Format("20060102T150405Z0700")
	sanitized := strings.ReplaceAll(domain, "*", "wildcard")
	sanitized = strings.ReplaceAll(sanitized, "..", ".")
	sanitized = strings.TrimPrefix(sanitized, ".")
	return fmt.Sprintf("domains/%s/%s.tar", sanitized, timestamp)
}

func (s *Store) writeBundle(ctx context.Context, name string, input BundleInput) (*nats.ObjectInfo, error) {
	pr, pw := io.Pipe()

	done := make(chan error, 1)
	go func() {
		defer close(done)
		writer := tar.NewWriter(pw)
		if err := writeTarFile(writer, "cert.pem", input.CertificatePEM); err != nil {
			done <- err
			_ = pw.CloseWithError(err)
			return
		}
		if err := writeTarFile(writer, "key.pem", input.PrivateKeyPEM); err != nil {
			done <- err
			_ = pw.CloseWithError(err)
			return
		}
		issuer := input.IssuerPEM
		if issuer == nil {
			issuer = []byte("")
		}
		if err := writeTarFile(writer, "issuer.pem", issuer); err != nil {
			done <- err
			_ = pw.CloseWithError(err)
			return
		}

		meta := map[string]any{
			"domain":    input.Domain,
			"app":       input.App,
			"provider":  input.Provider,
			"stored_at": time.Now().UTC().Format(time.RFC3339Nano),
		}
		metaPayload, err := json.Marshal(meta)
		if err != nil {
			done <- err
			_ = pw.CloseWithError(err)
			return
		}
		if err := writeTarFile(writer, "metadata.json", metaPayload); err != nil {
			done <- err
			_ = pw.CloseWithError(err)
			return
		}
		if err := writer.Close(); err != nil {
			done <- err
			_ = pw.CloseWithError(err)
			return
		}
		done <- pw.Close()
	}()

	meta := &nats.ObjectMeta{
		Name: name,
		Metadata: map[string]string{
			"domain": input.Domain,
			"app":    input.App,
		},
		Opts: &nats.ObjectMetaOptions{ChunkSize: uint32(s.chunkSize)},
	}

	info, err := s.bundles.Put(meta, pr, nats.Context(ctx))
	if err != nil {
		return nil, fmt.Errorf("store bundle: %w", err)
	}
	if pipeErr := <-done; pipeErr != nil {
		return nil, fmt.Errorf("write bundle tar: %w", pipeErr)
	}
	return info, nil
}

func writeTarFile(tw *tar.Writer, name string, data []byte) error {
	header := &tar.Header{
		Name:    name,
		Mode:    0600,
		Size:    int64(len(data)),
		ModTime: time.Now(),
		Format:  tar.FormatPAX,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	if len(data) > 0 {
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func certificateDetails(certPEM []byte) (string, string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", "", fmt.Errorf("decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("parse certificate: %w", err)
	}
	fingerprint := sha256.Sum256(cert.Raw)
	serial := cert.SerialNumber.Text(16)
	return strings.ToLower(hex.EncodeToString(fingerprint[:])), strings.ToUpper(serial), nil
}

func coalesceTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback
	}
	return value.UTC()
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func digestValue(raw string) string {
	return strings.TrimPrefix(raw, "SHA-256=")
}

// Get returns stored metadata for a domain.
func (s *Store) Get(ctx context.Context, domain string) (*Metadata, error) {
	if s == nil {
		return nil, fmt.Errorf("certificate store is nil")
	}
	entry, err := s.metadata.Get(metadataKey(domain))
	if errors.Is(err, nats.ErrKeyNotFound) {
		return nil, fmt.Errorf("certificate metadata not found for domain %s", domain)
	}
	if err != nil {
		return nil, fmt.Errorf("get metadata: %w", err)
	}

	meta := &Metadata{}
	if err := json.Unmarshal(entry.Value(), meta); err != nil {
		return nil, fmt.Errorf("decode metadata: %w", err)
	}
	if meta.Revision == "" {
		meta.Revision = fmt.Sprintf("%d", entry.Revision())
	}
	return meta, nil
}

// DownloadBundle retrieves the stored bundle as a tar archive byte slice.
func (s *Store) DownloadBundle(ctx context.Context, object string) ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("certificate store is nil")
	}
	result, err := s.bundles.Get(object, nats.Context(ctx))
	if errors.Is(err, nats.ErrObjectNotFound) {
		return nil, fmt.Errorf("bundle %s not found", object)
	}
	if err != nil {
		return nil, fmt.Errorf("get bundle: %w", err)
	}
	defer func() { _ = result.Close() }()

	buf := bytes.NewBuffer(nil)
	if _, err := io.Copy(buf, result); err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	return buf.Bytes(), nil
}

func (s *Store) publishRenewed(ctx context.Context, meta *Metadata) error {
	event := map[string]any{
		"domain":     meta.Domain,
		"app":        meta.App,
		"provider":   meta.Provider,
		"revision":   meta.Revision,
		"bundle":     meta.BundleObject,
		"not_after":  meta.NotAfter.Format(time.RFC3339),
		"auto_renew": meta.AutoRenew,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode renewal event: %w", err)
	}

	msg := &nats.Msg{
		Subject: s.renewedSubject,
		Header:  nats.Header{},
		Data:    payload,
	}
	msg.Header.Set("X-Ploy-Revision", meta.Revision)
	msg.Header.Set("X-Ploy-Bundle", meta.BundleObject)
	msg.Header.Set("X-Ploy-Digest", meta.BundleDigest)

	if _, err := s.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
		return fmt.Errorf("publish renewal: %w", err)
	}
	return nil
}

// Close terminates the underlying connection if owned by the store.
func (s *Store) Close() {
	s.close()
}

// SetAutoRenew updates the auto-renewal flag for the provided domain.
func (s *Store) SetAutoRenew(ctx context.Context, domain string, auto bool) error {
	return s.updateMetadata(ctx, domain, func(meta *Metadata) {
		meta.AutoRenew = auto
	})
}

// ExpiringSoon lists certificates expiring within the provided threshold.
func (s *Store) ExpiringSoon(ctx context.Context, threshold time.Duration) ([]*Metadata, error) {
	if s == nil {
		return nil, fmt.Errorf("certificate store is nil")
	}
	keys, err := s.metadata.Keys()
	if errors.Is(err, nats.ErrNoKeysFound) {
		return []*Metadata{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list metadata keys: %w", err)
	}

	now := time.Now().UTC()
	var results []*Metadata
	seen := make(map[string]struct{})
	for _, key := range keys {
		if !strings.HasPrefix(key, "domains/") {
			continue
		}
		domain := domainFromKey(key)
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		meta, err := s.Get(ctx, domain)
		if err != nil {
			continue
		}
		if !meta.AutoRenew {
			continue
		}
		if meta.NotAfter.IsZero() {
			continue
		}
		if now.Add(threshold).After(meta.NotAfter) {
			results = append(results, meta)
		}
	}
	return results, nil
}

// List returns all stored certificate metadata entries.
func (s *Store) List(ctx context.Context) ([]*Metadata, error) {
	if s == nil {
		return nil, fmt.Errorf("certificate store is nil")
	}
	keys, err := s.metadata.Keys()
	if errors.Is(err, nats.ErrNoKeysFound) {
		return []*Metadata{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list metadata keys: %w", err)
	}

	var results []*Metadata
	seen := make(map[string]struct{})
	for _, key := range keys {
		if !strings.HasPrefix(key, "domains/") {
			continue
		}
		domain := domainFromKey(key)
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		meta, err := s.Get(ctx, domain)
		if err != nil {
			continue
		}
		results = append(results, meta)
	}
	return results, nil
}

// Delete removes metadata and associated bundle for the provided domain.
func (s *Store) Delete(ctx context.Context, domain string) error {
	if s == nil {
		return fmt.Errorf("certificate store is nil")
	}
	meta, err := s.Get(ctx, domain)
	if err != nil {
		return err
	}

	if meta.BundleObject != "" {
		if err := s.bundles.Delete(meta.BundleObject); err != nil && !errors.Is(err, nats.ErrObjectNotFound) {
			return fmt.Errorf("delete bundle: %w", err)
		}
	}

	if err := s.metadata.Delete(metadataKey(domain)); err != nil {
		return fmt.Errorf("delete metadata: %w", err)
	}
	return nil
}

// RecordRenewal increments renewal counters and timestamps for a domain.
func (s *Store) RecordRenewal(ctx context.Context, domain string) error {
	return s.updateMetadata(ctx, domain, func(meta *Metadata) {
		meta.RenewalCount++
	})
}

func (s *Store) updateMetadata(ctx context.Context, domain string, mutate func(*Metadata)) error {
	if s == nil {
		return fmt.Errorf("certificate store is nil")
	}
	entry, err := s.metadata.Get(metadataKey(domain))
	if errors.Is(err, nats.ErrKeyNotFound) {
		return fmt.Errorf("certificate metadata not found for domain %s", domain)
	}
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}

	meta := &Metadata{}
	if err := json.Unmarshal(entry.Value(), meta); err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}
	mutate(meta)
	meta.RenewedAt = time.Now().UTC()
	payload, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	if _, err := s.metadata.Update(metadataKey(domain), payload, entry.Revision()); err != nil {
		return fmt.Errorf("update metadata: %w", err)
	}
	return nil
}

func domainFromKey(key string) string {
	return strings.TrimPrefix(key, "domains/")
}
