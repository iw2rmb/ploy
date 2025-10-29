package hydration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

// Index persists hydration snapshot metadata for reuse across tickets.
type Index struct {
	client       *clientv3.Client
	prefix       string
	ticketPrefix string
	clock        func() time.Time
}

// IndexOptions configures snapshot index behaviour.
type IndexOptions struct {
	Prefix string
	Clock  func() time.Time
}

// ReplicationPolicy records replication expectations for hydration snapshots.
type ReplicationPolicy struct {
	Min int `json:"min,omitempty"`
	Max int `json:"max,omitempty"`
}

// SharingPolicy conveys whether the snapshot can be reused by other tickets.
type SharingPolicy struct {
	Enabled bool `json:"enabled"`
}

// SnapshotRecord describes an observed hydration snapshot.
type SnapshotRecord struct {
	RepoURL     string
	Revision    string
	TicketID    string
	Bundle      scheduler.BundleRecord
	Replication ReplicationPolicy
	Sharing     SharingPolicy
}

// SnapshotEntry returns persisted hydration snapshot metadata.
type SnapshotEntry struct {
	Fingerprint string
	RepoURL     string
	Revision    string
	Bundle      scheduler.BundleRecord
	Replication ReplicationPolicy
	Sharing     SharingPolicy
	Tickets     map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ExpiresAt   time.Time
}

// LookupRequest scopes lookup queries to repository fingerprint.
type LookupRequest struct {
	RepoURL  string
	Revision string
}

// Policy surfaces hydration retention metadata for operators.
type Policy struct {
	Fingerprint     string
	Ticket          string
	RepoURL         string
	Revision        string
	SharedCID       string
	TTL             string
	ReplicationMin  int
	ReplicationMax  int
	Share           bool
	ExpiresAt       time.Time
	ReuseCandidates []string
}

// PolicyUpdate applies retention overrides to stored policies.
type PolicyUpdate struct {
	TTL            *string
	ReplicationMin *int
	ReplicationMax *int
	Share          *bool
}

// TuneRequest mirrors CLI/API update payloads.
type TuneRequest struct {
	TTL            string
	ReplicationMin *int
	ReplicationMax *int
	Share          *bool
}

// NewIndex constructs a snapshot index backed by etcd.
func NewIndex(client *clientv3.Client, opts IndexOptions) (*Index, error) {
	if client == nil {
		return nil, errors.New("hydration: etcd client required")
	}
	prefix := strings.TrimSpace(opts.Prefix)
	if prefix == "" {
		prefix = "hydration/index/"
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Index{
		client:       client,
		prefix:       prefix,
		ticketPrefix: path.Join(prefix, "tickets") + "/",
		clock:        clock,
	}, nil
}

// UpsertSnapshot records or updates a snapshot entry, binding the ticket to the fingerprint.
func (i *Index) UpsertSnapshot(ctx context.Context, record SnapshotRecord) (SnapshotEntry, error) {
	var zero SnapshotEntry
	if i == nil {
		return zero, errors.New("hydration: index not configured")
	}
	repo := strings.TrimSpace(record.RepoURL)
	revision := strings.TrimSpace(record.Revision)
	ticket := strings.TrimSpace(record.TicketID)
	if repo == "" || revision == "" {
		return zero, errors.New("hydration: repo url and revision required")
	}
	if ticket == "" {
		return zero, errors.New("hydration: ticket id required")
	}
	if strings.TrimSpace(record.Bundle.CID) == "" {
		return zero, errors.New("hydration: bundle cid required")
	}

	fingerprint := computeFingerprint(repo, revision)
	now := i.clock().UTC()

	key := path.Join(i.prefix, fingerprint)

	var doc snapshotDocument
	resp, err := i.client.Get(ctx, key)
	if err != nil {
		return zero, fmt.Errorf("hydration: fetch snapshot: %w", err)
	}
	if len(resp.Kvs) > 0 {
		if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
			return zero, fmt.Errorf("hydration: decode snapshot: %w", err)
		}
	} else {
		doc = snapshotDocument{
			Fingerprint: fingerprint,
			RepoURL:     repo,
			Revision:    revision,
			CreatedAt:   encodeTime(now),
			Tickets:     map[string]string{},
		}
	}

	doc.RepoURL = repo
	doc.Revision = revision
	doc.Bundle = record.Bundle
	if !record.Replication.empty() {
		doc.Replication = record.Replication
	}
	if ticket != "" {
		if doc.Tickets == nil {
			doc.Tickets = make(map[string]string)
		}
		doc.Tickets[ticket] = encodeTime(now)
	}
	if record.Sharing != (SharingPolicy{}) {
		doc.Sharing = record.Sharing
	}
	doc.UpdatedAt = encodeTime(now)

	payload, err := json.Marshal(doc)
	if err != nil {
		return zero, fmt.Errorf("hydration: marshal snapshot: %w", err)
	}

	if _, err := i.client.Put(ctx, key, string(payload)); err != nil {
		return zero, fmt.Errorf("hydration: persist snapshot: %w", err)
	}
	// bind ticket to fingerprint for fast lookup
	if ticket != "" {
		if _, err := i.client.Put(ctx, i.ticketKey(ticket), fingerprint); err != nil {
			return zero, fmt.Errorf("hydration: bind ticket: %w", err)
		}
	}
	return doc.toEntry()
}

// LookupSnapshot resolves snapshot metadata by repository fingerprint.
func (i *Index) LookupSnapshot(ctx context.Context, req LookupRequest) (SnapshotEntry, bool, error) {
	var zero SnapshotEntry
	if i == nil {
		return zero, false, errors.New("hydration: index not configured")
	}
	repo := strings.TrimSpace(req.RepoURL)
	revision := strings.TrimSpace(req.Revision)
	if repo == "" || revision == "" {
		return zero, false, errors.New("hydration: repo url and revision required")
	}
	fingerprint := computeFingerprint(repo, revision)
	key := path.Join(i.prefix, fingerprint)
	resp, err := i.client.Get(ctx, key)
	if err != nil {
		return zero, false, fmt.Errorf("hydration: lookup snapshot: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return zero, false, nil
	}
	var doc snapshotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return zero, false, fmt.Errorf("hydration: decode snapshot: %w", err)
	}
	entry, err := doc.toEntry()
	if err != nil {
		return zero, false, err
	}
	return entry, true, nil
}

// LookupTicket resolves the snapshot associated with a ticket if available.
func (i *Index) LookupTicket(ctx context.Context, ticket string) (SnapshotEntry, bool, error) {
	var zero SnapshotEntry
	if i == nil {
		return zero, false, errors.New("hydration: index not configured")
	}
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return zero, false, errors.New("hydration: ticket required")
	}
	resp, err := i.client.Get(ctx, i.ticketKey(ticket))
	if err != nil {
		return zero, false, fmt.Errorf("hydration: lookup ticket binding: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return zero, false, nil
	}
	fingerprint := string(resp.Kvs[0].Value)
	return i.lookupByFingerprint(ctx, fingerprint)
}

// ListSnapshots returns all stored hydration snapshots for policy evaluation.
func (i *Index) ListSnapshots(ctx context.Context) ([]SnapshotEntry, error) {
	if i == nil {
		return nil, errors.New("hydration: index not configured")
	}
	resp, err := i.client.Get(ctx, i.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("hydration: list snapshots: %w", err)
	}
	entries := make([]SnapshotEntry, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		if strings.HasPrefix(string(kv.Key), i.ticketPrefix) {
			continue
		}
		var doc snapshotDocument
		if err := json.Unmarshal(kv.Value, &doc); err != nil {
			return nil, fmt.Errorf("hydration: decode snapshot document: %w", err)
		}
		entry, err := doc.toEntry()
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UpdatedAt.Before(entries[j].UpdatedAt)
	})
	return entries, nil
}

// UpdateTicket applies retention overrides for the ticket's hydration snapshot.
func (i *Index) UpdateTicket(ctx context.Context, ticket string, update PolicyUpdate) (SnapshotEntry, error) {
	var zero SnapshotEntry
	if i == nil {
		return zero, errors.New("hydration: index not configured")
	}
	entry, ok, err := i.LookupTicket(ctx, ticket)
	if err != nil {
		return zero, err
	}
	if !ok {
		return zero, fmt.Errorf("hydration: ticket %s has no hydration snapshot", ticket)
	}
	key := path.Join(i.prefix, entry.Fingerprint)
	resp, err := i.client.Get(ctx, key)
	if err != nil {
		return zero, fmt.Errorf("hydration: fetch snapshot for update: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return zero, fmt.Errorf("hydration: snapshot %s missing", entry.Fingerprint)
	}
	var doc snapshotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return zero, fmt.Errorf("hydration: decode snapshot: %w", err)
	}

	changed := false
	if update.TTL != nil {
		doc.Bundle.TTL = strings.TrimSpace(*update.TTL)
		changed = true
	}
	if update.ReplicationMin != nil {
		doc.Replication.Min = *update.ReplicationMin
		changed = true
	}
	if update.ReplicationMax != nil {
		doc.Replication.Max = *update.ReplicationMax
		changed = true
	}
	if update.Share != nil {
		doc.Sharing.Enabled = *update.Share
		changed = true
	}
	if !changed {
		return doc.toEntry()
	}
	doc.UpdatedAt = encodeTime(i.clock().UTC())
	payload, err := json.Marshal(doc)
	if err != nil {
		return zero, fmt.Errorf("hydration: marshal updated snapshot: %w", err)
	}
	if _, err := i.client.Put(ctx, key, string(payload)); err != nil {
		return zero, fmt.Errorf("hydration: persist updated snapshot: %w", err)
	}
	return doc.toEntry()
}

// PolicyForTicket converts stored entry into operator-facing policy.
func (i *Index) PolicyForTicket(ctx context.Context, ticket string) (Policy, bool, error) {
	entry, ok, err := i.LookupTicket(ctx, ticket)
	if err != nil {
		return Policy{}, false, err
	}
	if !ok {
		return Policy{}, false, nil
	}
	return entry.toPolicy(ticket), true, nil
}

// UpdateReplication overrides the replication policy for a stored snapshot.
func (i *Index) UpdateReplication(ctx context.Context, fingerprint string, replication ReplicationPolicy) (SnapshotEntry, error) {
	var zero SnapshotEntry
	if i == nil {
		return zero, errors.New("hydration: index not configured")
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return zero, errors.New("hydration: fingerprint required")
	}
	key := path.Join(i.prefix, fingerprint)
	resp, err := i.client.Get(ctx, key)
	if err != nil {
		return zero, fmt.Errorf("hydration: fetch snapshot %s: %w", fingerprint, err)
	}
	if len(resp.Kvs) == 0 {
		return zero, fmt.Errorf("hydration: snapshot %s missing", fingerprint)
	}
	var doc snapshotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return zero, fmt.Errorf("hydration: decode snapshot: %w", err)
	}
	doc.Replication = replication
	doc.UpdatedAt = encodeTime(i.clock().UTC())

	payload, err := json.Marshal(doc)
	if err != nil {
		return zero, fmt.Errorf("hydration: marshal snapshot: %w", err)
	}

	txn := i.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", resp.Kvs[0].ModRevision)).
		Then(clientv3.OpPut(key, string(payload)))

	result, err := txn.Commit()
	if err != nil {
		return zero, fmt.Errorf("hydration: update replication: %w", err)
	}
	if !result.Succeeded {
		return zero, fmt.Errorf("hydration: snapshot %s concurrently modified", fingerprint)
	}
	return doc.toEntry()
}

// DeleteSnapshot removes a snapshot and clears associated ticket bindings.
func (i *Index) DeleteSnapshot(ctx context.Context, fingerprint string) error {
	if i == nil {
		return errors.New("hydration: index not configured")
	}
	fingerprint = strings.TrimSpace(fingerprint)
	if fingerprint == "" {
		return errors.New("hydration: fingerprint required")
	}
	key := path.Join(i.prefix, fingerprint)
	resp, err := i.client.Get(ctx, key)
	if err != nil {
		return fmt.Errorf("hydration: fetch snapshot %s: %w", fingerprint, err)
	}
	if len(resp.Kvs) == 0 {
		return nil
	}
	var doc snapshotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return fmt.Errorf("hydration: decode snapshot: %w", err)
	}

	ops := []clientv3.Op{clientv3.OpDelete(key)}
	for ticket := range doc.Tickets {
		ops = append(ops, clientv3.OpDelete(i.ticketKey(ticket)))
	}

	txn := i.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", resp.Kvs[0].ModRevision)).
		Then(ops...)

	result, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("hydration: delete snapshot: %w", err)
	}
	if !result.Succeeded {
		return fmt.Errorf("hydration: snapshot %s concurrently modified", fingerprint)
	}
	return nil
}

// ticketKey constructs the etcd key for ticket bindings.
func (i *Index) ticketKey(ticket string) string {
	return path.Join(i.ticketPrefix, ticket)
}

func (i *Index) lookupByFingerprint(ctx context.Context, fingerprint string) (SnapshotEntry, bool, error) {
	var zero SnapshotEntry
	if strings.TrimSpace(fingerprint) == "" {
		return zero, false, nil
	}
	key := path.Join(i.prefix, fingerprint)
	resp, err := i.client.Get(ctx, key)
	if err != nil {
		return zero, false, fmt.Errorf("hydration: lookup fingerprint: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return zero, false, nil
	}
	var doc snapshotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return zero, false, fmt.Errorf("hydration: decode snapshot doc: %w", err)
	}
	entry, err := doc.toEntry()
	if err != nil {
		return zero, false, err
	}
	return entry, true, nil
}

func computeFingerprint(repoURL, revision string) string {
	normalized := strings.ToLower(strings.TrimSpace(repoURL)) + "@" + strings.TrimSpace(revision)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

type snapshotDocument struct {
	Fingerprint string                 `json:"fingerprint"`
	RepoURL     string                 `json:"repo_url"`
	Revision    string                 `json:"revision"`
	Bundle      scheduler.BundleRecord `json:"bundle"`
	Replication ReplicationPolicy      `json:"replication,omitempty"`
	Sharing     SharingPolicy          `json:"sharing,omitempty"`
	Tickets     map[string]string      `json:"tickets,omitempty"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

func (d snapshotDocument) toEntry() (SnapshotEntry, error) {
	created, err := parseTime(d.CreatedAt)
	if err != nil {
		return SnapshotEntry{}, fmt.Errorf("hydrate: parse created_at: %w", err)
	}
	updated, err := parseTime(d.UpdatedAt)
	if err != nil {
		return SnapshotEntry{}, fmt.Errorf("hydrate: parse updated_at: %w", err)
	}
	expires, err := parseTime(d.Bundle.ExpiresAt)
	if err != nil && strings.TrimSpace(d.Bundle.ExpiresAt) != "" {
		return SnapshotEntry{}, fmt.Errorf("hydrate: parse expires_at: %w", err)
	}
	tickets := cloneTickets(d.Tickets)
	return SnapshotEntry{
		Fingerprint: d.Fingerprint,
		RepoURL:     d.RepoURL,
		Revision:    d.Revision,
		Bundle:      d.Bundle,
		Replication: d.Replication,
		Sharing:     d.Sharing,
		Tickets:     tickets,
		CreatedAt:   created,
		UpdatedAt:   updated,
		ExpiresAt:   expires,
	}, nil
}

func (d snapshotDocument) toPolicy(ticket string) Policy {
	entry, err := d.toEntry()
	if err != nil {
		return Policy{}
	}
	return entry.toPolicy(ticket)
}

func (e SnapshotEntry) toPolicy(ticket string) Policy {
	policy := Policy{
		Fingerprint:    e.Fingerprint,
		Ticket:         ticket,
		RepoURL:        e.RepoURL,
		Revision:       e.Revision,
		SharedCID:      strings.TrimSpace(e.Bundle.CID),
		TTL:            strings.TrimSpace(e.Bundle.TTL),
		ReplicationMin: e.Replication.Min,
		ReplicationMax: e.Replication.Max,
		Share:          e.Sharing.Enabled,
		ExpiresAt:      e.ExpiresAt,
	}
	if len(e.Tickets) > 0 {
		policy.ReuseCandidates = make([]string, 0, len(e.Tickets))
		for key := range e.Tickets {
			policy.ReuseCandidates = append(policy.ReuseCandidates, key)
		}
		sort.Strings(policy.ReuseCandidates)
	}
	return policy
}

func (p ReplicationPolicy) empty() bool {
	return p.Min == 0 && p.Max == 0
}

func cloneTickets(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}
