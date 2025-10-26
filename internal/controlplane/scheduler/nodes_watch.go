package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// watchNodeStatus mirrors node health snapshots into running jobs.
func (s *Scheduler) watchNodeStatus() {
	defer s.wg.Done()

	watch := s.client.Watch(s.ctx, s.nodesPrefix+"/", clientv3.WithPrefix())
	for {
		select {
		case <-s.ctx.Done():
			return
		case resp, ok := <-watch:
			if !ok || resp.Canceled {
				return
			}
			if err := resp.Err(); err != nil {
				log.Printf("scheduler: node status watch error: %v", err)
				continue
			}
			for _, ev := range resp.Events {
				if ev.Type != mvccpb.PUT || ev.Kv == nil {
					continue
				}
				key := string(ev.Kv.Key)
				if !strings.HasSuffix(key, "/status") {
					continue
				}
				trimmed := strings.TrimPrefix(key, s.nodesPrefix)
				trimmed = strings.TrimPrefix(trimmed, "/")
				parts := strings.Split(trimmed, "/")
				if len(parts) < 2 {
					continue
				}
				nodeID := strings.TrimSpace(parts[0])
				if nodeID == "" {
					continue
				}
				status := decodeKVMap(ev.Kv.Value)
				if len(status) == 0 {
					continue
				}
				observed := snapshotTimestamp(status, s.now())
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.applyNodeStatus(ctx, nodeID, status, observed); err != nil {
					log.Printf("scheduler: apply node status: %v", err)
				}
				cancel()
			}
		}
	}
}

func (s *Scheduler) applyNodeStatus(ctx context.Context, nodeID string, status map[string]any, observed string) error {
	if len(status) == 0 {
		return nil
	}
	if strings.TrimSpace(observed) == "" {
		observed = encodeTime(s.now())
	}
	prefix := s.jobsPrefix + "/"
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("fetch jobs for node status: %w", err)
	}
	for _, kv := range resp.Kvs {
		record, err := decodeJobRecord(kv.Value)
		if err != nil {
			log.Printf("scheduler: decode job for node status: %v", err)
			continue
		}
		if record.State != JobStateRunning || record.ClaimedBy != nodeID {
			continue
		}
		if record.NodeSnapshot == nil {
			record.NodeSnapshot = &nodeSnapshotRecord{NodeID: nodeID}
		} else if strings.TrimSpace(record.NodeSnapshot.NodeID) == "" {
			record.NodeSnapshot.NodeID = nodeID
		}
		record.NodeSnapshot.Status = cloneAnyMap(status)
		record.NodeSnapshot.StatusAt = observed

		jobBytes, err := json.Marshal(record)
		if err != nil {
			log.Printf("scheduler: marshal job node status: %v", err)
			continue
		}
		jobKey := string(kv.Key)
		_, err = s.client.Txn(ctx).If(
			clientv3.Compare(clientv3.ModRevision(jobKey), "=", kv.ModRevision),
		).Then(
			clientv3.OpPut(jobKey, string(jobBytes)),
		).Commit()
		if err != nil {
			log.Printf("scheduler: update node status for job %s: %v", record.ID, err)
		}
	}
	return nil
}

// captureNodeSnapshot fetches node capacity and status to persist with a job mutation.
func (s *Scheduler) captureNodeSnapshot(ctx context.Context, nodeID string) *nodeSnapshotRecord {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil
	}

	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = s.ctx
	}

	snapshot := &nodeSnapshotRecord{NodeID: nodeID}

	capCtx, cancelCap := context.WithTimeout(baseCtx, 2*time.Second)
	capResp, err := s.client.Get(capCtx, path.Join(s.nodesPrefix, nodeID, "capacity"))
	cancelCap()
	if err != nil {
		log.Printf("scheduler: fetch node capacity: %v", err)
	} else if len(capResp.Kvs) > 0 {
		snapshot.Capacity = decodeKVMap(capResp.Kvs[0].Value)
		if len(snapshot.Capacity) > 0 {
			snapshot.CapacityAt = snapshotTimestamp(snapshot.Capacity, s.now())
		}
	}

	statusCtx, cancelStatus := context.WithTimeout(baseCtx, 2*time.Second)
	statusResp, err := s.client.Get(statusCtx, path.Join(s.nodesPrefix, nodeID, "status"))
	cancelStatus()
	if err != nil {
		log.Printf("scheduler: fetch node status: %v", err)
	} else if len(statusResp.Kvs) > 0 {
		snapshot.Status = decodeKVMap(statusResp.Kvs[0].Value)
		if len(snapshot.Status) > 0 {
			snapshot.StatusAt = snapshotTimestamp(snapshot.Status, s.now())
		}
	}

	if len(snapshot.Capacity) == 0 && len(snapshot.Status) == 0 {
		return nil
	}
	if snapshot.CapacityAt == "" && len(snapshot.Capacity) > 0 {
		snapshot.CapacityAt = encodeTime(s.now())
	}
	if snapshot.StatusAt == "" && len(snapshot.Status) > 0 {
		snapshot.StatusAt = encodeTime(s.now())
	}
	return snapshot
}
