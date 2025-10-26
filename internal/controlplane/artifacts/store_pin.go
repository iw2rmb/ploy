package artifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// PinStateUpdate applies atomic pin metadata transitions.
type PinStateUpdate struct {
	State           PinState
	Replicas        *int
	RetryCountDelta int
	Error           string
	NextAttemptAt   time.Time
}

// UpdatePinState transitions the pin metadata for the supplied artifact identifier.
func (s *Store) UpdatePinState(ctx context.Context, id string, update PinStateUpdate) (Metadata, error) {
	if s == nil || s.client == nil {
		return Metadata{}, errors.New("artifacts: store uninitialised")
	}
	if strings.TrimSpace(id) == "" {
		return Metadata{}, errors.New("artifacts: id required")
	}
	if strings.TrimSpace(string(update.State)) == "" {
		return Metadata{}, errors.New("artifacts: pin state required")
	}

	meta, rev, err := s.readWithRevision(ctx, id)
	if err != nil {
		return Metadata{}, err
	}
	if meta.Deleted {
		return Metadata{}, ErrNotFound
	}

	now := s.clock().UTC()
	meta.PinState = update.State
	if update.Replicas != nil {
		meta.PinReplicas = *update.Replicas
	}
	if update.RetryCountDelta != 0 {
		meta.PinRetryCount += update.RetryCountDelta
		if meta.PinRetryCount < 0 {
			meta.PinRetryCount = 0
		}
	}
	meta.PinError = strings.TrimSpace(update.Error)
	meta.PinUpdatedAt = now
	meta.UpdatedAt = now
	if update.NextAttemptAt.IsZero() {
		meta.PinNextAttemptAt = time.Time{}
	} else {
		meta.PinNextAttemptAt = update.NextAttemptAt.UTC()
	}

	payload, err := json.Marshal(recordFromMetadata(meta))
	if err != nil {
		return Metadata{}, fmt.Errorf("artifacts: encode pin metadata: %w", err)
	}

	key := s.artifactKey(meta.ID)
	txn := s.client.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(key), "=", rev)).
		Then(clientv3.OpPut(key, string(payload)))

	resp, err := txn.Commit()
	if err != nil {
		return Metadata{}, fmt.Errorf("artifacts: update pin state %s: %w", meta.ID, err)
	}
	if !resp.Succeeded {
		return Metadata{}, fmt.Errorf("artifacts: update pin state %s: conflict", meta.ID)
	}
	return meta, nil
}
