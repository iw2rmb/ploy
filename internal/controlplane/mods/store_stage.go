package mods

import (
	"context"
	"encoding/json"
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
)

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
