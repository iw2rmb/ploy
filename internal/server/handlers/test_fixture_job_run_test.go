package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// Run mutation methods

func (m *handlerStore) CancelActiveJobsByRunAttempt(ctx context.Context, params store.CancelActiveJobsByRunAttemptParams) (int64, error) {
	return m.cancelActiveJobsByRunAttempt.record(params)
}

func (m *handlerStore) AckRunStart(ctx context.Context, id types.RunID) error {
	_, err := m.ackRunStart.ret()
	return err
}

func (m *handlerStore) UpdateRunCompletion(ctx context.Context, id types.RunID) error {
	_, err := m.updateRunCompletion.ret()
	return err
}

func (m *handlerStore) UpdateWaveStatus(ctx context.Context, params store.UpdateWaveStatusParams) error {
	_, err := m.updateWaveStatus.record(params)
	return err
}
