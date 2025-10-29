package hydration

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type stubClient struct {
	policy Policy
	err    error
}

func (s *stubClient) Inspect(ctx context.Context, ticket string) (Policy, error) {
	if s.err != nil {
		return Policy{}, s.err
	}
	return s.policy, nil
}

func (s *stubClient) Tune(ctx context.Context, ticket string, req TuneRequest) (Policy, error) {
	if s.err != nil {
		return Policy{}, s.err
	}
	policy := s.policy
	if req.TTL != "" {
		policy.TTL = req.TTL
	}
	if req.ReplicationMin != nil {
		policy.ReplicationMin = *req.ReplicationMin
	}
	if req.ReplicationMax != nil {
		policy.ReplicationMax = *req.ReplicationMax
	}
	if req.Share != nil {
		policy.Share = *req.Share
	}
	return policy, nil
}

// TestInspectCommandPrintsPolicy ensures inspect formatting renders policy details.
func TestInspectCommandPrintsPolicy(t *testing.T) {
	client := &stubClient{
		policy: Policy{
			Ticket:          "mod-1",
			SharedCID:       "bafy-123",
			TTL:             "24h",
			ReplicationMin:  2,
			ReplicationMax:  3,
			Share:           true,
			ExpiresAt:       time.Date(2025, 10, 29, 0, 0, 0, 0, time.UTC),
			RepoURL:         "https://git.example.com/repo.git",
			Revision:        "deadbeef",
			ReuseCandidates: []string{"mod-1", "mod-2"},
			Global: &GlobalPolicy{
				PolicyID: "default",
				PinnedBytes: ByteUsage{
					Used: 2048,
					Soft: 0,
					Hard: 52428800,
				},
				Snapshots:          CountUsage{Used: 2, Soft: 1, Hard: 10},
				Replicas:           CountUsage{Used: 4, Soft: 0, Hard: 6},
				ActiveFingerprints: []string{"fp1", "fp2"},
			},
		},
	}

	buf := &bytes.Buffer{}
	cmd := InspectCommand{
		Ticket: "mod-1",
		Client: client,
		Output: buf,
	}
	if err := cmd.Run(context.Background()); err != nil {
		t.Fatalf("inspect run: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "bafy-123") || !strings.Contains(out, "TTL: 24h") {
		t.Fatalf("inspect output missing expected fields: %q", out)
	}
	if !strings.Contains(out, "Global Policy: default") {
		t.Fatalf("expected global policy output, got %q", out)
	}
	if !strings.Contains(out, "Pinned Bytes: used=2048") {
		t.Fatalf("expected pinned bytes usage, got %q", out)
	}
	if !strings.Contains(out, "Active Fingerprints: fp1, fp2") {
		t.Fatalf("expected fingerprints output, got %q", out)
	}
}

// TestTuneCommandValidatesFlags ensures tune command surfaces client errors.
func TestTuneCommandValidatesFlags(t *testing.T) {
	client := &stubClient{err: errors.New("control plane unavailable")}
	cmd := TuneCommand{
		Ticket: "mod-err",
		Client: client,
	}
	err := cmd.Run(context.Background(), TuneRequest{})
	if err == nil {
		t.Fatalf("expected tune error when client returns failure")
	}
	if !strings.Contains(err.Error(), "control plane unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}
