package gitlab

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
