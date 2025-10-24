package gitlab

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	revoker    TokenRevoker
	audit      AuditRecorder

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	watchersMu sync.RWMutex
	watchers   map[int64]chan RotationEvent
	watcherSeq atomic.Int64

	issuedMu sync.Mutex
	issued   map[string]map[string]issuedToken
}

// SignerOption customises signer configuration.
type SignerOption func(*signerConfig)

type signerConfig struct {
	prefix     string
	defaultTTL time.Duration
	maxTTL     time.Duration
	now        func() time.Time
	revoker    TokenRevoker
	audit      AuditRecorder
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

// WithTokenRevoker overrides the GitLab token revoker used during rotations.
func WithTokenRevoker(revoker TokenRevoker) SignerOption {
	return func(cfg *signerConfig) {
		if revoker != nil {
			cfg.revoker = revoker
		}
	}
}

// WithAuditRecorder overrides the audit recorder used for rotation events.
func WithAuditRecorder(recorder AuditRecorder) SignerOption {
	return func(cfg *signerConfig) {
		if recorder != nil {
			cfg.audit = recorder
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

// AuditAction enumerates rotation audit activity types.
type AuditAction string

const (
	// AuditActionIssued records token issuance to a node.
	AuditActionIssued AuditAction = "issued"
	// AuditActionRevoked records successful token revocation.
	AuditActionRevoked AuditAction = "revoked"
	// AuditActionRevocationFailed records a revocation failure.
	AuditActionRevocationFailed AuditAction = "revocation_failed"
)

// AuditEvent captures rotation audit metadata for observability pipelines.
type AuditEvent struct {
	Action     AuditAction
	SecretName string
	NodeID     string
	TokenID    string
	Timestamp  time.Time
	ExpiresAt  time.Time
	Error      string
}

// AuditRecorder consumes rotation audit events.
type AuditRecorder interface {
	Record(event AuditEvent)
}

// NoopAuditRecorder returns an audit recorder that discards events.
func NoopAuditRecorder() AuditRecorder { return noopAuditRecorder{} }

type noopAuditRecorder struct{}

func (noopAuditRecorder) Record(AuditEvent) {}

// RevocableToken describes a token subject to GitLab API revocation.
type RevocableToken struct {
	ID     string
	NodeID string
}

// RevocationFailure tracks a revocation error for a specific token.
type RevocationFailure struct {
	Token RevocableToken
	Err   error
}

// RevocationReport summarises revocation outcomes.
type RevocationReport struct {
	Revoked []RevocableToken
	Failed  []RevocationFailure
}

// TokenRevoker revokes GitLab tokens via the GitLab API.
type TokenRevoker interface {
	Revoke(ctx context.Context, secret string, tokens []RevocableToken) RevocationReport
}

// NoopTokenRevoker returns a revoker that performs no operations.
func NoopTokenRevoker() TokenRevoker { return noopTokenRevoker{} }

type noopTokenRevoker struct{}

func (noopTokenRevoker) Revoke(context.Context, string, []RevocableToken) RevocationReport {
	return RevocationReport{}
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
	NodeID     string
}

// SignedToken captures the issued token and metadata.
type SignedToken struct {
	SecretName string
	Value      string
	Scopes     []string
	IssuedAt   time.Time
	ExpiresAt  time.Time
	TokenID    string
}

// TokenClaims captures validated token metadata.
type TokenClaims struct {
	SecretName string
	TokenID    string
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
	if cfg.revoker == nil {
		cfg.revoker = NoopTokenRevoker()
	}
	if cfg.audit == nil {
		cfg.audit = NoopAuditRecorder()
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
		revoker:    cfg.revoker,
		audit:      cfg.audit,
		issued:     make(map[string]map[string]issuedToken),
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
	result := RotateSecretResult{
		Revision:  revision,
		UpdatedAt: now,
	}
	s.handleRotation(ctx, name)
	return result, nil
}

// IssueToken returns a short-lived token scoped to the requested permissions.
func (s *Signer) IssueToken(ctx context.Context, req IssueTokenRequest) (SignedToken, error) {
	name := strings.TrimSpace(req.SecretName)
	if name == "" {
		return SignedToken{}, errEmptySecretName
	}
	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		return SignedToken{}, errors.New("gitlab signer: node_id required for issuance")
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

	tokenID, err := generateTokenID()
	if err != nil {
		return SignedToken{}, err
	}

	value, err := mintToken(apiKey, requestedScopes, issuedAt, expiresAt, tokenID)
	if err != nil {
		return SignedToken{}, err
	}

	signed := SignedToken{
		SecretName: name,
		Value:      value,
		Scopes:     requestedScopes,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
		TokenID:    tokenID,
	}
	s.recordIssuedToken(name, nodeID, signed)

	return signed, nil
}

func (s *Signer) recordIssuedToken(secret, nodeID string, token SignedToken) {
	evt := AuditEvent{
		Action:     AuditActionIssued,
		SecretName: secret,
		NodeID:     nodeID,
		TokenID:    token.TokenID,
		Timestamp:  token.IssuedAt,
		ExpiresAt:  token.ExpiresAt,
	}

	s.issuedMu.Lock()
	bySecret := s.ensureIssuedLocked(secret)
	bySecret[token.TokenID] = issuedToken{
		tokenID:   token.TokenID,
		nodeID:    nodeID,
		issuedAt:  token.IssuedAt,
		expiresAt: token.ExpiresAt,
	}
	s.issuedMu.Unlock()

	s.audit.Record(evt)
}

type tokenEnvelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"sig"`
}

type tokenPayload struct {
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	Scopes    []string `json:"scp"`
	TokenID   string   `json:"tid"`
}

// ValidateToken verifies the bearer token and returns its claims.
func (s *Signer) ValidateToken(ctx context.Context, token string) (TokenClaims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return TokenClaims{}, errors.New("gitlab signer: token required")
	}
	if !strings.HasPrefix(token, "gls_") {
		return TokenClaims{}, errors.New("gitlab signer: malformed token")
	}
	rawEnvelope, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "gls_"))
	if err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode token envelope: %w", err)
	}

	var envelope tokenEnvelope
	if err := json.Unmarshal(rawEnvelope, &envelope); err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode token payload: %w", err)
	}
	if envelope.Payload == "" || envelope.Signature == "" {
		return TokenClaims{}, errors.New("gitlab signer: token envelope incomplete")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode payload body: %w", err)
	}
	var payload tokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode payload: %w", err)
	}
	if payload.IssuedAt == 0 || payload.ExpiresAt == 0 {
		return TokenClaims{}, errors.New("gitlab signer: token missing timestamps")
	}
	issuedAt := time.Unix(payload.IssuedAt, 0).UTC()
	expiresAt := time.Unix(payload.ExpiresAt, 0).UTC()
	if time.Now().UTC().After(expiresAt) {
		return TokenClaims{}, errors.New("gitlab signer: token expired")
	}

	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return TokenClaims{}, fmt.Errorf("gitlab signer: decode signature: %w", err)
	}

	record, err := s.findTokenSecret(ctx, payload.TokenID, payloadJSON, signature)
	if err != nil {
		return TokenClaims{}, err
	}

	scopes := normalizeScopes(payload.Scopes)
	if err := ensureScopesAllowed(record.Scopes, scopes); err != nil {
		return TokenClaims{}, err
	}

	return TokenClaims{
		SecretName: record.SecretName,
		TokenID:    strings.TrimSpace(payload.TokenID),
		Scopes:     scopes,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
	}, nil
}

func (s *Signer) findTokenSecret(ctx context.Context, tokenID string, payloadJSON, signature []byte) (secretRecord, error) {
	// Attempt to resolve via in-memory cache first.
	s.issuedMu.Lock()
	var cachedSecret string
	for secret, tokens := range s.issued {
		if _, ok := tokens[tokenID]; ok {
			cachedSecret = secret
			break
		}
	}
	s.issuedMu.Unlock()

	if cachedSecret != "" {
		record, err := s.loadSecret(ctx, cachedSecret)
		if err == nil {
			if ok, verifyErr := s.secretMatches(ctx, record, payloadJSON, signature); verifyErr == nil && ok {
				return record, nil
			}
		}
	}

	secrets, err := s.listSecrets(ctx)
	if err != nil {
		return secretRecord{}, err
	}
	for _, record := range secrets {
		match, matchErr := s.secretMatches(ctx, record, payloadJSON, signature)
		if matchErr != nil {
			continue
		}
		if match {
			return record, nil
		}
	}
	return secretRecord{}, errors.New("gitlab signer: token not recognized")
}

func (s *Signer) secretMatches(ctx context.Context, record secretRecord, payloadJSON, signature []byte) (bool, error) {
	apiKey, err := record.decrypt(ctx, s.cipher)
	if err != nil {
		return false, err
	}
	mac := hmac.New(sha256.New, []byte(apiKey))
	mac.Write(payloadJSON)
	if !hmac.Equal(mac.Sum(nil), signature) {
		return false, nil
	}
	return true, nil
}

func (s *Signer) listSecrets(ctx context.Context) ([]secretRecord, error) {
	resp, err := s.client.Get(ctx, s.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("gitlab signer: list secrets: %w", err)
	}
	records := make([]secretRecord, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		record, err := decodeSecretRecord(kv, s.prefix)
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Signer) handleRotation(ctx context.Context, secret string) {
	tokens := s.popIssuedTokens(secret)
	if len(tokens) == 0 {
		return
	}
	revoker := s.revoker
	if revoker == nil {
		return
	}

	revocable := make([]RevocableToken, 0, len(tokens))
	lookup := make(map[string]issuedToken, len(tokens))
	for _, tok := range tokens {
		revocable = append(revocable, RevocableToken{ID: tok.tokenID, NodeID: tok.nodeID})
		lookup[tok.tokenID] = tok
	}

	report := revoker.Revoke(ctx, secret, revocable)
	now := s.now().UTC()

	for _, revoked := range report.Revoked {
		s.audit.Record(AuditEvent{
			Action:     AuditActionRevoked,
			SecretName: secret,
			NodeID:     revoked.NodeID,
			TokenID:    revoked.ID,
			Timestamp:  now,
		})
		delete(lookup, revoked.ID)
	}

	if len(report.Failed) == 0 {
		return
	}

	var retry []issuedToken
	for _, failure := range report.Failed {
		orig, ok := lookup[failure.Token.ID]
		if ok {
			retry = append(retry, orig)
		}
		errMsg := ""
		if failure.Err != nil {
			errMsg = failure.Err.Error()
		}
		s.audit.Record(AuditEvent{
			Action:     AuditActionRevocationFailed,
			SecretName: secret,
			NodeID:     failure.Token.NodeID,
			TokenID:    failure.Token.ID,
			Timestamp:  now,
			Error:      errMsg,
		})
	}
	if len(retry) > 0 {
		s.requeueTokens(secret, retry)
	}
}

func (s *Signer) ensureIssuedLocked(secret string) map[string]issuedToken {
	if s.issued == nil {
		s.issued = make(map[string]map[string]issuedToken)
	}
	bySecret, ok := s.issued[secret]
	if !ok {
		bySecret = make(map[string]issuedToken)
		s.issued[secret] = bySecret
	}
	return bySecret
}

func (s *Signer) popIssuedTokens(secret string) []issuedToken {
	s.issuedMu.Lock()
	defer s.issuedMu.Unlock()
	bySecret, ok := s.issued[secret]
	if !ok || len(bySecret) == 0 {
		return nil
	}
	result := make([]issuedToken, 0, len(bySecret))
	for _, tok := range bySecret {
		result = append(result, tok)
	}
	delete(s.issued, secret)
	return result
}

func (s *Signer) requeueTokens(secret string, tokens []issuedToken) {
	s.issuedMu.Lock()
	defer s.issuedMu.Unlock()
	bySecret := s.ensureIssuedLocked(secret)
	for _, tok := range tokens {
		bySecret[tok.tokenID] = tok
	}
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

type issuedToken struct {
	tokenID   string
	nodeID    string
	issuedAt  time.Time
	expiresAt time.Time
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

func generateTokenID() (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("gitlab signer: generate token id: %w", err)
	}
	return hex.EncodeToString(id), nil
}

func mintToken(apiKey string, scopes []string, issuedAt, expiresAt time.Time, tokenID string) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("gitlab signer: api key required for minting token")
	}
	payload := map[string]any{
		"iat": issuedAt.Unix(),
		"exp": expiresAt.Unix(),
		"scp": scopes,
	}
	if trimmed := strings.TrimSpace(tokenID); trimmed != "" {
		payload["tid"] = trimmed
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
