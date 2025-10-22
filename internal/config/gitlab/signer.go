package gitlab

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	// SecretsPrefix stores encrypted GitLab API keys used by the signer.
	SecretsPrefix = "config/gitlab/signer/secrets/"

	defaultSignerTTL    = 15 * time.Minute
	defaultSignerMaxTTL = 12 * time.Hour
)

var (
	errEmptySecretName = errors.New("gitlab signer: secret name required")
	errEmptyAPIKey     = errors.New("gitlab signer: api key required")
)

// Cipher encrypts and decrypts bytes using the control-plane KMS.
type Cipher interface {
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}

// AESCipher implements Cipher using AES-GCM with a static key.
type AESCipher struct {
	key []byte
}

// NewAESCipher returns an AES-GCM cipher backed by the provided key.
func NewAESCipher(key []byte) (*AESCipher, error) {
	length := len(key)
	if length != 16 && length != 24 && length != 32 {
		return nil, fmt.Errorf("gitlab signer: aes key must be 16, 24, or 32 bytes")
	}
	copied := make([]byte, length)
	copy(copied, key)
	return &AESCipher{key: copied}, nil
}

// Encrypt applies AES-GCM to the plaintext and returns nonce+ciphertext.
func (a *AESCipher) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	data := gcm.Seal(nonce, nonce, plaintext, nil)
	return data, nil
}

// Decrypt reverses the AES-GCM encryption applied by Encrypt.
func (a *AESCipher) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(a.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("gitlab signer: ciphertext too short")
	}
	nonce := ciphertext[:nonceSize]
	data := ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// Signer issues short-lived GitLab tokens backed by encrypted API keys in etcd.
type Signer struct {
	client     *clientv3.Client
	cipher     Cipher
	prefix     string
	defaultTTL time.Duration
	maxTTL     time.Duration
	now        func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	watchersMu sync.RWMutex
	watchers   map[int64]chan RotationEvent
	watcherSeq atomic.Int64
}

// SignerOption customises signer configuration.
type SignerOption func(*signerConfig)

type signerConfig struct {
	prefix     string
	defaultTTL time.Duration
	maxTTL     time.Duration
	now        func() time.Time
}

// WithPrefix overrides the etcd prefix used to store secrets.
func WithPrefix(prefix string) SignerOption {
	return func(cfg *signerConfig) {
		cfg.prefix = strings.TrimSpace(prefix)
	}
}

// WithDefaultTTL overrides the default token TTL when not supplied by callers.
func WithDefaultTTL(ttl time.Duration) SignerOption {
	return func(cfg *signerConfig) {
		if ttl > 0 {
			cfg.defaultTTL = ttl
		}
	}
}

// WithMaxTTL overrides the maximum TTL allowed for issued tokens.
func WithMaxTTL(ttl time.Duration) SignerOption {
	return func(cfg *signerConfig) {
		if ttl > 0 {
			cfg.maxTTL = ttl
		}
	}
}

// WithNow injects a custom clock for testing.
func WithNow(now func() time.Time) SignerOption {
	return func(cfg *signerConfig) {
		if now != nil {
			cfg.now = now
		}
	}
}

// RotationEvent captures a secret rotation observed via etcd.
type RotationEvent struct {
	SecretName string
	Revision   int64
	UpdatedAt  time.Time
}

// RotationSubscription delivers rotation events to subscribers.
type RotationSubscription struct {
	C <-chan RotationEvent

	closeOnce sync.Once
	closeFn   func()
}

// Close terminates the subscription and releases resources.
func (s *RotationSubscription) Close() {
	s.closeOnce.Do(func() {
		if s.closeFn != nil {
			s.closeFn()
		}
	})
}

// RotateSecretRequest stores or rotates a GitLab API key.
type RotateSecretRequest struct {
	SecretName string
	APIKey     string
	Scopes     []string
}

// RotateSecretResult returns revision metadata for persisted secrets.
type RotateSecretResult struct {
	Revision  int64
	UpdatedAt time.Time
}

// IssueTokenRequest specifies parameters for a short-lived token.
type IssueTokenRequest struct {
	SecretName string
	Scopes     []string
	TTL        time.Duration
}

// SignedToken captures the issued token and metadata.
type SignedToken struct {
	SecretName string
	Value      string
	Scopes     []string
	IssuedAt   time.Time
	ExpiresAt  time.Time
}

// NewSigner constructs a GitLab token signer.
func NewSigner(client *clientv3.Client, cipher Cipher, opts ...SignerOption) (*Signer, error) {
	if client == nil {
		return nil, errors.New("gitlab signer: etcd client required")
	}
	if cipher == nil {
		return nil, errors.New("gitlab signer: cipher required")
	}

	cfg := signerConfig{
		prefix:     SecretsPrefix,
		defaultTTL: defaultSignerTTL,
		maxTTL:     defaultSignerMaxTTL,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.prefix == "" {
		cfg.prefix = SecretsPrefix
	}
	if !strings.HasSuffix(cfg.prefix, "/") {
		cfg.prefix += "/"
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Signer{
		client:     client,
		cipher:     cipher,
		prefix:     cfg.prefix,
		defaultTTL: cfg.defaultTTL,
		maxTTL:     cfg.maxTTL,
		now:        cfg.now,
		ctx:        ctx,
		cancel:     cancel,
		watchers:   make(map[int64]chan RotationEvent),
	}

	s.wg.Add(1)
	go s.watchRotations()

	return s, nil
}

// Close terminates background watchers and releases resources.
func (s *Signer) Close() error {
	s.cancel()
	s.wg.Wait()

	s.watchersMu.Lock()
	for id, ch := range s.watchers {
		delete(s.watchers, id)
		close(ch)
	}
	s.watchersMu.Unlock()
	return nil
}

// SubscribeRotations registers for secret rotation events.
func (s *Signer) SubscribeRotations() *RotationSubscription {
	ch := make(chan RotationEvent, 8)
	id := s.watcherSeq.Add(1)

	s.watchersMu.Lock()
	s.watchers[id] = ch
	s.watchersMu.Unlock()

	sub := &RotationSubscription{
		C: ch,
		closeFn: func() {
			s.watchersMu.Lock()
			if watcher, ok := s.watchers[id]; ok {
				delete(s.watchers, id)
				close(watcher)
			}
			s.watchersMu.Unlock()
		},
	}
	return sub
}

// RotateSecret stores the provided API key encrypted in etcd.
func (s *Signer) RotateSecret(ctx context.Context, req RotateSecretRequest) (RotateSecretResult, error) {
	name := strings.TrimSpace(req.SecretName)
	if name == "" {
		return RotateSecretResult{}, errEmptySecretName
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		return RotateSecretResult{}, errEmptyAPIKey
	}
	scopes := normalizeScopes(req.Scopes)
	if len(scopes) == 0 {
		return RotateSecretResult{}, errors.New("gitlab signer: at least one scope required")
	}

	encodedScopes, err := json.Marshal(scopes)
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: encode scopes: %w", err)
	}

	ciphertext, err := s.cipher.Encrypt(ctx, []byte(key))
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: encrypt api key: %w", err)
	}

	now := s.now().UTC()
	record := secretRecord{
		SecretName: name,
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Scopes:     scopes,
		ScopeJSON:  string(encodedScopes),
		UpdatedAt:  now.Format(time.RFC3339Nano),
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: marshal secret record: %w", err)
	}

	resp, err := s.client.Put(ctx, s.prefix+name, string(payload))
	if err != nil {
		return RotateSecretResult{}, fmt.Errorf("gitlab signer: put secret: %w", err)
	}
	var revision int64
	if resp != nil && resp.Header != nil {
		revision = resp.Header.Revision
	}
	return RotateSecretResult{
		Revision:  revision,
		UpdatedAt: now,
	}, nil
}

// IssueToken returns a short-lived token scoped to the requested permissions.
func (s *Signer) IssueToken(ctx context.Context, req IssueTokenRequest) (SignedToken, error) {
	name := strings.TrimSpace(req.SecretName)
	if name == "" {
		return SignedToken{}, errEmptySecretName
	}

	record, err := s.loadSecret(ctx, name)
	if err != nil {
		return SignedToken{}, err
	}

	requestedScopes := normalizeScopes(req.Scopes)
	if len(requestedScopes) == 0 {
		requestedScopes = record.Scopes
	}
	if err := ensureScopesAllowed(record.Scopes, requestedScopes); err != nil {
		return SignedToken{}, err
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = s.defaultTTL
	}
	if ttl > s.maxTTL {
		return SignedToken{}, fmt.Errorf("gitlab signer: requested ttl %s exceeds max %s", ttl, s.maxTTL)
	}

	issuedAt := s.now().UTC()
	expiresAt := issuedAt.Add(ttl)

	apiKey, err := record.decrypt(ctx, s.cipher)
	if err != nil {
		return SignedToken{}, err
	}

	value, err := mintToken(apiKey, requestedScopes, issuedAt, expiresAt)
	if err != nil {
		return SignedToken{}, err
	}

	return SignedToken{
		SecretName: name,
		Value:      value,
		Scopes:     requestedScopes,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Signer) loadSecret(ctx context.Context, name string) (secretRecord, error) {
	resp, err := s.client.Get(ctx, s.prefix+name, clientv3.WithLimit(1))
	if err != nil {
		return secretRecord{}, fmt.Errorf("gitlab signer: fetch secret: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return secretRecord{}, fmt.Errorf("gitlab signer: secret %q not found", name)
	}
	record, err := decodeSecretRecord(resp.Kvs[0], s.prefix)
	if err != nil {
		return secretRecord{}, err
	}
	return record, nil
}

func (s *Signer) watchRotations() {
	defer s.wg.Done()

	for {
		watchChan := s.client.Watch(s.ctx, s.prefix, clientv3.WithPrefix())
		for {
			select {
			case <-s.ctx.Done():
				return
			case resp, ok := <-watchChan:
				if !ok || resp.Canceled {
					goto restart
				}
				if err := resp.Err(); err != nil {
					continue
				}
				for _, ev := range resp.Events {
					if ev.Type != mvccpb.PUT {
						continue
					}
					record, err := decodeSecretRecord(ev.Kv, s.prefix)
					if err != nil {
						continue
					}
					event := RotationEvent{
						SecretName: record.SecretName,
						Revision:   ev.Kv.ModRevision,
						UpdatedAt:  record.updatedAt(),
					}
					s.dispatch(event)
				}
			}
		}
	restart:
		if s.ctx.Err() != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (s *Signer) dispatch(event RotationEvent) {
	s.watchersMu.RLock()
	listeners := make([]chan RotationEvent, 0, len(s.watchers))
	for _, ch := range s.watchers {
		listeners = append(listeners, ch)
	}
	s.watchersMu.RUnlock()

	for _, ch := range listeners {
		select {
		case ch <- event:
		default:
		}
	}
}

type secretRecord struct {
	SecretName string   `json:"secret_name"`
	Ciphertext string   `json:"ciphertext"`
	Scopes     []string `json:"scopes"`
	ScopeJSON  string   `json:"scope_json"`
	UpdatedAt  string   `json:"updated_at"`
}

func (r secretRecord) updatedAt() time.Time {
	if r.UpdatedAt == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	if err != nil {
		return time.Time{}
	}
	return ts
}

func (r secretRecord) decrypt(ctx context.Context, cipher Cipher) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(r.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: decode ciphertext: %w", err)
	}
	plain, err := cipher.Decrypt(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: decrypt api key: %w", err)
	}
	return string(plain), nil
}

func decodeSecretRecord(kv *mvccpb.KeyValue, prefix string) (secretRecord, error) {
	var record secretRecord
	if err := json.Unmarshal(kv.Value, &record); err != nil {
		return secretRecord{}, fmt.Errorf("gitlab signer: decode secret payload: %w", err)
	}
	if record.SecretName == "" {
		record.SecretName = strings.TrimPrefix(string(kv.Key), prefix)
	}
	if len(record.Scopes) == 0 && record.ScopeJSON != "" {
		var scopes []string
		if err := json.Unmarshal([]byte(record.ScopeJSON), &scopes); err == nil {
			record.Scopes = scopes
		}
	}
	return record, nil
}

func normalizeScopes(scopes []string) []string {
	cleaned := cleanList(scopes, false)
	result := make([]string, 0, len(cleaned))
	for _, scope := range cleaned {
		if scope != "" {
			result = append(result, scope)
		}
	}
	return result
}

func ensureScopesAllowed(allowed, requested []string) error {
	set := make(map[string]struct{}, len(allowed))
	for _, scope := range allowed {
		set[scope] = struct{}{}
	}
	for _, scope := range requested {
		if _, ok := set[scope]; !ok {
			return fmt.Errorf("gitlab signer: scope %q not permitted", scope)
		}
	}
	return nil
}

func mintToken(apiKey string, scopes []string, issuedAt, expiresAt time.Time) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("gitlab signer: api key required for minting token")
	}
	payload := map[string]any{
		"iat": issuedAt.Unix(),
		"exp": expiresAt.Unix(),
		"scp": scopes,
	}

	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("gitlab signer: generate token nonce: %w", err)
	}
	payload["rnd"] = base64.RawURLEncoding.EncodeToString(nonce)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: encode token payload: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write(payloadJSON)
	signature := mac.Sum(nil)

	tokenEnvelope := map[string]string{
		"payload": base64.RawURLEncoding.EncodeToString(payloadJSON),
		"sig":     base64.RawURLEncoding.EncodeToString(signature),
	}

	envelopeJSON, err := json.Marshal(tokenEnvelope)
	if err != nil {
		return "", fmt.Errorf("gitlab signer: encode token envelope: %w", err)
	}
	return "gls_" + base64.RawURLEncoding.EncodeToString(envelopeJSON), nil
}
