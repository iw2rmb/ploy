package bootstrap

import (
	"context"
	"crypto/tls"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"

	"github.com/iw2rmb/ploy/internal/config/gitlab"
	"github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/node/git"
)

func TestGitLabRefresh(t *testing.T) {
	t.Helper()

	clock := clockwork.NewFakeClock()
	cache := git.NewCredentialCache()
	recorder := metrics.NewInMemoryGitLabNodeRecorder()
	logger := newTestLogger(t)
	signer := newFakeSigner(clock, handshakeResult{
		value: "boot-token",
		ttl:   2 * time.Minute,
	})

	cfg := Config{
		SecretName:          "deploy",
		NodeID:              "node-bootstrap",
		Signer:              signer,
		Cache:               cache,
		Metrics:             recorder,
		Logger:              logger,
		TLSConfig:           &tls.Config{Certificates: []tls.Certificate{{}}},
		Clock:               clock,
		RefreshBeforeExpiry: 30 * time.Second,
		RetryBackoff:        10 * time.Second,
	}

	bootstrapper, err := NewGitLabBootstrap(cfg)
	if err != nil {
		t.Fatalf("bootstrap config failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := make(chan error, 1)
	go func() {
		runErr <- bootstrapper.Run(ctx)
	}()

	waitFor(t, time.Second, func() bool {
		return signer.handshakeCount() == 1
	})

	token, ok := cache.Get("deploy")
	if !ok {
		t.Fatalf("expected initial token cached")
	}
	if token.Value != "boot-token" {
		t.Fatalf("expected boot-token, got %q", token.Value)
	}

	if signer.tlsConfig() == nil {
		t.Fatalf("expected mutual TLS config passed to signer")
	}

	if signer.lastNode() != "node-bootstrap" {
		t.Fatalf("expected handshake to include node id, got %q", signer.lastNode())
	}

	if err := clock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("block until: %v", err)
	}
	signer.queue(issueResult{value: "refresh-token", ttl: 4 * time.Minute})
	clock.Advance(90 * time.Second)

	waitFor(t, time.Second, func() bool {
		next, ok := cache.Get("deploy")
		return ok && next.Value == "refresh-token"
	})

	if got := recorder.RefreshTotals("deploy")["success"]; got != 2 {
		t.Fatalf("expected 2 successful refresh events (bootstrap + refresh), got %d", got)
	}

	signer.queue(issueResult{err: errors.New("signer offline")})
	signer.queue(issueResult{value: "recovered-token", ttl: 5 * time.Minute})

	if err := clock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("block until: %v", err)
	}
	clock.Advance(4 * time.Minute)

	waitFor(t, time.Second, func() bool {
		return recorder.RefreshTotals("deploy")["failure"] >= 1
	})

	if err := clock.BlockUntilContext(ctx, 1); err != nil {
		t.Fatalf("block until: %v", err)
	}
	clock.Advance(10 * time.Second)

	waitFor(t, time.Second, func() bool {
		token, ok := cache.Get("deploy")
		return ok && token.Value == "recovered-token"
	})

	for _, node := range signer.issuedNodes() {
		if node != "node-bootstrap" {
			t.Fatalf("expected refresh calls to include node id node-bootstrap, got %q", node)
		}
	}

	signer.emitRotation(gitlab.RotationEvent{
		SecretName: "deploy",
		Revision:   42,
		UpdatedAt:  clock.Now(),
	})

	waitFor(t, time.Second, func() bool {
		_, ok := cache.Get("deploy")
		return !ok
	})

	if recorder.CacheFlushes("deploy") != 1 {
		t.Fatalf("expected cache flush metric to increment")
	}

	cancel()
	if err := <-runErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("unexpected run error: %v", err)
	}
}

type handshakeResult struct {
	value string
	ttl   time.Duration
}

type issueResult struct {
	value string
	ttl   time.Duration
	err   error
}

type fakeSigner struct {
	clock clockwork.Clock

	mu              sync.Mutex
	hCalls          int
	lastTLSConfig   *tls.Config
	lastNodeID      string
	issuedNodeIDs   []string
	handshake       handshakeResult
	issueQueue      chan issueResult
	rotationUpdates chan gitlab.RotationEvent
}

func newFakeSigner(clock clockwork.Clock, handshake handshakeResult) *fakeSigner {
	return &fakeSigner{
		clock:           clock,
		handshake:       handshake,
		issueQueue:      make(chan issueResult, 8),
		rotationUpdates: make(chan gitlab.RotationEvent, 8),
	}
}

func (f *fakeSigner) queue(res issueResult) {
	f.issueQueue <- res
}

func (f *fakeSigner) emitRotation(ev gitlab.RotationEvent) {
	f.rotationUpdates <- ev
}

func (f *fakeSigner) handshakeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hCalls
}

func (f *fakeSigner) tlsConfig() *tls.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastTLSConfig
}

func (f *fakeSigner) lastNode() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastNodeID
}

func (f *fakeSigner) issuedNodes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.issuedNodeIDs...)
}

func (f *fakeSigner) Handshake(ctx context.Context, req HandshakeRequest) (HandshakeResponse, error) {
	f.mu.Lock()
	f.hCalls++
	f.lastTLSConfig = req.TLSConfig
	f.lastNodeID = req.NodeID
	f.mu.Unlock()

	if req.TLSConfig == nil {
		return HandshakeResponse{}, errors.New("missing tls config")
	}
	if strings.TrimSpace(req.NodeID) == "" {
		return HandshakeResponse{}, errors.New("missing node id")
	}

	now := f.clock.Now()
	return HandshakeResponse{
		Token: gitlab.SignedToken{
			SecretName: req.SecretName,
			Value:      f.handshake.value,
			IssuedAt:   now,
			ExpiresAt:  now.Add(f.handshake.ttl),
		},
		Scopes: []string{"read_repository"},
	}, nil
}

func (f *fakeSigner) IssueToken(ctx context.Context, req TokenRequest) (gitlab.SignedToken, error) {
	select {
	case <-ctx.Done():
		return gitlab.SignedToken{}, ctx.Err()
	case res := <-f.issueQueue:
		if res.err != nil {
			return gitlab.SignedToken{}, res.err
		}
		if strings.TrimSpace(req.NodeID) == "" {
			return gitlab.SignedToken{}, errors.New("missing node id")
		}
		now := f.clock.Now()
		f.mu.Lock()
		f.issuedNodeIDs = append(f.issuedNodeIDs, req.NodeID)
		f.mu.Unlock()
		return gitlab.SignedToken{
			SecretName: req.SecretName,
			Value:      res.value,
			IssuedAt:   now,
			ExpiresAt:  now.Add(res.ttl),
			Scopes:     req.Scopes,
		}, nil
	}
}

func (f *fakeSigner) SubscribeRotations(ctx context.Context, req RotationSubscriptionRequest) (RotationSubscription, error) {
	return RotationSubscription{
		C: f.rotationUpdates,
		Cancel: func() {
			close(f.rotationUpdates)
		},
	}, nil
}

type testLogger struct {
	t *testing.T
}

func newTestLogger(t *testing.T) *testLogger {
	t.Helper()
	return &testLogger{t: t}
}

func (l *testLogger) Debug(msg string, fields ...Field) {
	l.t.Logf("DEBUG %s %v", msg, fields)
}

func (l *testLogger) Info(msg string, fields ...Field) {
	l.t.Logf("INFO %s %v", msg, fields)
}

func (l *testLogger) Error(msg string, err error, fields ...Field) {
	l.t.Logf("ERROR %s: %v %v", msg, err, fields)
}

func waitFor(t *testing.T, timeout time.Duration, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if predicate() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
