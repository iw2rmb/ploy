// Package bootstrap implements node GitLab credential management as described in
// docs/design/gitlab-node-refresh/README.md; it adheres to the signer contract
// outlined in docs/design/gitlab-token-signer/README.md.
package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/node/git"
)

const (
	defaultRefreshLead  = time.Minute
	minRefreshLead      = 5 * time.Second
	defaultRetryBackoff = 5 * time.Second
	minRefreshWindow    = 5 * time.Second
)

// Field represents a structured logging attribute.
type Field struct {
	Key   string
	Value any
}

// String constructs a string field.
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Duration constructs a duration field.
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

// TimeField constructs a time field.
func TimeField(key string, value time.Time) Field {
	return Field{Key: key, Value: value}
}

// Int64 constructs an int64 field.
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Logger captures structured logs emitted during bootstrap operations.
type Logger interface {
	Debug(message string, fields ...Field)
	Info(message string, fields ...Field)
	Error(message string, err error, fields ...Field)
}

// Signer defines the GitLab signer client contract used by the node bootstrapper.
type Signer interface {
	Handshake(ctx context.Context, req HandshakeRequest) (HandshakeResponse, error)
	IssueToken(ctx context.Context, req TokenRequest) (gitlab.SignedToken, error)
	SubscribeRotations(ctx context.Context, req RotationSubscriptionRequest) (RotationSubscription, error)
}

// HandshakeRequest specifies bootstrap handshake parameters.
type HandshakeRequest struct {
	SecretName string
	Scopes     []string
	NodeID     string
	TLSConfig  *tls.Config
}

// HandshakeResponse returns the initial token and resolved scopes.
type HandshakeResponse struct {
	Token  gitlab.SignedToken
	Scopes []string
}

// TokenRequest requests a refreshed GitLab token from the signer.
type TokenRequest struct {
	SecretName string
	Scopes     []string
	TTL        time.Duration
	NodeID     string
}

// RotationSubscriptionRequest registers for rotation events.
type RotationSubscriptionRequest struct {
	SecretName string
}

// RotationSubscription delivers rotation events from the signer.
type RotationSubscription struct {
	C      <-chan gitlab.RotationEvent
	Cancel func()

	once sync.Once
}

// Close releases the rotation subscription resources.
func (r *RotationSubscription) Close() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.Cancel != nil {
			r.Cancel()
		}
	})
}

// Config configures the GitLab bootstrapper.
type Config struct {
	SecretName          string
	Scopes              []string
	NodeID              string
	Signer              Signer
	Cache               *git.CredentialCache
	Metrics             metrics.GitLabNodeRecorder
	Logger              Logger
	TLSConfig           *tls.Config
	Clock               clockwork.Clock
	RefreshBeforeExpiry time.Duration
	RetryBackoff        time.Duration
	RequestedTTL        time.Duration
}

// GitLabBootstrap manages GitLab credential refresh for worker nodes.
type GitLabBootstrap struct {
	secret       string
	scopes       []string
	requestedTTL time.Duration
	nodeID       string

	signer Signer
	cache  *git.CredentialCache
	log    Logger
	clock  clockwork.Clock

	metrics   metrics.GitLabNodeRecorder
	tlsConfig *tls.Config

	refreshLead  time.Duration
	retryBackoff time.Duration
}

// NewGitLabBootstrap validates config and constructs a bootstrapper.
func NewGitLabBootstrap(cfg Config) (*GitLabBootstrap, error) {
	secret := strings.TrimSpace(cfg.SecretName)
	if secret == "" {
		return nil, errors.New("bootstrap: secret name required")
	}
	nodeID := strings.TrimSpace(cfg.NodeID)
	if nodeID == "" {
		return nil, errors.New("bootstrap: node id required")
	}
	if cfg.Signer == nil {
		return nil, errors.New("bootstrap: signer client required")
	}
	if cfg.Cache == nil {
		return nil, errors.New("bootstrap: credential cache required")
	}
	if cfg.TLSConfig == nil {
		return nil, errors.New("bootstrap: tls config required")
	}

	refreshLead := cfg.RefreshBeforeExpiry
	if refreshLead <= 0 {
		refreshLead = defaultRefreshLead
	}
	if refreshLead < minRefreshLead {
		refreshLead = minRefreshLead
	}

	retryBackoff := cfg.RetryBackoff
	if retryBackoff <= 0 {
		retryBackoff = defaultRetryBackoff
	}

	clock := cfg.Clock
	if clock == nil {
		clock = clockwork.NewRealClock()
	}

	recorder := cfg.Metrics
	if recorder == nil {
		recorder = metrics.NewNoopGitLabNodeRecorder()
	}

	logger := cfg.Logger
	if logger == nil {
		logger = noopLogger{}
	}

	return &GitLabBootstrap{
		secret:       secret,
		scopes:       sanitizeScopes(cfg.Scopes),
		requestedTTL: cfg.RequestedTTL,
		nodeID:       nodeID,
		signer:       cfg.Signer,
		cache:        cfg.Cache,
		log:          logger,
		clock:        clock,
		metrics:      recorder,
		tlsConfig:    cfg.TLSConfig,
		refreshLead:  refreshLead,
		retryBackoff: retryBackoff,
	}, nil
}

// Run executes the bootstrap handshake and refresh loop until context cancellation.
func (b *GitLabBootstrap) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("bootstrap: context required")
	}

	resp, err := b.performHandshake(ctx)
	if err != nil {
		return err
	}

	if len(resp.Scopes) > 0 {
		b.scopes = sanitizeScopes(resp.Scopes)
	}

	b.cache.Set(resp.Token)
	b.metrics.RefreshSuccess(b.secret, resp.Token.ExpiresAt)

	b.log.Info("gitlab signer handshake established",
		String("secret", b.secret),
		TimeField("expires_at", resp.Token.ExpiresAt))

	if err := b.refreshLoop(ctx, resp.Token); err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("bootstrap: refresh loop: %w", err)
	}
	return nil
}

func (b *GitLabBootstrap) performHandshake(ctx context.Context) (HandshakeResponse, error) {
	backoff := b.retryBackoff
	if backoff <= 0 {
		backoff = defaultRetryBackoff
	}

	for {
		resp, err := b.signer.Handshake(ctx, HandshakeRequest{
			SecretName: b.secret,
			Scopes:     b.scopes,
			NodeID:     b.nodeID,
			TLSConfig:  b.tlsConfig,
		})
		if err == nil {
			return resp, nil
		}
		b.log.Error("gitlab signer handshake failed", err, Duration("backoff", backoff))
		if err := b.wait(ctx, backoff); err != nil {
			return HandshakeResponse{}, err
		}
	}
}

func (b *GitLabBootstrap) refreshLoop(ctx context.Context, current gitlab.SignedToken) error {
	sub, err := b.signer.SubscribeRotations(ctx, RotationSubscriptionRequest{
		SecretName: b.secret,
	})
	if err != nil {
		return fmt.Errorf("subscribe rotations: %w", err)
	}
	defer sub.Close()

	rotationC := sub.C
	now := b.clock.Now()
	nextRefresh := b.nextRefreshTime(current, now)
	waitDuration := durationUntil(nextRefresh, now)
	timer := b.clock.NewTimer(waitDuration)
	b.log.Debug("gitlab refresh timer armed",
		String("secret", b.secret),
		Duration("wait", waitDuration))
	defer timer.Stop()

	for {
		refreshC := timer.Chan()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-rotationC:
			if !ok {
				rotationC = nil
				continue
			}
			if ev.SecretName != "" && ev.SecretName != b.secret {
				continue
			}
			b.log.Info("gitlab token rotation detected",
				String("secret", b.secret),
				Int64("revision", ev.Revision))
			b.cache.Flush(b.secret)
			b.metrics.CacheFlushed(b.secret)
			nextRefresh = b.clock.Now()
			wait := durationUntil(nextRefresh, b.clock.Now())
			timer = b.resetTimer(timer, wait)
			b.log.Debug("gitlab refresh timer re-armed after rotation",
				String("secret", b.secret),
				Duration("wait", wait))
		case <-refreshC:
			token, err := b.signer.IssueToken(ctx, TokenRequest{
				SecretName: b.secret,
				Scopes:     b.scopes,
				TTL:        b.requestedTTL,
				NodeID:     b.nodeID,
			})
			if err != nil {
				b.metrics.RefreshFailure(b.secret, err)
				b.log.Error("gitlab token refresh failed", err,
					Duration("backoff", b.retryBackoff))
				nextRefresh = b.clock.Now().Add(b.retryBackoff)
				wait := durationUntil(nextRefresh, b.clock.Now())
				timer = b.resetTimer(timer, wait)
				b.log.Debug("gitlab refresh timer re-armed after failure",
					String("secret", b.secret),
					Duration("wait", wait))
				continue
			}

			b.cache.Set(token)
			b.metrics.RefreshSuccess(b.secret, token.ExpiresAt)
			b.log.Info("gitlab token refreshed",
				String("secret", b.secret),
				TimeField("expires_at", token.ExpiresAt))

			nextRefresh = b.nextRefreshTime(token, b.clock.Now())
			wait := durationUntil(nextRefresh, b.clock.Now())
			timer = b.resetTimer(timer, wait)
			b.log.Debug("gitlab refresh timer re-armed after success",
				String("secret", b.secret),
				Duration("wait", wait))
		}
	}
}

func (b *GitLabBootstrap) nextRefreshTime(token gitlab.SignedToken, now time.Time) time.Time {
	refreshLead := b.refreshLead
	if refreshLead <= 0 {
		refreshLead = defaultRefreshLead
	}

	next := token.ExpiresAt.Add(-refreshLead)
	if next.Before(now.Add(minRefreshWindow)) {
		next = now.Add(minRefreshWindow)
	}
	return next
}

func (b *GitLabBootstrap) resetTimer(timer clockwork.Timer, duration time.Duration) clockwork.Timer {
	duration = clampDuration(duration)
	if timer == nil {
		return b.clock.NewTimer(duration)
	}
	if !timer.Stop() {
		select {
		case <-timer.Chan():
		default:
		}
	}
	timer.Reset(duration)
	return timer
}

func (b *GitLabBootstrap) wait(ctx context.Context, duration time.Duration) error {
	duration = clampDuration(duration)
	timer := b.clock.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.Chan():
		return nil
	}
}

func durationUntil(target, now time.Time) time.Duration {
	return clampDuration(target.Sub(now))
}

func clampDuration(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}

func sanitizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	var out []string
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...Field)        {}
func (noopLogger) Info(string, ...Field)         {}
func (noopLogger) Error(string, error, ...Field) {}
