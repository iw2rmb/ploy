package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// watchGCMarkers keeps job expiry metadata aligned with GC markers.
func (s *Scheduler) watchGCMarkers() {
	defer s.wg.Done()

	watch := s.client.Watch(s.ctx, s.gcPrefix, clientv3.WithPrefix())
	for {
		select {
		case <-s.ctx.Done():
			return
		case resp, ok := <-watch:
			if !ok || resp.Canceled {
				return
			}
			if err := resp.Err(); err != nil {
				log.Printf("scheduler: gc watch error: %v", err)
				continue
			}
			for _, ev := range resp.Events {
				if ev.Type != mvccpb.PUT || ev.Kv == nil {
					continue
				}
				var marker gcEntry
				if err := json.Unmarshal(ev.Kv.Value, &marker); err != nil {
					log.Printf("scheduler: decode gc entry: %v", err)
					continue
				}
				if strings.TrimSpace(marker.JobID) == "" || strings.TrimSpace(marker.Ticket) == "" {
					continue
				}
				ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
				if err := s.syncJobExpiry(ctx, marker); err != nil {
					log.Printf("scheduler: sync job expiry: %v", err)
				}
				cancel()
			}
		}
	}
}

func (s *Scheduler) syncJobExpiry(ctx context.Context, marker gcEntry) error {
	ticket := strings.TrimSpace(marker.Ticket)
	jobID := strings.TrimSpace(marker.JobID)
	if ticket == "" || jobID == "" {
		return nil
	}
	jobKey := s.jobKey(ticket, jobID)
	resp, err := s.client.Get(ctx, jobKey)
	if err != nil {
		return fmt.Errorf("scheduler: fetch job for gc marker: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return nil
	}
	record, err := decodeJobRecord(resp.Kvs[0].Value)
	if err != nil {
		return fmt.Errorf("scheduler: decode job for gc marker: %w", err)
	}

	expiry := strings.TrimSpace(marker.ExpiresAt)
	changed := false
	if expiry != "" {
		if record.ExpiresAt != expiry {
			record.ExpiresAt = expiry
			changed = true
		}
		if record.Retention != nil && record.Retention.ExpiresAt != expiry {
			record.Retention.ExpiresAt = expiry
			changed = true
		}
	} else {
		if record.ExpiresAt != "" {
			record.ExpiresAt = ""
			changed = true
		}
		if record.Retention != nil && record.Retention.ExpiresAt != "" {
			record.Retention.ExpiresAt = ""
			changed = true
		}
	}
	if !changed {
		return nil
	}

	jobBytes, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal job for gc sync: %w", err)
	}
	_, err = s.client.Txn(ctx).If(
		clientv3.Compare(clientv3.ModRevision(jobKey), "=", resp.Kvs[0].ModRevision),
	).Then(
		clientv3.OpPut(jobKey, string(jobBytes)),
	).Commit()
	return err
}
