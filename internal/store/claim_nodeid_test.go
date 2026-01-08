package store

import (
	"context"
	"errors"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestClaimJobRequiresNodeID(t *testing.T) {
	t.Parallel()

	s := &PgStore{}
	_, err := s.ClaimJob(context.Background(), types.NodeID(""))
	if !errors.Is(err, ErrEmptyNodeID) {
		t.Fatalf("ClaimJob(empty nodeID) error = %v, want %v", err, ErrEmptyNodeID)
	}
}
