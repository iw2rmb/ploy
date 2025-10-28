package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/internal/metrics"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// addPublisher is the subset of the cluster client required by the step publisher.
type addPublisher interface {
	Add(ctx context.Context, req AddRequest) (AddResponse, error)
}

const (
	defaultMaxRetries = 2
	defaultRetryDelay = 200 * time.Millisecond
)

// ClusterPublisherOptions configure the IPFS-backed step artifact publisher.
type ClusterPublisherOptions struct {
	Client addPublisher

	ReplicationFactorMin int
	ReplicationFactorMax int
	Local                bool
	MaxRetries           int
	RetryDelay           time.Duration
	Recorder             metrics.BundleRecorder

	// NameBuilder optionally customises the artifact file name using the artifact kind.
	NameBuilder func(kind step.ArtifactKind) string
}

// ClusterPublisher implements the step ArtifactPublisher backed by IPFS Cluster.
type ClusterPublisher struct {
	client      addPublisher
	replMin     int
	replMax     int
	local       bool
	maxRetries  int
	retryDelay  time.Duration
	recorder    metrics.BundleRecorder
	cache       map[string]step.PublishedArtifact
	mu          sync.Mutex
	nameBuilder func(kind step.ArtifactKind) string
}

// NewClusterPublisher constructs a publisher that uploads diff/log artifacts to IPFS Cluster.
func NewClusterPublisher(opts ClusterPublisherOptions) (*ClusterPublisher, error) {
	if opts.Client == nil {
		return nil, errors.New("artifacts: cluster publisher client required")
	}
	nameBuilder := opts.NameBuilder
	if nameBuilder == nil {
		nameBuilder = defaultNameBuilder
	}
	maxRetries := opts.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if opts.MaxRetries == 0 {
		maxRetries = defaultMaxRetries
	}
	retryDelay := opts.RetryDelay
	if retryDelay < 0 {
		retryDelay = 0
	}
	if opts.RetryDelay == 0 {
		retryDelay = defaultRetryDelay
	}
	recorder := opts.Recorder
	if recorder == nil {
		recorder = metrics.NewNoopBundleRecorder()
	}
	return &ClusterPublisher{
		client:      opts.Client,
		replMin:     opts.ReplicationFactorMin,
		replMax:     opts.ReplicationFactorMax,
		local:       opts.Local,
		maxRetries:  maxRetries,
		retryDelay:  retryDelay,
		recorder:    recorder,
		cache:       make(map[string]step.PublishedArtifact),
		nameBuilder: nameBuilder,
	}, nil
}

// Publish uploads the artifact payload to IPFS Cluster and returns the resulting reference.
func (p *ClusterPublisher) Publish(ctx context.Context, req step.ArtifactRequest) (step.PublishedArtifact, error) {
	if p == nil || p.client == nil {
		return step.PublishedArtifact{}, errors.New("artifacts: cluster publisher not configured")
	}
	payload, name, err := p.resolvePayload(req)
	if err != nil {
		return step.PublishedArtifact{}, err
	}

	digest := sha256.Sum256(payload)
	digestValue := "sha256:" + hex.EncodeToString(digest[:])
	cacheKey := p.cacheKey(req.Kind, digestValue)
	if artifact, ok := p.lookup(cacheKey); ok {
		return artifact, nil
	}

	addRequest := AddRequest{
		Name:                 name,
		Kind:                 req.Kind,
		Payload:              payload,
		ReplicationFactorMin: p.replMin,
		ReplicationFactorMax: p.replMax,
		Local:                p.local,
	}

	attempts := p.maxRetries + 1
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	start := time.Now()
	kindLabel := string(req.Kind)

	for attempt := 0; attempt < attempts; attempt++ {
		result, err := p.client.Add(ctx, addRequest)
		if err == nil {
			digestOut := strings.TrimSpace(result.Digest)
			if digestOut == "" {
				digestOut = digestValue
			}
			artifact := step.PublishedArtifact{
				CID:    result.CID,
				Kind:   req.Kind,
				Digest: digestOut,
				Size:   result.Size,
			}
			p.store(cacheKey, artifact)
			otherKey := p.cacheKey(req.Kind, digestOut)
			if otherKey != cacheKey {
				p.store(otherKey, artifact)
			}
			if p.recorder != nil {
				p.recorder.PinSuccess(kindLabel, time.Since(start))
			}
			return artifact, nil
		}

		lastErr = err
		if p.recorder != nil {
			p.recorder.PinFailure(kindLabel, err)
		}
		if attempt == attempts-1 {
			break
		}
		if p.recorder != nil {
			p.recorder.PinRetry(kindLabel)
		}
		if err := p.wait(ctx, attempt); err != nil {
			return step.PublishedArtifact{}, err
		}
	}

	if lastErr != nil {
		return step.PublishedArtifact{}, lastErr
	}
	if err := ctx.Err(); err != nil {
		return step.PublishedArtifact{}, err
	}
	return step.PublishedArtifact{}, errors.New("artifacts: add failed")
}

func (p *ClusterPublisher) resolvePayload(req step.ArtifactRequest) ([]byte, string, error) {
	if path := filepath.Clean(req.Path); path != "." && path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", fmt.Errorf("artifacts: read artifact %s: %w", path, err)
		}
		return data, p.deriveName(req.Kind, filepath.Base(path)), nil
	}
	if len(req.Buffer) > 0 {
		return req.Buffer, p.deriveName(req.Kind, ""), nil
	}
	return nil, "", errors.New("artifacts: artifact payload required")
}

// cacheKey returns the deduplication cache key combining kind and digest.
func (p *ClusterPublisher) cacheKey(kind step.ArtifactKind, digest string) string {
	return string(kind) + "|" + strings.TrimSpace(digest)
}

// lookup fetches a cached artifact for the provided key when available.
func (p *ClusterPublisher) lookup(key string) (step.PublishedArtifact, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	artifact, ok := p.cache[key]
	if !ok {
		return step.PublishedArtifact{}, false
	}
	return artifact, true
}

// store records an artifact in the deduplication cache.
func (p *ClusterPublisher) store(key string, artifact step.PublishedArtifact) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cache == nil {
		p.cache = make(map[string]step.PublishedArtifact)
	}
	p.cache[key] = artifact
}

// wait pauses between retries honouring the provided context.
func (p *ClusterPublisher) wait(ctx context.Context, attempt int) error {
	delay := p.retryDelay
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	factor := time.Duration(1 << attempt)
	if factor <= 0 {
		factor = 1
	}
	waitFor := delay * factor
	timer := time.NewTimer(waitFor)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *ClusterPublisher) deriveName(kind step.ArtifactKind, fallback string) string {
	if fallback != "" {
		return fallback
	}
	return p.nameBuilder(kind)
}

func defaultNameBuilder(kind step.ArtifactKind) string {
	timestamp := time.Now().UTC().Format("20060102-150405")
	switch kind {
	case step.ArtifactKindDiff:
		return fmt.Sprintf("diff-%s.tar", timestamp)
	case step.ArtifactKindLogs:
		return fmt.Sprintf("logs-%s.txt", timestamp)
	case step.ArtifactKindShiftReport:
		return fmt.Sprintf("shift-report-%s.json", timestamp)
	case step.ArtifactKindSnapshot:
		return fmt.Sprintf("snapshot-%s.tar", timestamp)
	default:
		return fmt.Sprintf("artifact-%s.bin", timestamp)
	}
}
