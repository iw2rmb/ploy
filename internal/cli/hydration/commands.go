package hydration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Client defines the control-plane hydration API contract required by the CLI.
type Client interface {
	Inspect(ctx context.Context, ticket string) (Policy, error)
	Tune(ctx context.Context, ticket string, req TuneRequest) (Policy, error)
}

// Policy summarises hydration retention metadata for CLI rendering.
type Policy struct {
	Ticket          string
	SharedCID       string
	TTL             string
	ReplicationMin  int
	ReplicationMax  int
	Share           bool
	ExpiresAt       time.Time
	RepoURL         string
	Revision        string
	ReuseCandidates []string
	Global          *GlobalPolicy
}

// GlobalPolicy summarises aggregated policy usage for presentation.
type GlobalPolicy struct {
	PolicyID           string
	PinnedBytes        ByteUsage
	Snapshots          CountUsage
	Replicas           CountUsage
	ActiveFingerprints []string
}

// ByteUsage captures byte-based quota consumption.
type ByteUsage struct {
	Used int64
	Soft int64
	Hard int64
}

// CountUsage captures count-based quota consumption.
type CountUsage struct {
	Used int
	Soft int
	Hard int
}

// TuneRequest captures requested policy overrides.
type TuneRequest struct {
	TTL            string
	ReplicationMin *int
	ReplicationMax *int
	Share          *bool
}

// InspectCommand renders hydration policy details for a ticket.
type InspectCommand struct {
	Ticket string
	Client Client
	Output io.Writer
}

// Run executes the hydration inspect command.
func (c InspectCommand) Run(ctx context.Context) error {
	if c.Client == nil {
		return errors.New("hydration: client required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("hydration: ticket required")
	}
	policy, err := c.Client.Inspect(ctx, ticket)
	if err != nil {
		return err
	}
	out := c.Output
	if out == nil {
		out = io.Discard
	}
	renderPolicy(out, policy)
	return nil
}

// TuneCommand applies retention overrides for a ticket.
type TuneCommand struct {
	Ticket string
	Client Client
}

// Run executes the tune command with the provided request payload.
func (c TuneCommand) Run(ctx context.Context, req TuneRequest) error {
	if c.Client == nil {
		return errors.New("hydration: client required")
	}
	ticket := strings.TrimSpace(c.Ticket)
	if ticket == "" {
		return errors.New("hydration: ticket required")
	}
	_, err := c.Client.Tune(ctx, ticket, req)
	return err
}

func renderPolicy(out io.Writer, policy Policy) {
	expires := "never"
	if !policy.ExpiresAt.IsZero() {
		expires = policy.ExpiresAt.UTC().Format(time.RFC3339)
	}
	candidates := strings.Join(policy.ReuseCandidates, ", ")
	if candidates == "" {
		candidates = "none"
	}
	fmt.Fprintf(out, "Ticket: %s\n", policy.Ticket)
	fmt.Fprintf(out, "Shared CID: %s\n", policy.SharedCID)
	fmt.Fprintf(out, "TTL: %s\n", strings.TrimSpace(policy.TTL))
	fmt.Fprintf(out, "Replication: min=%d max=%d\n", policy.ReplicationMin, policy.ReplicationMax)
	fmt.Fprintf(out, "Share: %t\n", policy.Share)
	fmt.Fprintf(out, "Expires At: %s\n", expires)
	fmt.Fprintf(out, "Repository: %s@%s\n", policy.RepoURL, policy.Revision)
	fmt.Fprintf(out, "Reuse Candidates: %s\n", candidates)
	if policy.Global != nil {
		fmt.Fprintf(out, "Global Policy: %s\n", policy.Global.PolicyID)
		fmt.Fprintf(out, "  Pinned Bytes: used=%d soft=%d hard=%d\n", policy.Global.PinnedBytes.Used, policy.Global.PinnedBytes.Soft, policy.Global.PinnedBytes.Hard)
		fmt.Fprintf(out, "  Snapshots: used=%d soft=%d hard=%d\n", policy.Global.Snapshots.Used, policy.Global.Snapshots.Soft, policy.Global.Snapshots.Hard)
		fmt.Fprintf(out, "  Replicas: used=%d soft=%d hard=%d\n", policy.Global.Replicas.Used, policy.Global.Replicas.Soft, policy.Global.Replicas.Hard)
		if len(policy.Global.ActiveFingerprints) > 0 {
			fmt.Fprintf(out, "  Active Fingerprints: %s\n", strings.Join(policy.Global.ActiveFingerprints, ", "))
		}
	}
}
