//go:build legacy
// +build legacy

package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Publisher writes lifecycle capacity snapshots to etcd and updates status cache.
type Publisher struct {
	mu        sync.Mutex
	client    *clientv3.Client
	collector *Collector
	cache     *Cache
	prefix    string
	nodeID    string
	now       func() time.Time
	revision  uint64
}

// PublisherOptions configure the lifecycle publisher.
type PublisherOptions struct {
	Client    *clientv3.Client
	Collector *Collector
	Cache     *Cache
	Prefix    string
	NodeID    string
	Clock     func() time.Time
}

// NewPublisher constructs a lifecycle publisher.
func NewPublisher(opts PublisherOptions) (*Publisher, error) {
	if opts.Collector == nil {
		return nil, errors.New("lifecycle: collector required")
	}
	if opts.Client == nil {
		return nil, errors.New("lifecycle: etcd client required")
	}
	clock := opts.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Publisher{
		client:    opts.Client,
		collector: opts.Collector,
		cache:     opts.Cache,
		prefix:    normalizePrefix(opts.Prefix),
		nodeID:    opts.NodeID,
		now:       clock,
	}, nil
}

// Publish collects the latest snapshot, persists capacity to etcd, and refreshes the cache.
func (p *Publisher) Publish(ctx context.Context) error {
	if p == nil {
		return nil
	}
	snapshot, err := p.collector.Collect(ctx)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.revision++
	revision := p.revision
	capacity := cloneAnyMap(snapshot.Capacity)
	capacity["revision"] = revision
	if _, ok := capacity["heartbeat"]; !ok {
		capacity["heartbeat"] = p.now().Format(time.RFC3339Nano)
	}
	nodeID := stringsTrim(p.nodeID)
	if nodeID == "" {
		p.mu.Unlock()
		return errors.New("lifecycle: node id required")
	}
	key := path.Join(p.prefix, nodeID, "capacity")
	payload, err := json.Marshal(capacity)
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("lifecycle: marshal capacity: %w", err)
	}

	if p.cache != nil {
		p.cache.Store(snapshot.Status)
	}
	if _, err := p.client.Put(ctx, key, string(payload)); err != nil {
		return fmt.Errorf("lifecycle: put capacity: %w", err)
	}
	return nil
}

// Close releases the underlying etcd client.
func (p *Publisher) Close(ctx context.Context) error {
	if p == nil || p.client == nil {
		return nil
	}
	_ = ctx
	return p.client.Close()
}

func normalizePrefix(prefix string) string {
	trimmed := strings.Trim(prefix, "/")
	if trimmed == "" {
		return "nodes"
	}
	return trimmed
}

func stringsTrim(value string) string {
	return strings.Trim(value, "/")
}
