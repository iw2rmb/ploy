package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EventKind identifies the type of Mods event emitted by the service watcher.
type EventKind string

const (
	// EventTicket signals a ticket-level state update.
	EventTicket EventKind = "ticket"
	// EventStage signals a per-stage lifecycle change.
	EventStage EventKind = "stage"
)

// Event surfaces ticket or stage updates observed in etcd.
type Event struct {
	Kind     EventKind
	Ticket   *TicketStatus
	Stage    *StageStatus
	Revision int64
}

// WatchTicket streams ticket metadata and stage updates from etcd.
func (s *Service) WatchTicket(ctx context.Context, ticketID string, sinceRevision int64) (<-chan Event, error) {
	ticketID = strings.TrimSpace(ticketID)
	if ticketID == "" {
		return nil, fmt.Errorf("mods: ticket id required")
	}
	if s == nil || s.store == nil || s.store.client == nil {
		return nil, fmt.Errorf("mods: watch unavailable")
	}

	prefix := path.Join(s.store.prefix, ticketID)
	opts := []clientv3.OpOption{clientv3.WithPrefix()}
	if sinceRevision > 0 {
		opts = append(opts, clientv3.WithRev(sinceRevision+1))
	}
	watch := s.store.client.Watch(ctx, prefix, opts...)

	out := make(chan Event, 32)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case resp, ok := <-watch:
				if !ok {
					return
				}
				if err := resp.Err(); err != nil {
					continue
				}
				for _, ev := range resp.Events {
					if ev == nil || ev.Kv == nil || ev.Type != mvccpb.PUT {
						continue
					}
					event, ok := s.eventFromKV(ctx, ticketID, ev.Kv)
					if !ok {
						continue
					}
					event.Revision = ev.Kv.ModRevision
					select {
					case out <- event:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, nil
}

func (s *Service) eventFromKV(ctx context.Context, ticketID string, kv *mvccpb.KeyValue) (Event, bool) {
	if kv == nil {
		return Event{}, false
	}
	key := string(kv.Key)
	base := path.Join(s.store.prefix, ticketID)
	trimmed := strings.TrimPrefix(key, base)
	trimmed = strings.TrimPrefix(trimmed, "/")
	switch {
	case trimmed == "meta":
		status, err := s.TicketStatus(ctx, ticketID)
		if err != nil {
			return Event{}, false
		}
		return Event{Kind: EventTicket, Ticket: status}, true
	case strings.HasPrefix(trimmed, "stages/"):
		var doc stageDocument
		if err := json.Unmarshal(kv.Value, &doc); err != nil {
			return Event{}, false
		}
		stage := &StageStatus{
			StageID:      doc.StageID,
			State:        doc.State,
			Attempts:     doc.Attempts,
			MaxAttempts:  doc.MaxAttempts,
			CurrentJobID: doc.CurrentJobID,
			Artifacts:    cloneMap(doc.Artifacts),
			LastError:    doc.LastError,
			Version:      kv.ModRevision,
		}
		return Event{Kind: EventStage, Stage: stage}, true
	default:
		return Event{}, false
	}
}

// Prefix returns the etcd prefix backing the Mods store.
func (s *Service) Prefix() string {
	if s == nil || s.store == nil {
		return ""
	}
	return s.store.prefix
}

// TicketStatusWithRevision returns the ticket summary alongside the etcd revision.
func (s *Service) TicketStatusWithRevision(ctx context.Context, ticketID string) (*TicketStatus, int64, error) {
	status, err := s.store.ticketStatus(ctx, ticketID)
	if err != nil {
		return nil, 0, err
	}
	_, rev, err := s.store.readMeta(ctx, ticketID)
	if err != nil {
		return nil, 0, err
	}
	return status, rev, nil
}
