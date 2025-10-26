// This file isolates the GitLab signer etcd watch loop and event fan-out.
package gitlab

import (
	"time"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// watchRotations streams etcd updates and notifies rotation subscribers.
func (s *Signer) watchRotations() {
	defer s.wg.Done()

	for {
		watchChan := s.client.Watch(s.ctx, s.prefix, clientv3.WithPrefix())
		for {
			select {
			case <-s.ctx.Done():
				return
			case resp, ok := <-watchChan:
				if !ok || resp.Canceled {
					goto restart
				}
				if err := resp.Err(); err != nil {
					continue
				}
				for _, ev := range resp.Events {
					if ev.Type != mvccpb.PUT {
						continue
					}
					record, err := decodeSecretRecord(ev.Kv, s.prefix)
					if err != nil {
						continue
					}
					event := RotationEvent{
						SecretName: record.SecretName,
						Revision:   ev.Kv.ModRevision,
						UpdatedAt:  record.updatedAt(),
					}
					s.dispatch(event)
				}
			}
		}
	restart:
		if s.ctx.Err() != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// dispatch fan-outs a rotation event to every subscriber without blocking them.
func (s *Signer) dispatch(event RotationEvent) {
	s.watchersMu.RLock()
	listeners := make([]chan RotationEvent, 0, len(s.watchers))
	for _, ch := range s.watchers {
		listeners = append(listeners, ch)
	}
	s.watchersMu.RUnlock()

	for _, ch := range listeners {
		select {
		case ch <- event:
		default:
		}
	}
}
