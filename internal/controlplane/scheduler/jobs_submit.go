package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// SubmitJob enqueues a new job and returns its persisted record.
func (s *Scheduler) SubmitJob(ctx context.Context, spec JobSpec) (*Job, error) {
	if strings.TrimSpace(spec.Ticket) == "" {
		return nil, errors.New("scheduler: ticket is required")
	}
	if strings.TrimSpace(spec.StepID) == "" {
		return nil, errors.New("scheduler: step id is required")
	}

	priority := strings.TrimSpace(spec.Priority)
	if priority == "" {
		priority = "default"
	}

	maxAttempts := spec.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	jobID := s.idGenerator()
	now := s.now().UTC()

	record := jobRecord{
		ID:             jobID,
		Ticket:         spec.Ticket,
		StepID:         spec.StepID,
		Priority:       priority,
		State:          JobStateQueued,
		CreatedAt:      encodeTime(now),
		EnqueuedAt:     encodeTime(now),
		RetryAttempt:   0,
		MaxAttempts:    maxAttempts,
		Metadata:       cloneMap(spec.Metadata),
		Artifacts:      map[string]string{},
		Bundles:        map[string]bundleRecord{},
		ExpiresAt:      "",
		NodeSnapshot:   nil,
		LeaseExpiresAt: "",
	}

	queue := queueEntry{
		JobID:        jobID,
		Ticket:       spec.Ticket,
		StepID:       spec.StepID,
		Priority:     priority,
		RetryAttempt: 0,
		MaxAttempts:  maxAttempts,
		EnqueuedAt:   encodeTime(now),
		Metadata:     cloneMap(spec.Metadata),
	}

	jobKey := s.jobKey(spec.Ticket, jobID)
	queueKey := s.queueKey(priority, jobID)

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("marshal job record: %w", err)
	}
	queueBytes, err := json.Marshal(queue)
	if err != nil {
		return nil, fmt.Errorf("marshal queue entry: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.CreateRevision(jobKey), "=", 0),
		clientv3.Compare(clientv3.CreateRevision(queueKey), "=", 0),
	).Then(
		clientv3.OpPut(jobKey, string(jobBytes)),
		clientv3.OpPut(queueKey, string(queueBytes)),
	)

	resp, err := txn.Commit()
	if err != nil {
		return nil, fmt.Errorf("commit job submit txn: %w", err)
	}
	if !resp.Succeeded {
		return nil, fmt.Errorf("scheduler: job already exists for ticket %s step %s", spec.Ticket, spec.StepID)
	}

	s.metrics.QueueEnqueued(priority)

	return record.toJob(), nil
}

// GetJob returns the current job record.
func (s *Scheduler) GetJob(ctx context.Context, ticket, jobID string) (*Job, error) {
	jobKey := s.jobKey(ticket, jobID)
	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return nil, fmt.Errorf("scheduler: get job: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil, fmt.Errorf("scheduler: job %s not found", jobID)
	}
	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return nil, fmt.Errorf("scheduler: decode job: %w", err)
	}
	return record.toJob(), nil
}

// ListJobs returns all jobs for a ticket ordered by etcd key order.
func (s *Scheduler) ListJobs(ctx context.Context, ticket string) ([]*Job, error) {
	if strings.TrimSpace(ticket) == "" {
		return nil, errors.New("scheduler: ticket is required")
	}
	prefix := path.Join(s.jobsPrefix, ticket, "jobs")
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("scheduler: list jobs: %w", err)
	}
	jobs := make([]*Job, 0, resp.Count)
	for _, kv := range resp.Kvs {
		record, err := decodeJobRecord(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("scheduler: decode job listing: %w", err)
		}
		jobs = append(jobs, record.toJob())
	}
	return jobs, nil
}

// RunningJobsForNode returns all jobs currently running on the provided node.
func (s *Scheduler) RunningJobsForNode(ctx context.Context, nodeID string) ([]*Job, error) {
	if strings.TrimSpace(nodeID) == "" {
		return nil, errors.New("scheduler: node id is required")
	}
	prefix := s.jobsPrefix + "/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("scheduler: list jobs for node: %w", err)
	}
	jobs := make([]*Job, 0, resp.Count)
	for _, kv := range resp.Kvs {
		record, err := decodeJobRecord(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("scheduler: decode job for node listing: %w", err)
		}
		if record.State != JobStateRunning {
			continue
		}
		if record.ClaimedBy != nodeID {
			continue
		}
		jobs = append(jobs, record.toJob())
	}
	return jobs, nil
}
