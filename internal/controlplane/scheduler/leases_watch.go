package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// watchLeases monitors lease prefix deletions to requeue expired jobs.
func (s *Scheduler) watchLeases() {
	defer s.wg.Done()

	watch := s.client.Watch(s.ctx, s.leasesPrefix, clientv3.WithPrefix(), clientv3.WithPrevKV())
	for {
		select {
		case <-s.ctx.Done():
			return
		case resp, ok := <-watch:
			if !ok || resp.Canceled {
				return
			}
			if resp.Err() != nil {
				log.Printf("scheduler: lease watch error: %v", resp.Err())
				continue
			}
			for _, ev := range resp.Events {
				if ev.Type != mvccpb.DELETE || ev.PrevKv == nil {
					continue
				}
				var lease leaseEntry
				if err := json.Unmarshal(ev.PrevKv.Value, &lease); err != nil {
					log.Printf("scheduler: decode lease entry: %v", err)
					continue
				}
				if lease.JobID == "" || lease.Ticket == "" {
					continue
				}
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.handleLeaseExpiry(ctx, lease, clientv3.LeaseID(ev.PrevKv.Lease)); err != nil {
					log.Printf("scheduler: handle lease expiry: %v", err)
				}
				cancel()
			}
		}
	}
}

func (s *Scheduler) handleLeaseExpiry(ctx context.Context, lease leaseEntry, leaseID clientv3.LeaseID) error {
	jobKey := s.jobKey(lease.Ticket, lease.JobID)
	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return fmt.Errorf("fetch job for lease expiry: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil
	}
	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return fmt.Errorf("decode job for lease expiry: %w", err)
	}
	if record.State != JobStateRunning {
		return nil
	}
	if clientv3.LeaseID(record.LeaseID) != leaseID {
		return nil
	}

	now := s.now().UTC()
	record.ClaimedBy = ""
	record.ClaimedAt = ""
	record.LeaseID = 0
	record.LeaseExpiresAt = ""
	record.ExpiresAt = ""
	record.NodeSnapshot = nil
	record.RetryAttempt++
	record.Gate = nil

	var (
		queuePut clientv3.Op
		errMsg   *JobError
		newState JobState
		requeued bool
	)

	if record.RetryAttempt >= record.MaxAttempts {
		newState = JobStateFailed
		errMsg = &JobError{
			Reason:  "lease_expired",
			Message: "worker lease expired without heartbeat",
		}
		record.CompletedAt = encodeTime(now)
	} else {
		newState = JobStateQueued
		queue := queueEntry{
			JobID:        record.ID,
			Ticket:       record.Ticket,
			StepID:       record.StepID,
			Priority:     lease.Priority,
			RetryAttempt: record.RetryAttempt,
			MaxAttempts:  record.MaxAttempts,
			EnqueuedAt:   encodeTime(now),
			Metadata:     cloneMap(record.Metadata),
		}
		queueBytes, err := json.Marshal(queue)
		if err != nil {
			return fmt.Errorf("marshal queue requeue: %w", err)
		}
		queuePut = clientv3.OpPut(s.queueKey(lease.Priority, record.ID), string(queueBytes))
		record.EnqueuedAt = queue.EnqueuedAt
		requeued = true
	}

	record.State = newState
	if errMsg != nil {
		record.Error = errMsg
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal job requeue: %w", err)
	}

	txn := s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", resp.Kvs[0].ModRevision),
	)

	ops := []clientv3.Op{clientv3.OpPut(jobKey, string(jobBytes))}
	if queuePut.KeyBytes() != nil {
		ops = append(ops, queuePut)
	}

	txn = txn.Then(ops...)

	respTxn, err := txn.Commit()
	if err != nil {
		return err
	}
	_ = respTxn
	if requeued {
		s.metrics.QueueEnqueued(lease.Priority)
	}
	s.metrics.ObserveJobRetry(lease.Priority, jobRetryReasonLeaseExpired)
	return nil
}
