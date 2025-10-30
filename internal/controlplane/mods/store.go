package mods

import (
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
