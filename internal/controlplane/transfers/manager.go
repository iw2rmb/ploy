package transfers

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

	gonanoid "github.com/matoous/go-nanoid/v2"

	controlplaneartifacts "github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

// Kind enumerates supported transfer kinds.
type Kind string

const (
	KindRepo   Kind = "repo"
	KindLogs   Kind = "logs"
	KindReport Kind = "report"
	// KindRegistryBlob represents registry blob staging slots.
	KindRegistryBlob Kind = "registry-blob"
)

// SlotState represents lifecycle states for a slot.
type SlotState string

const (
	SlotPending   SlotState = "pending"
	SlotCommitted SlotState = "committed"
	SlotAborted   SlotState = "aborted"
)

// Slot describes a reserved transfer slot.
type Slot struct {
	ID         string    `json:"slot_id"`
	Kind       Kind      `json:"kind"`
	JobID      string    `json:"job_id"`
	Stage      string    `json:"stage,omitempty"`
	NodeID     string    `json:"node_id"`
	RemotePath string    `json:"remote_path"`
	LocalPath  string    `json:"local_path,omitempty"`
	MaxSize    int64     `json:"max_size"`
	ExpiresAt  time.Time `json:"expires_at"`
	Digest     string    `json:"digest,omitempty"`
	State      SlotState `json:"state"`
}

// Artifact captures a committed upload.
type Artifact struct {
	ID         string    `json:"artifact_id"`
	Kind       Kind      `json:"kind"`
	JobID      string    `json:"job_id"`
	Stage      string    `json:"stage,omitempty"`
	NodeID     string    `json:"node_id"`
	RemotePath string    `json:"remote_path"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	CID        string    `json:"cid"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ArtifactStore persists metadata for committed transfers.
type ArtifactStore interface {
	Create(ctx context.Context, meta controlplaneartifacts.Metadata) (controlplaneartifacts.Metadata, error)
}

type artifactPublisher interface {
	Add(ctx context.Context, req workflowartifacts.AddRequest) (workflowartifacts.AddResponse, error)
}

// Options configure the Manager.
type Options struct {
	BaseDir   string
	MaxSize   int64
	TTL       time.Duration
	Now       func() time.Time
	SlotStore *SlotStore
	Store     ArtifactStore
	Publisher artifactPublisher
}

// Manager tracks transfer slots in-memory for the control plane.

type Manager struct {
	mu        sync.Mutex
	baseDir   string
	maxSize   int64
	ttl       time.Duration
	now       func() time.Time
	slots     map[string]*Slot
	artifacts map[string][]Artifact
	slotStore *SlotStore
	store     ArtifactStore
	publisher artifactPublisher
}

// NewManager returns a slot manager with sane defaults.
func NewManager(opts Options) *Manager {
	base := opts.BaseDir
	if strings.TrimSpace(base) == "" {
		base = "/var/lib/ploy/ssh-artifacts"
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	maxSize := opts.MaxSize
	if maxSize <= 0 {
		maxSize = 10 << 30 // 10 GiB
	}
	clock := opts.Now
	if clock == nil {
		clock = time.Now
	}
	m := &Manager{
		baseDir:   base,
		maxSize:   maxSize,
		ttl:       ttl,
		now:       clock,
		slotStore: opts.SlotStore,
		store:     opts.Store,
		publisher: opts.Publisher,
		slots:     make(map[string]*Slot),
		artifacts: make(map[string][]Artifact),
	}
	return m
}

// CreateUploadSlot reserves an upload slot targeting the supplied node.
func (m *Manager) CreateUploadSlot(kind Kind, jobID, stageID, nodeID string, sizeHint int64) (Slot, error) {
	return m.createSlot(kind, jobID, stageID, nodeID, sizeHint)
}

// CreateDownloadSlot reserves a download slot for an existing artifact.
func (m *Manager) CreateDownloadSlot(jobID, artifactID string, kind Kind) (Slot, Artifact, error) {
	if m.slotStore != nil {
		artifacts, err := m.slotStore.JobArtifacts(context.Background(), strings.TrimSpace(jobID))
		if err != nil {
			return Slot{}, Artifact{}, err
		}
		artifact, err := selectArtifactFromList(artifacts, jobID, artifactID, kind)
		if err != nil {
			return Slot{}, Artifact{}, err
		}
		slot := Slot{
			ID:         m.generateSlotID(),
			Kind:       artifact.Kind,
			JobID:      artifact.JobID,
			NodeID:     artifact.NodeID,
			RemotePath: artifact.RemotePath,
			MaxSize:    artifact.Size,
			ExpiresAt:  m.now().Add(m.ttl),
			State:      SlotPending,
			Digest:     artifact.Digest,
		}
		slot.LocalPath = m.localPathForRemote(slot.RemotePath)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := m.slotStore.CreateSlot(ctx, slot); err != nil {
			return Slot{}, Artifact{}, err
		}
		return slot, artifact, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	artifact, err := selectArtifactFromList(m.artifacts[strings.TrimSpace(jobID)], jobID, artifactID, kind)
	if err != nil {
		return Slot{}, Artifact{}, err
	}
	slotID := m.generateSlotID()
	slot := &Slot{
		ID:         slotID,
		Kind:       artifact.Kind,
		JobID:      artifact.JobID,
		NodeID:     artifact.NodeID,
		RemotePath: artifact.RemotePath,
		MaxSize:    artifact.Size,
		ExpiresAt:  m.now().Add(m.ttl),
		State:      SlotPending,
		Digest:     artifact.Digest,
	}
	slot.LocalPath = m.localPathForRemote(slot.RemotePath)
	m.slots[slotID] = slot
	return *slot, artifact, nil
}

// Commit finalises a slot.
func (m *Manager) Commit(ctx context.Context, slotID string, size int64, digest string) (Slot, error) {
	if m.slotStore != nil {
		record, err := m.slotStore.GetSlot(ctx, slotID)
		if err != nil {
			return Slot{}, err
		}
		if record.Slot.State != SlotPending {
			return record.Slot, nil
		}
		slotCopy := record.Slot
		var storedArtifact *Artifact
		if m.publisher != nil && m.store != nil && slotCopy.Kind != "" {
			artifact, err := m.publishAndStore(ctx, slotCopy, size, digest)
			if err != nil {
				return Slot{}, err
			}
			storedArtifact = &artifact
		}
		updated, err := m.slotStore.UpdateSlotState(ctx, slotID, record.Revision, SlotCommitted, digest)
		if err != nil {
			return Slot{}, err
		}
		slotCopy = updated.Slot
		if slotCopy.Kind != "" {
			var art Artifact
			if storedArtifact != nil {
				art = *storedArtifact
			} else {
				art = Artifact{
					ID:         slotCopy.ID,
					Kind:       slotCopy.Kind,
					JobID:      slotCopy.JobID,
					Stage:      slotCopy.Stage,
					NodeID:     slotCopy.NodeID,
					RemotePath: slotCopy.RemotePath,
					Size:       size,
					Digest:     slotCopy.Digest,
					UpdatedAt:  m.now(),
				}
			}
			if err := m.slotStore.RecordArtifact(ctx, art); err != nil {
				return Slot{}, err
			}
		}
		return slotCopy, nil
	}

	m.mu.Lock()
	slot, ok := m.slots[slotID]
	if !ok {
		m.mu.Unlock()
		return Slot{}, fmt.Errorf("unknown slot %s", slotID)
	}
	if slot.State != SlotPending {
		result := *slot
		m.mu.Unlock()
		return result, nil
	}
	slot.State = SlotCommitted
	slot.Digest = digest
	slotCopy := *slot
	m.mu.Unlock()

	var recorded *Artifact
	if m.publisher != nil && m.store != nil && slotCopy.Kind != "" {
		artifact, err := m.publishAndStore(ctx, slotCopy, size, digest)
		if err != nil {
			return Slot{}, err
		}
		recorded = &artifact
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if recorded != nil {
		m.artifacts[slotCopy.JobID] = append([]Artifact{*recorded}, m.artifacts[slotCopy.JobID]...)
	} else if slotCopy.Kind != "" {
		art := Artifact{
			ID:         slotCopy.ID,
			Kind:       slotCopy.Kind,
			JobID:      slotCopy.JobID,
			Stage:      slotCopy.Stage,
			NodeID:     slotCopy.NodeID,
			RemotePath: slotCopy.RemotePath,
			Size:       size,
			Digest:     digest,
			UpdatedAt:  m.now(),
		}
		m.artifacts[slotCopy.JobID] = append([]Artifact{art}, m.artifacts[slotCopy.JobID]...)
	}
	return *slot, nil
}

// Abort releases a slot without committing it.
func (m *Manager) Abort(slotID string) {
	if m.slotStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		record, err := m.slotStore.GetSlot(ctx, slotID)
		if err == nil && record.Slot.State == SlotPending {
			_, _ = m.slotStore.UpdateSlotState(ctx, slotID, record.Revision, SlotAborted, record.Slot.Digest)
		}
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if slot, ok := m.slots[slotID]; ok {
		slot.State = SlotAborted
	}
}

// Slot returns a copy of the slot metadata for the given identifier.
func (m *Manager) Slot(slotID string) (Slot, error) {
	id := strings.TrimSpace(slotID)
	if id == "" {
		return Slot{}, errors.New("slot id required")
	}
	if m.slotStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		record, err := m.slotStore.GetSlot(ctx, id)
		if err != nil {
			return Slot{}, err
		}
		return record.Slot, nil
	}
	m.mu.Lock()
	slot, ok := m.slots[id]
	m.mu.Unlock()
	if !ok {
		return Slot{}, fmt.Errorf("unknown slot %s", id)
	}
	return *slot, nil
}

// LoadSlotPayload reads and validates the payload staged for the supplied slot.
func (m *Manager) LoadSlotPayload(slotID string, declaredSize int64, declaredDigest string) (Slot, []byte, string, error) {
	slot, err := m.Slot(slotID)
	if err != nil {
		return Slot{}, nil, "", err
	}
	data, digest, err := m.readSlotPayload(slot, declaredSize, declaredDigest)
	if err != nil {
		return Slot{}, nil, "", err
	}
	return slot, data, digest, nil
}

func (m *Manager) createSlot(kind Kind, jobID, stageID, nodeID string, sizeHint int64) (Slot, error) {
	if strings.TrimSpace(jobID) == "" {
		return Slot{}, errors.New("job id required")
	}
	if strings.TrimSpace(nodeID) == "" {
		return Slot{}, errors.New("node id required")
	}
	slotID := m.generateSlotID()
	slotDir := filepath.Join(m.baseDir, "slots", slotID)
	localPath := filepath.Join(slotDir, "payload")
	remotePath := filepath.ToSlash(filepath.Join("/slots", slotID, "payload"))
	slot := &Slot{
		ID:         slotID,
		Kind:       kind,
		JobID:      strings.TrimSpace(jobID),
		Stage:      strings.TrimSpace(stageID),
		NodeID:     strings.TrimSpace(nodeID),
		RemotePath: remotePath,
		LocalPath:  localPath,
		MaxSize:    firstNonZero(sizeHint, m.maxSize),
		ExpiresAt:  m.now().Add(m.ttl),
		State:      SlotPending,
	}
	if m.slotStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := m.slotStore.CreateSlot(ctx, *slot); err != nil {
			return Slot{}, err
		}
		return *slot, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slots[slotID] = slot
	return *slot, nil
}

func selectArtifactFromList(list []Artifact, jobID, artifactID string, kind Kind) (Artifact, error) {
	if len(list) == 0 {
		return Artifact{}, fmt.Errorf("no artifacts recorded for %s", strings.TrimSpace(jobID))
	}
	trimmedID := strings.TrimSpace(artifactID)
	if trimmedID != "" {
		for _, art := range list {
			if art.ID == trimmedID {
				return art, nil
			}
		}
		return Artifact{}, fmt.Errorf("artifact %s not found for job %s", trimmedID, strings.TrimSpace(jobID))
	}
	if kind != "" {
		for _, art := range list {
			if art.Kind == kind {
				return art, nil
			}
		}
		return Artifact{}, fmt.Errorf("no artifacts of kind %s for job %s", kind, strings.TrimSpace(jobID))
	}
	return list[0], nil
}

func (m *Manager) generateSlotID() string {
	id, err := gonanoid.Generate("abcdefghijklmnopqrstuvwxyz0123456789", 12)
	if err != nil {
		return fmt.Sprintf("slot-%d", time.Now().UnixNano())
	}
	return "slot-" + id
}

func firstNonZero(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func (m *Manager) localPathForRemote(remote string) string {
	trimmed := strings.TrimSpace(remote)
	if trimmed == "" {
		return ""
	}
	clean := filepath.Clean(trimmed)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" {
		return ""
	}
	return filepath.Join(m.baseDir, clean)
}

func (m *Manager) publishAndStore(ctx context.Context, slot Slot, declaredSize int64, declaredDigest string) (Artifact, error) {
	data, computedDigest, err := m.readSlotPayload(slot, declaredSize, declaredDigest)
	if err != nil {
		return Artifact{}, err
	}
	actualSize := int64(len(data))
	name := filepath.Base(slot.RemotePath)
	addReq := workflowartifacts.AddRequest{
		Name:    name,
		Payload: data,
	}
	result, err := m.publisher.Add(ctx, addReq)
	if err != nil {
		return Artifact{}, fmt.Errorf("publish artifact: %w", err)
	}
	meta := controlplaneartifacts.Metadata{
		ID:                   slot.ID,
		SlotID:               slot.ID,
		JobID:                slot.JobID,
		Stage:                slot.Stage,
		Kind:                 string(slot.Kind),
		NodeID:               slot.NodeID,
		CID:                  strings.TrimSpace(result.CID),
		Digest:               strings.TrimSpace(result.Digest),
		Size:                 result.Size,
		Name:                 strings.TrimSpace(result.Name),
		Source:               "ssh-slot",
		ReplicationFactorMin: result.ReplicationFactorMin,
		ReplicationFactorMax: result.ReplicationFactorMax,
	}
	if meta.Digest == "" {
		meta.Digest = computedDigest
	}
	if meta.Size == 0 {
		meta.Size = actualSize
	}
	if meta.Name == "" {
		meta.Name = name
	}
	created, err := m.store.Create(ctx, meta)
	if err != nil {
		return Artifact{}, fmt.Errorf("persist artifact metadata: %w", err)
	}
	if strings.TrimSpace(slot.LocalPath) != "" {
		_ = os.RemoveAll(filepath.Dir(slot.LocalPath))
	} else {
		_ = os.RemoveAll(filepath.Dir(slot.RemotePath))
	}
	return Artifact{
		ID:         slot.ID,
		Kind:       slot.Kind,
		JobID:      slot.JobID,
		Stage:      slot.Stage,
		NodeID:     slot.NodeID,
		RemotePath: slot.RemotePath,
		Size:       created.Size,
		Digest:     created.Digest,
		CID:        created.CID,
		UpdatedAt:  created.UpdatedAt,
	}, nil
}

func (m *Manager) readSlotPayload(slot Slot, declaredSize int64, declaredDigest string) ([]byte, string, error) {
	path := strings.TrimSpace(slot.LocalPath)
	if path == "" {
		path = strings.TrimSpace(slot.RemotePath)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read slot payload: %w", err)
	}
	actualSize := int64(len(data))
	if declaredSize > 0 && actualSize != declaredSize {
		return nil, "", fmt.Errorf("payload size mismatch: want %d got %d", declaredSize, actualSize)
	}
	checksum := sha256.Sum256(data)
	computedDigest := "sha256:" + hex.EncodeToString(checksum[:])
	trimmedDigest := strings.TrimSpace(declaredDigest)
	if trimmedDigest != "" && !strings.EqualFold(trimmedDigest, computedDigest) {
		return nil, "", fmt.Errorf("payload digest mismatch")
	}
	return data, computedDigest, nil
}
