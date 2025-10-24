package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// store wraps etcd access for Mods ticket metadata.
type store struct {
	client *clientv3.Client
	prefix string
	clock  func() time.Time
}

type ticketMetaDocument struct {
	TicketID   string            `json:"ticket_id"`
	Tenant     string            `json:"tenant,omitempty"`
	Submitter  string            `json:"submitter"`
	Repository string            `json:"repository"`
	Status     TicketState       `json:"status"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
}

type stageDocument struct {
	StageID      string            `json:"stage_id"`
	State        StageState        `json:"state"`
	Attempts     int               `json:"attempts"`
	MaxAttempts  int               `json:"max_attempts"`
	CurrentJobID string            `json:"current_job_id,omitempty"`
	Artifacts    map[string]string `json:"artifacts,omitempty"`
	LastError    string            `json:"last_error,omitempty"`
}

type ticketGraphDocument struct {
	Stages map[string]StageDefinition `json:"stages"`
}

// newStore initialises a Mods store wrapper.
func newStore(client *clientv3.Client, prefix string, clock func() time.Time) *store {
	return &store{
		client: client,
		prefix: prefix,
		clock:  clock,
	}
}

// createTicket persists ticket metadata, graph, and stage documents transactionally.
func (s *store) createTicket(ctx context.Context, spec TicketSpec, graph *stageGraph) (*TicketStatus, error) {
	metaKey := s.metaKey(spec.TicketID)
	graphKey := s.graphKey(spec.TicketID)
	now := s.clock().UTC().Format(time.RFC3339Nano)
	metaDoc := ticketMetaDocument{
		TicketID:   spec.TicketID,
		Tenant:     spec.Tenant,
		Submitter:  spec.Submitter,
		Repository: spec.Repository,
		Status:     TicketStatePending,
		Metadata:   cloneMap(spec.Metadata),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	graphDoc := ticketGraphDocument{
		Stages: make(map[string]StageDefinition, len(graph.stages)),
	}
	stageOps := make([]clientv3.Op, 0, len(graph.stages))
	for id, def := range graph.stages {
		clean := def
		if clean.MaxAttempts <= 0 {
			clean.MaxAttempts = 1
		}
		graphDoc.Stages[id] = clean
		stageDoc := stageDocument{
			StageID:     id,
			State:       StageStatePending,
			Attempts:    0,
			MaxAttempts: clean.MaxAttempts,
			Artifacts:   map[string]string{},
		}
		stageValue, err := json.Marshal(stageDoc)
		if err != nil {
			return nil, fmt.Errorf("marshal stage %s: %w", id, err)
		}
		stageOps = append(stageOps, clientv3.OpPut(s.stageKey(spec.TicketID, id), string(stageValue)))
	}
	metaValue, err := json.Marshal(metaDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal meta: %w", err)
	}
	graphValue, err := json.Marshal(graphDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal graph: %w", err)
	}
	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(metaKey), "=", 0),
		clientv3.Compare(clientv3.CreateRevision(graphKey), "=", 0),
	)
	ops := []clientv3.Op{
		clientv3.OpPut(metaKey, string(metaValue)),
		clientv3.OpPut(graphKey, string(graphValue)),
	}
	ops = append(ops, stageOps...)
	txn = txn.Then(ops...)
	resp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit create ticket txn: %w", err)
	}
	if !resp.Succeeded {
		return nil, fmt.Errorf("ticket %q already exists", spec.TicketID)
	}
	return s.ticketStatus(ctx, spec.TicketID)
}

// ticketStatus fetches ticket metadata and stage states.
func (s *store) ticketStatus(ctx context.Context, ticketID string) (*TicketStatus, error) {
	metaDoc, metaRev, err := s.readMeta(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	stageDocs, err := s.listStages(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	result := TicketStatus{
		TicketID:   metaDoc.TicketID,
		State:      metaDoc.Status,
		Tenant:     metaDoc.Tenant,
		Submitter:  metaDoc.Submitter,
		Repository: metaDoc.Repository,
		Metadata:   cloneMap(metaDoc.Metadata),
		Stages:     make(map[string]StageStatus, len(stageDocs)),
	}
	if metaDoc.CreatedAt != "" {
		if createdAt, parseErr := time.Parse(time.RFC3339Nano, metaDoc.CreatedAt); parseErr == nil {
			result.CreatedAt = createdAt
		}
	}
	if metaDoc.UpdatedAt != "" {
		if updatedAt, parseErr := time.Parse(time.RFC3339Nano, metaDoc.UpdatedAt); parseErr == nil {
			result.UpdatedAt = updatedAt
		}
	}
	for id, entry := range stageDocs {
		result.Stages[id] = StageStatus{
			StageID:      entry.doc.StageID,
			State:        entry.doc.State,
			Attempts:     entry.doc.Attempts,
			MaxAttempts:  entry.doc.MaxAttempts,
			CurrentJobID: entry.doc.CurrentJobID,
			Artifacts:    cloneMap(entry.doc.Artifacts),
			LastError:    entry.doc.LastError,
			Version:      entry.revision,
		}
	}
	if result.State == TicketStatePending {
		// Promote ticket to running when execution is in-flight.
		for _, stage := range result.Stages {
			if stage.State == StageStateRunning {
				result.State = TicketStateRunning
				break
			}
		}
	}
	_ = metaRev
	return &result, nil
}

// stageStatus fetches a single stage record.
func (s *store) stageStatus(ctx context.Context, ticketID, stageID string) (*StageStatus, error) {
	stageDoc, rev, err := s.readStage(ctx, ticketID, stageID)
	if err != nil {
		return nil, err
	}
	return &StageStatus{
		StageID:      stageDoc.StageID,
		State:        stageDoc.State,
		Attempts:     stageDoc.Attempts,
		MaxAttempts:  stageDoc.MaxAttempts,
		CurrentJobID: stageDoc.CurrentJobID,
		Artifacts:    cloneMap(stageDoc.Artifacts),
		LastError:    stageDoc.LastError,
		Version:      rev,
	}, nil
}

// readGraph loads the persisted stage graph.
func (s *store) readGraph(ctx context.Context, ticketID string) (*stageGraph, error) {
	resp, err := s.client.Get(ctx, s.graphKey(ticketID))
	if err != nil {
		return nil, fmt.Errorf("load graph: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrTicketNotFound
	}
	var doc ticketGraphDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return nil, fmt.Errorf("decode graph: %w", err)
	}
	stages := make([]StageDefinition, 0, len(doc.Stages))
	for _, stage := range doc.Stages {
		stages = append(stages, stage)
	}
	return buildStageGraph(stages)
}

// markStageQueued sets a stage state to queued and records the active job id.
func (s *store) markStageQueued(ctx context.Context, ticketID, stageID, jobID string) (*StageStatus, error) {
	doc, rev, err := s.readStage(ctx, ticketID, stageID)
	if err != nil {
		return nil, err
	}
	if doc.State == StageStateSucceeded || doc.State == StageStateFailed {
		return s.stageStatus(ctx, ticketID, stageID)
	}
	doc.State = StageStateQueued
	doc.CurrentJobID = jobID
	doc.LastError = ""
	return s.writeStage(ctx, ticketID, doc, rev)
}

// claimStage transitions a queued stage into running with optimistic concurrency.
func (s *store) claimStage(ctx context.Context, ticketID string, req ClaimStageRequest) (*StageStatus, error) {
	doc, rev, err := s.readStage(ctx, ticketID, req.StageID)
	if err != nil {
		return nil, err
	}
	if doc.State != StageStateQueued {
		return nil, ErrStageAlreadyClaimed
	}
	doc.State = StageStateRunning
	doc.Attempts++
	doc.CurrentJobID = req.JobID
	return s.writeStage(ctx, ticketID, doc, rev)
}

// completeStageSuccess finalises a stage with success and records artifacts.
func (s *store) completeStageSuccess(ctx context.Context, ticketID string, completion JobCompletion) (*StageStatus, error) {
	doc, rev, err := s.readStage(ctx, ticketID, completion.StageID)
	if err != nil {
		return nil, err
	}
	doc.State = StageStateSucceeded
	doc.CurrentJobID = ""
	doc.Artifacts = cloneMap(completion.Artifacts)
	doc.LastError = ""
	return s.writeStage(ctx, ticketID, doc, rev)
}

// completeStageFailure finalises a stage with failure metadata.
func (s *store) completeStageFailure(ctx context.Context, ticketID string, completion JobCompletion) (*StageStatus, error) {
	doc, rev, err := s.readStage(ctx, ticketID, completion.StageID)
	if err != nil {
		return nil, err
	}
	doc.State = StageStateFailed
	doc.CurrentJobID = ""
	if doc.Artifacts == nil {
		doc.Artifacts = map[string]string{}
	}
	doc.LastError = completion.Error
	return s.writeStage(ctx, ticketID, doc, rev)
}

// requeueStageFailure updates failure metadata while requeueing a retry.
func (s *store) requeueStageFailure(ctx context.Context, ticketID string, completion JobCompletion) (*StageStatus, error) {
	doc, rev, err := s.readStage(ctx, ticketID, completion.StageID)
	if err != nil {
		return nil, err
	}
	doc.State = StageStateQueued
	doc.CurrentJobID = ""
	doc.LastError = completion.Error
	return s.writeStage(ctx, ticketID, doc, rev)
}

// updateTicketState transitions the ticket meta state with CAS semantics.
func (s *store) updateTicketState(ctx context.Context, ticketID string, next TicketState) error {
	meta, rev, err := s.readMeta(ctx, ticketID)
	if err != nil {
		return err
	}
	if meta.Status == next {
		return nil
	}
	meta.Status = next
	meta.UpdatedAt = s.clock().UTC().Format(time.RFC3339Nano)
	return s.writeMeta(ctx, ticketID, meta, rev)
}

// readMeta fetches ticket meta and revision.
func (s *store) readMeta(ctx context.Context, ticketID string) (ticketMetaDocument, int64, error) {
	resp, err := s.client.Get(ctx, s.metaKey(ticketID))
	if err != nil {
		return ticketMetaDocument{}, 0, fmt.Errorf("load ticket meta: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return ticketMetaDocument{}, 0, ErrTicketNotFound
	}
	var doc ticketMetaDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return ticketMetaDocument{}, 0, fmt.Errorf("decode ticket meta: %w", err)
	}
	return doc, resp.Kvs[0].ModRevision, nil
}

// readStage fetches a stage document and revision.
func (s *store) readStage(ctx context.Context, ticketID, stageID string) (stageDocument, int64, error) {
	resp, err := s.client.Get(ctx, s.stageKey(ticketID, stageID))
	if err != nil {
		return stageDocument{}, 0, fmt.Errorf("load stage: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return stageDocument{}, 0, ErrStageNotFound
	}
	var doc stageDocument
	if err := json.Unmarshal(resp.Kvs[0].Value, &doc); err != nil {
		return stageDocument{}, 0, fmt.Errorf("decode stage: %w", err)
	}
	return doc, resp.Kvs[0].ModRevision, nil
}

type stageEntry struct {
	doc      stageDocument
	revision int64
}

// listStages returns all stage documents under a ticket.
func (s *store) listStages(ctx context.Context, ticketID string) (map[string]stageEntry, error) {
	resp, err := s.client.Get(ctx, s.stagesPrefix(ticketID), clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("list stages: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, ErrStageNotFound
	}
	result := make(map[string]stageEntry, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var doc stageDocument
		if err := json.Unmarshal(kv.Value, &doc); err != nil {
			return nil, fmt.Errorf("decode stage document: %w", err)
		}
		result[doc.StageID] = stageEntry{
			doc:      doc,
			revision: kv.ModRevision,
		}
	}
	return result, nil
}

// writeStage updates a stage document with optimistic concurrency.
func (s *store) writeStage(ctx context.Context, ticketID string, doc stageDocument, expectedRevision int64) (*StageStatus, error) {
	key := s.stageKey(ticketID, doc.StageID)
	payload, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal stage: %w", err)
	}
	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(key), "=", expectedRevision),
	).Then(
		clientv3.OpPut(key, string(payload)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit stage update: %w", err)
	}
	if !resp.Succeeded {
		return nil, ErrStageAlreadyClaimed
	}
	// Fetch updated stage to return accurate revision.
	return s.stageStatus(ctx, ticketID, doc.StageID)
}

// writeMeta updates ticket meta using compare-and-swap.
func (s *store) writeMeta(ctx context.Context, ticketID string, doc ticketMetaDocument, expectedRevision int64) error {
	key := s.metaKey(ticketID)
	payload, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal ticket meta: %w", err)
	}
	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(key), "=", expectedRevision),
	).Then(
		clientv3.OpPut(key, string(payload)),
	)
	resp, err := txn.Commit()
	if err != nil {
		return fmt.Errorf("commit ticket meta update: %w", err)
	}
	if !resp.Succeeded {
		return fmt.Errorf("ticket meta modified concurrently")
	}
	return nil
}

// metaKey computes the etcd key for ticket metadata.
func (s *store) metaKey(ticketID string) string {
	return path.Join(s.prefix, ticketID, "meta")
}

// graphKey computes the etcd key for the stored stage graph.
func (s *store) graphKey(ticketID string) string {
	return path.Join(s.prefix, ticketID, "graph")
}

// stagesPrefix returns the stage records prefix for a ticket.
func (s *store) stagesPrefix(ticketID string) string {
	return path.Join(s.prefix, ticketID, "stages") + "/"
}

// stageKey returns the etcd key for a specific stage record.
func (s *store) stageKey(ticketID, stageID string) string {
	return path.Join(s.prefix, ticketID, "stages", stageID)
}

// cloneMap shallow copies the map when provided.
func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
