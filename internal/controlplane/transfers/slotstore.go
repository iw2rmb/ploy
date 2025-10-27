package transfers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// SlotStoreOptions configure persistence.
type SlotStoreOptions struct {
	ClusterID string
	TTL       time.Duration
	LeaseSkew time.Duration
	Clock     func() time.Time
}

// SlotRecord captures stored slot metadata.
type SlotRecord struct {
	Slot     Slot
	Revision int64
	LeaseID  clientv3.LeaseID
}

// SlotStore persists slots and artifacts in etcd.
type SlotStore struct {
	client          *clientv3.Client
	slotsPrefix     string
	artifactsPrefix string
	revisionKey     string
	ttl             time.Duration
	leaseTTL        int64
	clock           func() time.Time

	cache  *artifactCache
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// NewSlotStore initialises an etcd-backed slot store.
func NewSlotStore(client *clientv3.Client, opts SlotStoreOptions) (*SlotStore, error) {
	if client == nil {
		return nil, errors.New("slotstore: etcd client required")
	}
	clusterID := strings.TrimSpace(opts.ClusterID)
	if clusterID == "" {
		clusterID = "default"
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	leaseSkew := opts.LeaseSkew
	if leaseSkew <= 0 {
		leaseSkew = 5 * time.Minute
	}
	leaseTTL := int64((ttl + leaseSkew).Seconds())
	if leaseTTL < 60 {
		leaseTTL = 60
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}

	root := path.Join("/ploy/clusters", clusterID, "transfers")
	ctx, cancel := context.WithCancel(context.Background())
	store := &SlotStore{
		client:          client,
		slotsPrefix:     root + "/slots/",
		artifactsPrefix: root + "/artifacts/",
		revisionKey:     path.Join(root, "state", "artifacts_rev"),
		ttl:             ttl,
		leaseTTL:        leaseTTL,
		clock:           clock,
		ctx:             ctx,
		cancel:          cancel,
	}
	store.cache = newArtifactCache(artifactCacheOptions{
		Client:      client,
		Prefix:      store.artifactsPrefix,
		RevisionKey: store.revisionKey,
		Clock:       clock,
	})
	store.cache.start(ctx)
	return store, nil
}

// Close stops background watches.
func (s *SlotStore) Close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
	return nil
}

// CreateSlot writes a slot record bound to a lease.
func (s *SlotStore) CreateSlot(ctx context.Context, slot Slot) (SlotRecord, error) {
	if err := validateSlot(slot); err != nil {
		return SlotRecord{}, err
	}
	doc := slotDocument{
		Slot:      slot,
		CreatedAt: s.clock().UTC(),
		UpdatedAt: s.clock().UTC(),
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: marshal slot: %w", err)
	}
	lease, err := s.client.Grant(ctx, s.leaseTTL)
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: grant lease: %w", err)
	}
	key := s.slotKey(slot.ID)
	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(key), "=", 0),
	).Then(
		clientv3.OpPut(key, string(payload), clientv3.WithLease(lease.ID)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: create slot txn: %w", err)
	}
	if !resp.Succeeded {
		return SlotRecord{}, fmt.Errorf("slotstore: slot %s already exists", slot.ID)
	}
	modRev := resp.Header.Revision
	if len(resp.Responses) > 0 {
		if put := resp.Responses[0].GetResponsePut(); put != nil {
			modRev = put.Header.Revision
		}
	}
	return SlotRecord{Slot: slot, Revision: modRev, LeaseID: lease.ID}, nil
}

// GetSlot loads a slot document by id.
func (s *SlotStore) GetSlot(ctx context.Context, slotID string) (SlotRecord, error) {
	resp, err := s.client.Get(ctx, s.slotKey(slotID))
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: get slot: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return SlotRecord{}, fmt.Errorf("slotstore: slot %s not found", slotID)
	}
	var doc slotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: decode slot %s: %w", slotID, err)
	}
	return SlotRecord{
		Slot:     doc.Slot,
		Revision: resp.Kvs[0].ModRevision,
		LeaseID:  clientv3.LeaseID(resp.Kvs[0].Lease),
	}, nil
}

// UpdateSlotState atomically mutates the slot state/digest pair.
func (s *SlotStore) UpdateSlotState(ctx context.Context, slotID string, expectedRevision int64, state SlotState, digest string) (SlotRecord, error) {
	resp, err := s.client.Get(ctx, s.slotKey(slotID))
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: fetch slot: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return SlotRecord{}, fmt.Errorf("slotstore: slot %s not found", slotID)
	}
	if expectedRevision > 0 && resp.Kvs[0].ModRevision != expectedRevision {
		return SlotRecord{}, fmt.Errorf("slotstore: slot %s revision mismatch", slotID)
	}
	var doc slotDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: decode slot %s: %w", slotID, err)
	}
	doc.Slot.State = state
	if trimmed := strings.TrimSpace(digest); trimmed != "" {
		doc.Slot.Digest = trimmed
	}
	doc.UpdatedAt = s.clock().UTC()
	payload, err := json.Marshal(doc)
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: marshal slot %s: %w", slotID, err)
	}
	cmp := clientv3.Compare(clientv3.ModRevision(s.slotKey(slotID)), "=", resp.Kvs[0].ModRevision)
	put := clientv3.OpPut(s.slotKey(slotID), string(payload), clientv3.WithLease(clientv3.LeaseID(resp.Kvs[0].Lease)))
	txn, err := s.client.Txn(ctx).If(cmp).Then(put).Commit()
	if err != nil {
		return SlotRecord{}, fmt.Errorf("slotstore: update slot %s: %w", slotID, err)
	}
	if !txn.Succeeded {
		return SlotRecord{}, fmt.Errorf("slotstore: conflicting update for slot %s", slotID)
	}
	modRev := txn.Header.Revision
	if len(txn.Responses) > 0 {
		if putResp := txn.Responses[0].GetResponsePut(); putResp != nil {
			modRev = putResp.Header.Revision
		}
	}
	return SlotRecord{
		Slot:     doc.Slot,
		Revision: modRev,
		LeaseID:  clientv3.LeaseID(resp.Kvs[0].Lease),
	}, nil
}

// RecordArtifact persists artifact metadata for committed slots.
func (s *SlotStore) RecordArtifact(ctx context.Context, artifact Artifact) error {
	job := strings.TrimSpace(artifact.JobID)
	if job == "" {
		return errors.New("slotstore: artifact job id required")
	}
	artifact.JobID = job
	artifact.ID = strings.TrimSpace(artifact.ID)
	if artifact.ID == "" {
		return errors.New("slotstore: artifact id required")
	}
	if artifact.UpdatedAt.IsZero() {
		artifact.UpdatedAt = s.clock().UTC()
	} else {
		artifact.UpdatedAt = artifact.UpdatedAt.UTC()
	}
	data, err := json.Marshal(artifactEnvelope{Artifact: artifact})
	if err != nil {
		return fmt.Errorf("slotstore: marshal artifact: %w", err)
	}
	if _, err := s.client.Put(ctx, s.artifactKey(job, artifact.ID), string(data)); err != nil {
		return fmt.Errorf("slotstore: persist artifact: %w", err)
	}
	if s.cache != nil {
		s.cache.upsert(artifact)
	}
	return nil
}

// JobArtifacts returns artifacts for a job, preferring the cache.
func (s *SlotStore) JobArtifacts(ctx context.Context, jobID string) ([]Artifact, error) {
	job := strings.TrimSpace(jobID)
	if job == "" {
		return nil, errors.New("slotstore: job id required")
	}
	if s.cache != nil {
		if cached := s.cache.forJob(job); len(cached) > 0 {
			return cached, nil
		}
	}
	resp, err := s.client.Get(ctx, s.artifactKey(job, ""), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("slotstore: list artifacts: %w", err)
	}
	list := make([]Artifact, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var env artifactEnvelope
		if err := json.Unmarshal(kv.Value, &env); err != nil {
			continue
		}
		list = append(list, env.Artifact)
	}
	sortArtifacts(list)
	if s.cache != nil {
		s.cache.replace(job, list)
	}
	return list, nil
}

// CachedArtifacts exposes the most recent snapshot for a job.
func (s *SlotStore) CachedArtifacts(jobID string) []Artifact {
	if s.cache == nil {
		return nil
	}
	return s.cache.forJob(strings.TrimSpace(jobID))
}

func (s *SlotStore) slotKey(slotID string) string {
	return s.slotsPrefix + strings.TrimSpace(slotID)
}

func (s *SlotStore) artifactKey(jobID, artifactID string) string {
	job := strings.TrimSpace(jobID)
	if artifactID == "" {
		return s.artifactsPrefix + job + "/"
	}
	return s.artifactsPrefix + job + "/" + strings.TrimSpace(artifactID)
}

func validateSlot(slot Slot) error {
	switch {
	case strings.TrimSpace(slot.ID) == "":
		return errors.New("slotstore: slot id required")
	case strings.TrimSpace(slot.JobID) == "":
		return errors.New("slotstore: job id required")
	case strings.TrimSpace(slot.NodeID) == "":
		return errors.New("slotstore: node id required")
	case strings.TrimSpace(slot.RemotePath) == "":
		return errors.New("slotstore: remote path required")
	case strings.Contains(slot.RemotePath, ".."):
		return fmt.Errorf("slotstore: remote path %s invalid", slot.RemotePath)
	case !strings.HasPrefix(strings.TrimSpace(slot.RemotePath), "/slots/"):
		return fmt.Errorf("slotstore: remote path %s must be under /slots/", slot.RemotePath)
	default:
		if trimmed := strings.TrimSpace(slot.LocalPath); trimmed != "" {
			if !filepath.IsAbs(trimmed) {
				return fmt.Errorf("slotstore: local path %s must be absolute", trimmed)
			}
		}
		return nil
	}
}

func sortArtifacts(list []Artifact) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].UpdatedAt.Equal(list[j].UpdatedAt) {
			return list[i].ID > list[j].ID
		}
		return list[i].UpdatedAt.After(list[j].UpdatedAt)
	})
}

type slotDocument struct {
	Slot      Slot      `json:"slot"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type artifactEnvelope struct {
	Artifact Artifact `json:"artifact"`
}
