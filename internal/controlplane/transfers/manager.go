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
	MaxSize    int64     `json:"max_size"`
	ExpiresAt  time.Time `json:"expires_at"`
	Digest     string    `json:"digest,omitempty"`
	State      SlotState `json:"state"`
}

// Artifact captures a committed upload.
type Artifact struct {
	ID         string
	Kind       Kind
	JobID      string
	Stage      string
	NodeID     string
	RemotePath string
	Size       int64
	Digest     string
	CID        string
	UpdatedAt  time.Time
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
	return &Manager{
		baseDir:   base,
		maxSize:   maxSize,
		ttl:       ttl,
		now:       clock,
		slots:     make(map[string]*Slot),
		artifacts: make(map[string][]Artifact),
		store:     opts.Store,
		publisher: opts.Publisher,
	}
}

// CreateUploadSlot reserves an upload slot targeting the supplied node.
func (m *Manager) CreateUploadSlot(kind Kind, jobID, stageID, nodeID string, sizeHint int64) (Slot, error) {
	return m.createSlot(kind, jobID, stageID, nodeID, sizeHint)
}

// CreateDownloadSlot reserves a download slot for an existing artifact.
func (m *Manager) CreateDownloadSlot(jobID, artifactID string, kind Kind) (Slot, Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	artifact, err := m.selectArtifact(jobID, artifactID, kind)
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
	m.slots[slotID] = slot
	return *slot, artifact, nil
}

// Commit finalises a slot.
func (m *Manager) Commit(ctx context.Context, slotID string, size int64, digest string) (Slot, error) {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if slot, ok := m.slots[slotID]; ok {
		slot.State = SlotAborted
	}
}

func (m *Manager) createSlot(kind Kind, jobID, stageID, nodeID string, sizeHint int64) (Slot, error) {
	if strings.TrimSpace(jobID) == "" {
		return Slot{}, errors.New("job id required")
	}
	if strings.TrimSpace(nodeID) == "" {
		return Slot{}, errors.New("node id required")
	}
	slotID := m.generateSlotID()
	remotePath := filepath.Join(m.baseDir, "slots", slotID, "payload")
	slot := &Slot{
		ID:         slotID,
		Kind:       kind,
		JobID:      strings.TrimSpace(jobID),
		Stage:      strings.TrimSpace(stageID),
		NodeID:     strings.TrimSpace(nodeID),
		RemotePath: remotePath,
		MaxSize:    firstNonZero(sizeHint, m.maxSize),
		ExpiresAt:  m.now().Add(m.ttl),
		State:      SlotPending,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.slots[slotID] = slot
	return *slot, nil
}

func (m *Manager) selectArtifact(jobID, artifactID string, kind Kind) (Artifact, error) {
	list := m.artifacts[strings.TrimSpace(jobID)]
	if len(list) == 0 {
		return Artifact{}, fmt.Errorf("no artifacts recorded for %s", jobID)
	}
	if strings.TrimSpace(artifactID) != "" {
		for _, art := range list {
			if art.ID == artifactID {
				return art, nil
			}
		}
		return Artifact{}, fmt.Errorf("artifact %s not found for job %s", artifactID, jobID)
	}
	if kind != "" {
		for _, art := range list {
			if art.Kind == kind {
				return art, nil
			}
		}
		return Artifact{}, fmt.Errorf("no artifacts of kind %s for job %s", kind, jobID)
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

func (m *Manager) publishAndStore(ctx context.Context, slot Slot, declaredSize int64, declaredDigest string) (Artifact, error) {
	data, err := os.ReadFile(slot.RemotePath)
	if err != nil {
		return Artifact{}, fmt.Errorf("read slot payload: %w", err)
	}
	actualSize := int64(len(data))
	if declaredSize > 0 && actualSize != declaredSize {
		return Artifact{}, fmt.Errorf("payload size mismatch: want %d got %d", declaredSize, actualSize)
	}
	checksum := sha256.Sum256(data)
	computedDigest := "sha256:" + hex.EncodeToString(checksum[:])
	trimmedDigest := strings.TrimSpace(declaredDigest)
	if trimmedDigest != "" && !strings.EqualFold(trimmedDigest, computedDigest) {
		return Artifact{}, fmt.Errorf("payload digest mismatch")
	}
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
	_ = os.RemoveAll(filepath.Dir(slot.RemotePath))
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
