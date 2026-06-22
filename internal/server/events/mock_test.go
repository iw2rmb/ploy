package events

import (
	"context"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestService creates a Service with sensible test defaults.
func newTestService(t *testing.T, store store.Store, opts ...func(*Options)) *Service {
	t.Helper()
	o := Options{
		BufferSize:  4,
		HistorySize: 8,
		Store:       store,
	}
	for _, fn := range opts {
		fn(&o)
	}
	svc, err := NewService(o)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return svc
}

// mockStore is a minimal mock implementation of store.Store for testing.
type mockStore struct {
	store.Querier
	createEventFunc func(ctx context.Context, arg store.CreateEventParams) (store.Event, error)
	createLogFunc   func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
	getJobFunc      func(ctx context.Context, id domaintypes.JobID) (store.Job, error)
}

func (m *mockStore) CreateEvent(ctx context.Context, arg store.CreateEventParams) (store.Event, error) {
	if m.createEventFunc != nil {
		return m.createEventFunc(ctx, arg)
	}
	return store.Event{}, nil
}

func (m *mockStore) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	if m.createLogFunc != nil {
		return m.createLogFunc(ctx, arg)
	}
	return store.Log{}, nil
}

// GetJob returns job metadata for log enrichment.
func (m *mockStore) GetJob(ctx context.Context, id domaintypes.JobID) (store.Job, error) {
	if m.getJobFunc != nil {
		return m.getJobFunc(ctx, id)
	}
	return store.Job{}, nil
}

func (m *mockStore) Close() {}

func (m *mockStore) Pool() *pgxpool.Pool {
	return nil
}

func (m *mockStore) CancelRun(ctx context.Context, runID domaintypes.RunID) error {
	return nil
}

func (m *mockStore) CancelWave(ctx context.Context, waveID domaintypes.WaveID) error {
	return nil
}

func (m *mockStore) CompleteBootstrapEnrollment(ctx context.Context, arg store.CompleteBootstrapEnrollmentParams) error {
	return nil
}

func (m *mockStore) RestartRun(ctx context.Context, runID domaintypes.RunID) (store.Run, error) {
	return store.Run{}, nil
}

func (m *mockStore) CreateWaveWithRuns(ctx context.Context, arg store.CreateWaveWithRunsParams) (store.Wave, []store.Run, error) {
	return store.Wave{}, nil, nil
}

func (m *mockStore) UnclaimJob(ctx context.Context, arg store.UnclaimJobParams) error {
	return nil
}
