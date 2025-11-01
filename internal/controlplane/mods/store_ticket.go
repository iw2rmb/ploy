package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// createTicket persists ticket metadata, graph, and stage documents transactionally.
func (s *store) createTicket(ctx context.Context, spec TicketSpec, graph *stageGraph) (*TicketStatus, error) {
	metaKey := s.metaKey(spec.TicketID)
	graphKey := s.graphKey(spec.TicketID)
	now := s.clock().UTC().Format(time.RFC3339Nano)
	metaDoc := ticketMetaDocument{
		TicketID:   spec.TicketID,
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
