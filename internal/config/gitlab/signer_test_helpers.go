package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

type revocationCall struct {
	secret string
	tokens []RevocableToken
}

type recordingRevoker struct {
	mu       sync.Mutex
	failures map[string]error
	calls    []revocationCall
}

// newRecordingRevoker builds a revoker fake that can inject per-token failures.
func newRecordingRevoker(failures map[string]error) *recordingRevoker {
	if failures == nil {
		failures = make(map[string]error)
	}
	return &recordingRevoker{failures: failures}
}

// Revoke captures the provided tokens and returns a report matching configured failures.
func (r *recordingRevoker) Revoke(_ context.Context, secret string, tokens []RevocableToken) RevocationReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := append([]RevocableToken(nil), tokens...)
	r.calls = append(r.calls, revocationCall{secret: secret, tokens: clone})

	report := RevocationReport{}
	for _, tok := range tokens {
		if err, ok := r.failures[tok.ID]; ok {
			report.Failed = append(report.Failed, RevocationFailure{
				Token: tok,
				Err:   err,
			})
			continue
		}
		report.Revoked = append(report.Revoked, tok)
	}
	return report
}

// lastCall returns the most recent revocation invocation for assertions.
func (r *recordingRevoker) lastCall() *revocationCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return nil
	}
	call := r.calls[len(r.calls)-1]
	return &call
}

type recordingAudit struct {
	mu     sync.Mutex
	events []AuditEvent
}

// newRecordingAudit creates an in-memory audit recorder for tests.
func newRecordingAudit() *recordingAudit {
	return &recordingAudit{}
}

// Record captures the provided audit event for later inspection.
func (r *recordingAudit) Record(event AuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

// eventsByAction filters stored events by action to support focused assertions.
func (r *recordingAudit) eventsByAction(action AuditAction) []AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []AuditEvent
	for _, evt := range r.events {
		if evt.Action == action {
			out = append(out, evt)
		}
	}
	return out
}

// newTestEtcd starts an embedded etcd server and client for signer integration tests.
func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir

	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")

	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "signer"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "signer-test"
	cfg.Logger = "zap"
	cfg.LogLevel = "panic"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	etcd, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start embedded etcd: %v", err)
	}

	select {
	case <-etcd.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for etcd readiness")
	}

	endpoint := etcd.Clients[0].Addr().String()
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create etcd client: %v", err)
	}

	return etcd, client
}

// mustParseURL converts a raw URL string into a url.URL or panics for tests.
func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}

// mustNewAESCipher builds a cipher from the provided key or fails the test.
func mustNewAESCipher(t *testing.T, key string) Cipher {
	t.Helper()
	c, err := NewAESCipher([]byte(key))
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return c
}

// mustNewSigner constructs a signer with the supplied options or fails the test.
func mustNewSigner(t *testing.T, client *clientv3.Client, cipher Cipher, opts ...SignerOption) *Signer {
	t.Helper()
	signer, err := NewSigner(client, cipher, opts...)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	return signer
}
