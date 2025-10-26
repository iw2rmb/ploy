package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrNotFound indicates the requested artifact metadata does not exist.
var ErrNotFound = errors.New("artifacts: metadata not found")

const (
	defaultPrefix    = "ploy"
	defaultListLimit = 50
	maxListLimit     = 200
)

// StoreOptions configure the metadata store.
type StoreOptions struct {
	Prefix string
	Clock  func() time.Time
}

// Store persists artifact metadata in etcd with derived indexes for job/stage lookups.
type Store struct {
	client *clientv3.Client
	prefix string
	clock  func() time.Time
}

// NewStore constructs an etcd-backed artifact metadata store.
func NewStore(client *clientv3.Client, opts StoreOptions) (*Store, error) {
	if client == nil {
		return nil, errors.New("artifacts: etcd client required")
	}
	prefix := strings.Trim(opts.Prefix, "/")
	if prefix == "" {
		prefix = defaultPrefix
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Store{
		client: client,
		prefix: "/" + prefix,
		clock:  clock,
	}, nil
}

// Create inserts a new artifact metadata document.
func (s *Store) Create(ctx context.Context, meta Metadata) (Metadata, error) {
	if s == nil || s.client == nil {
		return Metadata{}, errors.New("artifacts: store uninitialised")
	}
	meta.ID = strings.TrimSpace(meta.ID)
	if meta.ID == "" {
		return Metadata{}, errors.New("artifacts: id required")
	}
	meta.JobID = strings.TrimSpace(meta.JobID)
	if meta.JobID == "" {
		return Metadata{}, errors.New("artifacts: job id required")
	}
	meta.CID = strings.TrimSpace(meta.CID)
	if meta.CID == "" {
		return Metadata{}, errors.New("artifacts: cid required")
	}
	meta.Digest = strings.TrimSpace(meta.Digest)
	if meta.Digest == "" {
		return Metadata{}, errors.New("artifacts: digest required")
	}
	now := s.clock().UTC()
	meta.CreatedAt = now
	meta.UpdatedAt = now
	meta.Stage = strings.TrimSpace(meta.Stage)
	meta.Kind = strings.TrimSpace(meta.Kind)
	meta.NodeID = strings.TrimSpace(meta.NodeID)
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Source = strings.TrimSpace(meta.Source)
	meta.SlotID = strings.TrimSpace(meta.SlotID)
	meta.TTL = strings.TrimSpace(meta.TTL)
	if meta.ExpiresAt.IsZero() && meta.TTL != "" {
		if duration, err := time.ParseDuration(meta.TTL); err == nil && duration > 0 {
			meta.ExpiresAt = meta.CreatedAt.Add(duration)
		}
	}

	record := recordFromMetadata(meta)
	payload, err := json.Marshal(record)
	if err != nil {
		return Metadata{}, fmt.Errorf("artifacts: encode metadata: %w", err)
	}

	key := s.artifactKey(meta.ID)
	ops := []clientv3.Op{clientv3.OpPut(key, string(payload))}
	ops = append(ops, clientv3.OpPut(s.jobIndexKey(meta.JobID, meta.ID), meta.ID))
	if meta.Stage != "" {
		ops = append(ops, clientv3.OpPut(s.stageIndexKey(meta.JobID, meta.Stage, meta.ID), meta.ID))
	}

	txn := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0)).
		Then(ops...)

	resp, err := txn.Commit()
	if err != nil {
		return Metadata{}, fmt.Errorf("artifacts: create %s: %w", meta.ID, err)
	}
	if !resp.Succeeded {
		return Metadata{}, fmt.Errorf("artifacts: id %s already exists", meta.ID)
	}
	return meta, nil
}

// Get fetches metadata for the supplied artifact identifier.
func (s *Store) Get(ctx context.Context, id string) (Metadata, error) {
	meta, err := s.read(ctx, strings.TrimSpace(id))
	if err != nil {
		return Metadata{}, err
	}
	if meta.Deleted {
		return Metadata{}, ErrNotFound
	}
	return meta, nil
}

// List returns artifacts matching the provided filters, ordered lexicographically.
func (s *Store) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	if s == nil || s.client == nil {
		return ListResult{}, errors.New("artifacts: store uninitialised")
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	jobID := strings.TrimSpace(opts.JobID)
	stage := strings.TrimSpace(opts.Stage)

	if jobID != "" {
		return s.listByJob(ctx, jobID, stage, opts.Cursor, limit, opts.IncludeDeleted)
	}
	return s.listAll(ctx, opts.Cursor, limit, opts.IncludeDeleted)
}

// Delete marks metadata as deleted and removes index entries.
func (s *Store) Delete(ctx context.Context, id string) (Metadata, error) {
	if s == nil || s.client == nil {
		return Metadata{}, errors.New("artifacts: store uninitialised")
	}
	key := s.artifactKey(strings.TrimSpace(id))
	if key == "" {
		return Metadata{}, errors.New("artifacts: id required")
	}

	meta, rev, err := s.readWithRevision(ctx, id)
	if err != nil {
		return Metadata{}, err
	}
	if meta.Deleted {
		return meta, nil
	}
	now := s.clock().UTC()
	meta.Deleted = true
	meta.DeletedAt = now
	meta.UpdatedAt = now

	record := recordFromMetadata(meta)
	payload, err := json.Marshal(record)
	if err != nil {
		return Metadata{}, fmt.Errorf("artifacts: encode delete payload: %w", err)
	}

	ops := []clientv3.Op{clientv3.OpPut(key, string(payload)), clientv3.OpDelete(s.jobIndexKey(meta.JobID, meta.ID))}
	if meta.Stage != "" {
		ops = append(ops, clientv3.OpDelete(s.stageIndexKey(meta.JobID, meta.Stage, meta.ID)))
	}

	txn := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", rev)).
		Then(ops...)

	resp, err := txn.Commit()
	if err != nil {
		return Metadata{}, fmt.Errorf("artifacts: delete %s: %w", meta.ID, err)
	}
	if !resp.Succeeded {
		return Metadata{}, fmt.Errorf("artifacts: delete %s: conflict", meta.ID)
	}
	return meta, nil
}
