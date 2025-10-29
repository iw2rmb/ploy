package httpserver

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/hydration"
)

// handleModsHydration routes hydration inspect/tune operations for a ticket.
func (s *controlPlaneServer) handleModsHydration(w http.ResponseWriter, r *http.Request, ticketID string) {
	if !s.ensureMods(w) {
		return
	}
	if !s.ensureHydration(w) {
		return
	}
	ticket := strings.TrimSpace(ticketID)
	if ticket == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleModsHydrationInspect(w, r, ticket)
	case http.MethodPatch:
		s.handleModsHydrationTune(w, r, ticket)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *controlPlaneServer) ensureHydration(w http.ResponseWriter) bool {
	if s.hydration == nil {
		http.Error(w, "hydration index unavailable", http.StatusNotImplemented)
		return false
	}
	return true
}

func (s *controlPlaneServer) handleModsHydrationInspect(w http.ResponseWriter, r *http.Request, ticket string) {
	entry, ok, err := s.hydration.LookupTicket(r.Context(), ticket)
	if err != nil {
		http.Error(w, fmt.Sprintf("hydrate: lookup ticket: %v", err), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	policy, ok, err := s.hydration.PolicyForTicket(r.Context(), ticket)
	if err != nil {
		http.Error(w, fmt.Sprintf("hydrate: inspect policy: %v", err), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	dto := hydrationPolicyDTO(policy, s.matchGlobalPolicy(r, entry))
	writeJSON(w, http.StatusOK, map[string]any{"hydration": dto})
}

func (s *controlPlaneServer) handleModsHydrationTune(w http.ResponseWriter, r *http.Request, ticket string) {
	var req hydrationTunePayload
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	update := hydration.PolicyUpdate{}
	if strings.TrimSpace(req.TTL) != "" {
		ttl := strings.TrimSpace(req.TTL)
		if _, err := time.ParseDuration(ttl); err != nil {
			http.Error(w, fmt.Sprintf("invalid ttl: %v", err), http.StatusBadRequest)
			return
		}
		update.TTL = &ttl
	}
	if req.ReplicationMin != nil {
		update.ReplicationMin = req.ReplicationMin
	}
	if req.ReplicationMax != nil {
		update.ReplicationMax = req.ReplicationMax
	}
	if req.Share != nil {
		update.Share = req.Share
	}

	if _, err := s.hydration.UpdateTicket(r.Context(), ticket, update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry, ok, err := s.hydration.LookupTicket(r.Context(), ticket)
	if err != nil {
		http.Error(w, fmt.Sprintf("hydrate: lookup updated ticket: %v", err), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	policy, _, err := s.hydration.PolicyForTicket(r.Context(), ticket)
	if err != nil {
		http.Error(w, fmt.Sprintf("hydrate: inspect updated policy: %v", err), http.StatusInternalServerError)
		return
	}
	dto := hydrationPolicyDTO(policy, s.matchGlobalPolicy(r, entry))
	writeJSON(w, http.StatusAccepted, map[string]any{"hydration": dto})
}

type hydrationTunePayload struct {
	TTL            string `json:"ttl"`
	ReplicationMin *int   `json:"replication_min"`
	ReplicationMax *int   `json:"replication_max"`
	Share          *bool  `json:"share"`
}

type hydrationPolicy struct {
	Ticket         string                 `json:"ticket"`
	SharedCID      string                 `json:"shared_cid"`
	TTL            string                 `json:"ttl"`
	ReplicationMin int                    `json:"replication_min"`
	ReplicationMax int                    `json:"replication_max"`
	Share          bool                   `json:"share"`
	ExpiresAt      string                 `json:"expires_at"`
	RepoURL        string                 `json:"repo_url"`
	Revision       string                 `json:"revision"`
	Candidates     []string               `json:"reuse_candidates"`
	Global         *hydrationPolicyGlobal `json:"global,omitempty"`
}

type hydrationPolicyGlobal struct {
	PolicyID           string                    `json:"policy_id"`
	PinnedBytes        hydrationPolicyByteUsage  `json:"pinned_bytes"`
	Snapshots          hydrationPolicyCountUsage `json:"snapshots"`
	Replicas           hydrationPolicyCountUsage `json:"replicas"`
	ActiveFingerprints []string                  `json:"active_fingerprints,omitempty"`
}

type hydrationPolicyByteUsage struct {
	Used int64 `json:"used"`
	Soft int64 `json:"soft,omitempty"`
	Hard int64 `json:"hard,omitempty"`
}

type hydrationPolicyCountUsage struct {
	Used int `json:"used"`
	Soft int `json:"soft,omitempty"`
	Hard int `json:"hard,omitempty"`
}

func hydrationPolicyDTO(policy hydration.Policy, global *hydrationPolicyGlobal) hydrationPolicy {
	dto := hydrationPolicy{
		Ticket:         policy.Ticket,
		SharedCID:      policy.SharedCID,
		TTL:            policy.TTL,
		ReplicationMin: policy.ReplicationMin,
		ReplicationMax: policy.ReplicationMax,
		Share:          policy.Share,
		RepoURL:        policy.RepoURL,
		Revision:       policy.Revision,
		Candidates:     append([]string(nil), policy.ReuseCandidates...),
		Global:         global,
	}
	if !policy.ExpiresAt.IsZero() {
		dto.ExpiresAt = policy.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	return dto
}

func (s *controlPlaneServer) matchGlobalPolicy(r *http.Request, entry hydration.SnapshotEntry) *hydrationPolicyGlobal {
	if s.hydrationPolicies == nil {
		return nil
	}
	policy, ok, err := s.hydrationPolicies.MatchSnapshot(r.Context(), entry)
	if err != nil || !ok {
		if err != nil {
			log.Printf("hydration: match global policy: %v", err)
		}
		return nil
	}
	dto := &hydrationPolicyGlobal{
		PolicyID: policy.ID,
		PinnedBytes: hydrationPolicyByteUsage{
			Used: policy.Usage.PinnedBytes,
			Soft: policy.Window.PinnedBytes.Soft,
			Hard: policy.Window.PinnedBytes.Hard,
		},
		Snapshots: hydrationPolicyCountUsage{
			Used: policy.Usage.SnapshotCount,
			Soft: policy.Window.Snapshots.Soft,
			Hard: policy.Window.Snapshots.Hard,
		},
		Replicas: hydrationPolicyCountUsage{
			Used: policy.Usage.ReplicaCount,
			Soft: policy.Window.Replicas.Soft,
			Hard: policy.Window.Replicas.Hard,
		},
		ActiveFingerprints: append([]string(nil), policy.Usage.ActiveFingerprints...),
	}
	return dto
}
