package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
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

// Metadata captures persisted artifact attributes backed by etcd.
type Metadata struct {
	ID                   string
	SlotID               string
	CID                  string
	Digest               string
	Size                 int64
	JobID                string
	Stage                string
	Kind                 string
	NodeID               string
	Name                 string
	Source               string
	TTL                  string
	ExpiresAt            time.Time
	ReplicationFactorMin int
	ReplicationFactorMax int
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Deleted              bool
	DeletedAt            time.Time
}

// ListOptions scope artifact listings.
type ListOptions struct {
	JobID          string
	Stage          string
	Cursor         string
	Limit          int
	IncludeDeleted bool
}

// ListResult wraps a page of artifact metadata.
type ListResult struct {
	Artifacts  []Metadata
	NextCursor string
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

func (s *Store) listByJob(ctx context.Context, jobID, stage, cursor string, limit int, includeDeleted bool) (ListResult, error) {
	prefix := s.jobIndexPrefix(jobID)
	if stage != "" {
		prefix = s.stageIndexPrefix(jobID, stage)
	}
	start := prefix
	if cursor != "" {
		start = path.Join(prefix, cursor)
	}
	resp, err := s.client.Get(ctx, start,
		clientv3.WithRange(prefixRangeEnd(prefix)),
		clientv3.WithLimit(int64(limit+1)),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
	)
	if err != nil {
		return ListResult{}, fmt.Errorf("artifacts: list job %s: %w", jobID, err)
	}
	kvs := dropCursor(resp.Kvs, cursor, prefix)
	return s.collectFromIndex(ctx, kvs, limit, resp.More, includeDeleted)
}

func (s *Store) listAll(ctx context.Context, cursor string, limit int, includeDeleted bool) (ListResult, error) {
	prefix := s.artifactPrefix()
	start := prefix
	if cursor != "" {
		start = path.Join(prefix, cursor)
	}
	resp, err := s.client.Get(ctx, start,
		clientv3.WithRange(prefixRangeEnd(prefix)),
		clientv3.WithLimit(int64(limit+1)),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend))
	if err != nil {
		return ListResult{}, fmt.Errorf("artifacts: list: %w", err)
	}
	kvs := dropCursor(resp.Kvs, cursor, prefix)
	results := make([]Metadata, 0, len(kvs))
	var lastProcessedID string
	for _, kv := range kvs {
		id := artifactIDFromKey(string(kv.Key))
		lastProcessedID = id
		meta, err := s.readRawRecord(kv.Value)
		if err != nil {
			continue
		}
		if meta.Deleted && !includeDeleted {
			continue
		}
		results = append(results, meta)
		if len(results) == limit {
			break
		}
	}
	nextCursor := ""
	if (resp.More || len(results) == limit) && lastProcessedID != "" {
		nextCursor = lastProcessedID
	}
	return ListResult{Artifacts: results, NextCursor: nextCursor}, nil
}

func (s *Store) collectFromIndex(ctx context.Context, kvs []*mvccpb.KeyValue, limit int, hasMore bool, includeDeleted bool) (ListResult, error) {
	results := make([]Metadata, 0, len(kvs))
	var lastProcessedID string
	for _, kv := range kvs {
		id := artifactIDFromKey(string(kv.Key))
		if id == "" {
			continue
		}
		lastProcessedID = id
		meta, err := s.readRaw(ctx, id)
		if err != nil {
			continue
		}
		if meta.Deleted && !includeDeleted {
			continue
		}
		results = append(results, meta)
		if len(results) == limit {
			break
		}
	}
	nextCursor := ""
	if (hasMore || len(results) == limit) && lastProcessedID != "" {
		nextCursor = lastProcessedID
	}
	return ListResult{Artifacts: results, NextCursor: nextCursor}, nil
}

func (s *Store) read(ctx context.Context, id string) (Metadata, error) {
	meta, _, err := s.readWithRevision(ctx, id)
	return meta, err
}

func (s *Store) readWithRevision(ctx context.Context, id string) (Metadata, int64, error) {
	key := s.artifactKey(strings.TrimSpace(id))
	if key == "" {
		return Metadata{}, 0, ErrNotFound
	}
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return Metadata{}, 0, fmt.Errorf("artifacts: get %s: %w", id, err)
	}
	if len(resp.Kvs) == 0 {
		return Metadata{}, 0, ErrNotFound
	}
	meta, err := s.readRawRecord(resp.Kvs[0].Value)
	if err != nil {
		return Metadata{}, 0, err
	}
	return meta, resp.Kvs[0].ModRevision, nil
}

func (s *Store) readRaw(ctx context.Context, id string) (Metadata, error) {
	meta, _, err := s.readWithRevision(ctx, id)
	return meta, err
}

func (s *Store) readRawRecord(data []byte) (Metadata, error) {
	var rec metadataRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return Metadata{}, fmt.Errorf("artifacts: decode metadata: %w", err)
	}
	return rec.toMetadata(), nil
}

func (s *Store) artifactPrefix() string {
	return path.Join(s.prefix, "artifacts")
}

func (s *Store) artifactKey(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return ""
	}
	return path.Join(s.artifactPrefix(), trimmed)
}

func (s *Store) jobIndexPrefix(jobID string) string {
	return path.Join(s.prefix, "index", "artifacts", "jobs", encodeSegment(jobID), "artifacts")
}

func (s *Store) jobIndexKey(jobID, id string) string {
	return path.Join(s.jobIndexPrefix(jobID), strings.TrimSpace(id))
}

func (s *Store) stageIndexPrefix(jobID, stage string) string {
	return path.Join(s.prefix, "index", "artifacts", "jobs", encodeSegment(jobID), "stages", encodeSegment(stage))
}

func (s *Store) stageIndexKey(jobID, stage, id string) string {
	return path.Join(s.stageIndexPrefix(jobID, stage), strings.TrimSpace(id))
}

func encodeSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "_"
	}
	return url.PathEscape(trimmed)
}

func artifactIDFromKey(key string) string {
	idx := strings.LastIndex(key, "/")
	if idx == -1 {
		return key
	}
	return key[idx+1:]
}

func dropCursor(kvs []*mvccpb.KeyValue, cursor, prefix string) []*mvccpb.KeyValue {
	if cursor == "" || len(kvs) == 0 {
		return kvs
	}
	cursorKey := path.Join(prefix, cursor)
	if string(kvs[0].Key) == cursorKey {
		return kvs[1:]
	}
	return kvs
}

func prefixRangeEnd(prefix string) string {
	return clientv3.GetPrefixRangeEnd(prefix)
}

func recordFromMetadata(meta Metadata) metadataRecord {
	rec := metadataRecord{
		ID:                   meta.ID,
		SlotID:               meta.SlotID,
		CID:                  meta.CID,
		Digest:               meta.Digest,
		Size:                 meta.Size,
		JobID:                meta.JobID,
		Stage:                meta.Stage,
		Kind:                 meta.Kind,
		NodeID:               meta.NodeID,
		Name:                 meta.Name,
		Source:               meta.Source,
		TTL:                  meta.TTL,
		ReplicationFactorMin: meta.ReplicationFactorMin,
		ReplicationFactorMax: meta.ReplicationFactorMax,
		CreatedAt:            meta.CreatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:            meta.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Deleted:              meta.Deleted,
	}
	if !meta.ExpiresAt.IsZero() {
		rec.ExpiresAt = meta.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	if !meta.DeletedAt.IsZero() {
		rec.DeletedAt = meta.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	return rec
}

type metadataRecord struct {
	ID                   string `json:"id"`
	SlotID               string `json:"slot_id,omitempty"`
	CID                  string `json:"cid"`
	Digest               string `json:"digest"`
	Size                 int64  `json:"size"`
	JobID                string `json:"job_id"`
	Stage                string `json:"stage,omitempty"`
	Kind                 string `json:"kind,omitempty"`
	NodeID               string `json:"node_id,omitempty"`
	Name                 string `json:"name,omitempty"`
	Source               string `json:"source,omitempty"`
	TTL                  string `json:"ttl,omitempty"`
	ExpiresAt            string `json:"expires_at,omitempty"`
	ReplicationFactorMin int    `json:"replication_factor_min,omitempty"`
	ReplicationFactorMax int    `json:"replication_factor_max,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
	Deleted              bool   `json:"deleted,omitempty"`
	DeletedAt            string `json:"deleted_at,omitempty"`
}

func (r metadataRecord) toMetadata() Metadata {
	meta := Metadata{
		ID:                   strings.TrimSpace(r.ID),
		SlotID:               strings.TrimSpace(r.SlotID),
		CID:                  strings.TrimSpace(r.CID),
		Digest:               strings.TrimSpace(r.Digest),
		Size:                 r.Size,
		JobID:                strings.TrimSpace(r.JobID),
		Stage:                strings.TrimSpace(r.Stage),
		Kind:                 strings.TrimSpace(r.Kind),
		NodeID:               strings.TrimSpace(r.NodeID),
		Name:                 strings.TrimSpace(r.Name),
		Source:               strings.TrimSpace(r.Source),
		TTL:                  strings.TrimSpace(r.TTL),
		ReplicationFactorMin: r.ReplicationFactorMin,
		ReplicationFactorMax: r.ReplicationFactorMax,
		Deleted:              r.Deleted,
	}
	if ts, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err == nil {
		meta.CreatedAt = ts.UTC()
	}
	if ts, err := time.Parse(time.RFC3339Nano, r.UpdatedAt); err == nil {
		meta.UpdatedAt = ts.UTC()
	}
	if strings.TrimSpace(r.ExpiresAt) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, r.ExpiresAt); err == nil {
			meta.ExpiresAt = ts.UTC()
		}
	}
	if strings.TrimSpace(r.DeletedAt) != "" {
		if ts, err := time.Parse(time.RFC3339Nano, r.DeletedAt); err == nil {
			meta.DeletedAt = ts.UTC()
		}
	}
	return meta
}

// SortArtifactsByCreatedDesc sorts metadata in-place by CreatedAt descending.
func SortArtifactsByCreatedDesc(items []Metadata) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
}
