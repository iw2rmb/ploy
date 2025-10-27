package transfers

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type artifactCache struct {
	client      *clientv3.Client
	prefix      string
	revisionKey string
	clock       func() time.Time

	mu       sync.RWMutex
	jobIndex map[string][]Artifact
}

type artifactCacheOptions struct {
	Client      *clientv3.Client
	Prefix      string
	RevisionKey string
	Clock       func() time.Time
}

func newArtifactCache(opts artifactCacheOptions) *artifactCache {
	return &artifactCache{
		client:   opts.Client,
		prefix:   opts.Prefix,
		clock:    opts.Clock,
		jobIndex: make(map[string][]Artifact),
		revisionKey: func() string {
			if strings.TrimSpace(opts.RevisionKey) == "" {
				return opts.Prefix + "#rev"
			}
			return opts.RevisionKey
		}(),
	}
}

func (c *artifactCache) start(ctx context.Context) {
	go c.run(ctx)
}

func (c *artifactCache) run(ctx context.Context) {
	delay := 100 * time.Millisecond
	for ctx.Err() == nil {
		rev, err := c.loadSnapshot(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}
		if err := c.watch(ctx, rev+1); err != nil {
			if ctx.Err() != nil {
				return
			}
			if err == rpctypes.ErrCompacted {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}
}

func (c *artifactCache) loadSnapshot(ctx context.Context) (int64, error) {
	resp, err := c.client.Get(ctx, c.prefix, clientv3.WithPrefix())
	if err != nil {
		return 0, err
	}
	snapshot := make([]Artifact, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var env artifactEnvelope
		if err := json.Unmarshal(kv.Value, &env); err != nil {
			continue
		}
		snapshot = append(snapshot, env.Artifact)
	}
	sortArtifacts(snapshot)
	c.replaceAll(snapshot)
	if err := c.saveRevision(ctx, resp.Header.Revision); err != nil {
		return 0, err
	}
	return resp.Header.Revision, nil
}

func (c *artifactCache) watch(ctx context.Context, rev int64) error {
	opts := []clientv3.OpOption{clientv3.WithPrefix(), clientv3.WithPrevKV()}
	if rev > 0 {
		opts = append(opts, clientv3.WithRev(rev))
	}
	watchCh := c.client.Watch(ctx, c.prefix, opts...)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp, ok := <-watchCh:
			if !ok {
				return nil
			}
			if err := resp.Err(); err != nil {
				return err
			}
			var lastRev int64
			for _, ev := range resp.Events {
				switch ev.Type {
				case clientv3.EventTypePut:
					var env artifactEnvelope
					if err := json.Unmarshal(ev.Kv.Value, &env); err != nil {
						continue
					}
					c.upsert(env.Artifact)
				case clientv3.EventTypeDelete:
					job, artID := c.parseKey(string(ev.Kv.Key))
					c.remove(job, artID)
				}
				lastRev = ev.Kv.ModRevision
			}
			if lastRev > 0 {
				_ = c.saveRevision(ctx, lastRev)
			}
		}
	}
}

func (c *artifactCache) parseKey(raw string) (string, string) {
	trimmed := strings.TrimPrefix(raw, c.prefix)
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func (c *artifactCache) saveRevision(ctx context.Context, rev int64) error {
	if rev <= 0 {
		return nil
	}
	_, err := c.client.Put(ctx, c.revisionKey, strconv.FormatInt(rev, 10))
	return err
}

func (c *artifactCache) replaceAll(list []Artifact) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jobIndex = make(map[string][]Artifact)
	for _, art := range list {
		job := strings.TrimSpace(art.JobID)
		if job == "" {
			continue
		}
		c.jobIndex[job] = append(c.jobIndex[job], art)
	}
}

func (c *artifactCache) upsert(artifact Artifact) {
	job := strings.TrimSpace(artifact.JobID)
	if job == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.jobIndex[job]
	filtered := list[:0]
	for _, existing := range list {
		if existing.ID != artifact.ID {
			filtered = append(filtered, existing)
		}
	}
	newList := make([]Artifact, 0, len(filtered)+1)
	newList = append(newList, artifact)
	newList = append(newList, filtered...)
	c.jobIndex[job] = newList
}

func (c *artifactCache) remove(jobID, artifactID string) {
	job := strings.TrimSpace(jobID)
	if job == "" || strings.TrimSpace(artifactID) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	list := c.jobIndex[job]
	if len(list) == 0 {
		return
	}
	filtered := list[:0]
	for _, existing := range list {
		if existing.ID != artifactID {
			filtered = append(filtered, existing)
		}
	}
	if len(filtered) == len(list) {
		return
	}
	if len(filtered) == 0 {
		delete(c.jobIndex, job)
		return
	}
	newList := make([]Artifact, len(filtered))
	copy(newList, filtered)
	c.jobIndex[job] = newList
}

func (c *artifactCache) replace(jobID string, artifacts []Artifact) {
	job := strings.TrimSpace(jobID)
	if job == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	newList := make([]Artifact, len(artifacts))
	copy(newList, artifacts)
	c.jobIndex[job] = newList
}

func (c *artifactCache) forJob(jobID string) []Artifact {
	job := strings.TrimSpace(jobID)
	if job == "" {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	list := c.jobIndex[job]
	if len(list) == 0 {
		return nil
	}
	out := make([]Artifact, len(list))
	copy(out, list)
	return out
}
