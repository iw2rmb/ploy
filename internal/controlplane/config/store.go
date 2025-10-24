package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// ErrNotFound indicates the configuration document was not present.
var ErrNotFound = errors.New("config: document not found")

// ErrConflict signals the stored document changed between read/write attempts.
var ErrConflict = errors.New("config: revision mismatch")

// Document captures the persisted cluster configuration and associated metadata.
type Document struct {
	Data       map[string]any
	VersionTag string
	UpdatedAt  time.Time
	UpdatedBy  string
}

// Store persists and retrieves configuration documents for clusters.
type Store struct {
	client *clientv3.Client
}

// NewStore constructs a configuration store backed by etcd.
func NewStore(client *clientv3.Client) (*Store, error) {
	if client == nil {
		return nil, errors.New("config: etcd client required")
	}
	return &Store{client: client}, nil
}

// Load returns the stored configuration document for the supplied cluster identifier.
func (s *Store) Load(ctx context.Context, clusterID string) (Document, int64, error) {
	key, err := s.key(clusterID)
	if err != nil {
		return Document{}, 0, err
	}

	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return Document{}, 0, fmt.Errorf("config: read document: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return Document{}, 0, ErrNotFound
	}

	rec, err := decodeRecord(resp.Kvs[0].Value)
	if err != nil {
		return Document{}, 0, err
	}
	return rec.toDocument(), resp.Kvs[0].ModRevision, nil
}

// Save writes the configuration document applying optimistic concurrency via the revision.
// expectedRevision:
//   - 0 enforces creation when the document is absent.
//   - >0 enforces updates to the specified revision.
//   - <0 skips concurrency checks (used for wildcard updates).
func (s *Store) Save(ctx context.Context, clusterID string, expectedRevision int64, doc Document) (Document, int64, error) {
	key, err := s.key(clusterID)
	if err != nil {
		return Document{}, 0, err
	}

	if doc.Data == nil {
		doc.Data = map[string]any{}
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = time.Now().UTC()
	} else {
		doc.UpdatedAt = doc.UpdatedAt.UTC()
	}

	rec := record{
		Data:       cloneMap(doc.Data),
		VersionTag: strings.TrimSpace(doc.VersionTag),
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339Nano),
		UpdatedBy:  strings.TrimSpace(doc.UpdatedBy),
	}

	payload, err := json.Marshal(rec)
	if err != nil {
		return Document{}, 0, fmt.Errorf("config: encode document: %w", err)
	}

	txn := s.client.Txn(ctx)
	switch {
	case expectedRevision < 0:
		// no concurrency guard
	case expectedRevision == 0:
		txn = txn.If(clientv3.Compare(clientv3.CreateRevision(key), "=", 0))
	default:
		txn = txn.If(clientv3.Compare(clientv3.ModRevision(key), "=", expectedRevision))
	}
	txn = txn.Then(clientv3.OpPut(key, string(payload)))

	resp, err := txn.Commit()
	if err != nil {
		return Document{}, 0, fmt.Errorf("config: commit document: %w", err)
	}
	if len(resp.Responses) == 0 && expectedRevision >= 0 && !resp.Succeeded {
		return Document{}, 0, ErrConflict
	}
	if expectedRevision >= 0 && !resp.Succeeded {
		return Document{}, 0, ErrConflict
	}

	return s.Load(ctx, clusterID)
}

func (s *Store) key(clusterID string) (string, error) {
	trimmed := strings.TrimSpace(clusterID)
	if trimmed == "" {
		return "", errors.New("config: cluster id required")
	}
	return fmt.Sprintf("/ploy/clusters/%s/config/document", trimmed), nil
}

func decodeRecord(data []byte) (record, error) {
	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		return record{}, fmt.Errorf("config: decode document: %w", err)
	}
	return rec, nil
}

type record struct {
	Data       map[string]any `json:"data,omitempty"`
	VersionTag string         `json:"version_tag,omitempty"`
	UpdatedAt  string         `json:"updated_at"`
	UpdatedBy  string         `json:"updated_by,omitempty"`
}

func (r record) toDocument() Document {
	doc := Document{
		Data:       cloneMap(r.Data),
		VersionTag: r.VersionTag,
		UpdatedBy:  r.UpdatedBy,
	}
	if ts, err := time.Parse(time.RFC3339Nano, r.UpdatedAt); err == nil {
		doc.UpdatedAt = ts.UTC()
	}
	return doc
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
