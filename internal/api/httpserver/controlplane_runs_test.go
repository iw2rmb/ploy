package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockRunsStore implements store.Store for testing runs timing endpoints.
type mockRunsStore struct {
	store.Store
	getRunTimingFunc    func(ctx context.Context, id pgtype.UUID) (store.RunsTiming, error)
	listRunsTimingsFunc func(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error)
}

func (m *mockRunsStore) GetRunTiming(ctx context.Context, id pgtype.UUID) (store.RunsTiming, error) {
	if m.getRunTimingFunc != nil {
		return m.getRunTimingFunc(ctx, id)
	}
	return store.RunsTiming{}, nil
}

func (m *mockRunsStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	if m.listRunsTimingsFunc != nil {
		return m.listRunsTimingsFunc(ctx, arg)
	}
	return []store.RunsTiming{}, nil
}

func (m *mockRunsStore) Close() {}

func TestHandleRunsTiming(t *testing.T) {
	t.Parallel()

	testUUID := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}

	mockStore := &mockRunsStore{
		getRunTimingFunc: func(ctx context.Context, id pgtype.UUID) (store.RunsTiming, error) {
			return store.RunsTiming{
				ID:      testUUID,
				QueueMs: 1500,
				RunMs:   30000,
			}, nil
		},
	}

	server := &controlPlaneServer{
		store: mockStore,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/01020304-0506-0708-090a-0b0c0d0e0f10/timing", nil)
	rec := httptest.NewRecorder()

	server.handleRunsTiming(rec, req, "01020304-0506-0708-090a-0b0c0d0e0f10")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRunsTimingsList(t *testing.T) {
	t.Parallel()

	testUUID := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}

	mockStore := &mockRunsStore{
		listRunsTimingsFunc: func(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
			return []store.RunsTiming{
				{
					ID:      testUUID,
					QueueMs: 1500,
					RunMs:   30000,
				},
			}, nil
		},
	}

	server := &controlPlaneServer{
		store: mockStore,
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/runs?view=timing", nil)
	rec := httptest.NewRecorder()

	server.handleRunsTimingsList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
