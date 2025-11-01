package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// mockIngestionStore implements store.Store for testing ingestion endpoints.
type mockIngestionStore struct {
	store.Store
	getRunFunc               func(ctx context.Context, id pgtype.UUID) (store.Run, error)
	createDiffFunc           func(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error)
	createLogFunc            func(ctx context.Context, arg store.CreateLogParams) (store.Log, error)
	createArtifactBundleFunc func(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error)
}

func (m *mockIngestionStore) GetRun(ctx context.Context, id pgtype.UUID) (store.Run, error) {
	if m.getRunFunc != nil {
		return m.getRunFunc(ctx, id)
	}
	return store.Run{}, nil
}

func (m *mockIngestionStore) CreateDiff(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error) {
	if m.createDiffFunc != nil {
		return m.createDiffFunc(ctx, arg)
	}
	return store.Diff{}, nil
}

func (m *mockIngestionStore) CreateLog(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
	if m.createLogFunc != nil {
		return m.createLogFunc(ctx, arg)
	}
	return store.Log{}, nil
}

func (m *mockIngestionStore) CreateArtifactBundle(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
	if m.createArtifactBundleFunc != nil {
		return m.createArtifactBundleFunc(ctx, arg)
	}
	return store.ArtifactBundle{}, nil
}

func (m *mockIngestionStore) Close() {}

func TestHandleRunsDiffs(t *testing.T) {
	t.Parallel()

	testRunUUID := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}

	testDiffUUID := pgtype.UUID{
		Bytes: [16]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18,
			0x19, 0x1a, 0x1b, 0x1c, 0x1d, 0x1e, 0x1f, 0x20},
		Valid: true,
	}

	testStageUUID := pgtype.UUID{
		Bytes: [16]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
			0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30},
		Valid: true,
	}

	tests := []struct {
		name           string
		runID          string
		payload        CreateDiffRequest
		mockStore      *mockIngestionStore
		expectedStatus int
	}{
		{
			name:  "valid diff",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateDiffRequest{
				StageID: "21222324-2526-2728-292a-2b2c2d2e2f30",
				Patch:   []byte("diff --git a/file.txt b/file.txt\n+new line"),
				Summary: json.RawMessage(`{"files_changed": 1}`),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{ID: testRunUUID}, nil
				},
				createDiffFunc: func(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error) {
					return store.Diff{
						ID:      testDiffUUID,
						RunID:   testRunUUID,
						StageID: testStageUUID,
						Patch:   arg.Patch,
						Summary: arg.Summary,
						CreatedAt: pgtype.Timestamptz{
							Valid: true,
						},
					}, nil
				},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:  "run not found",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateDiffRequest{
				Patch: []byte("diff --git a/file.txt b/file.txt\n+new line"),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{}, pgx.ErrNoRows
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:  "invalid run id",
			runID: "invalid-uuid",
			payload: CreateDiffRequest{
				Patch: []byte("diff --git a/file.txt b/file.txt\n+new line"),
			},
			mockStore:      &mockIngestionStore{},
			expectedStatus: http.StatusBadRequest,
		},
        {
            name:  "payload too large",
            runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
            payload: CreateDiffRequest{
                Patch: make([]byte, maxIngestionPayloadSize+1),
            },
            mockStore: &mockIngestionStore{
                getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
                    return store.Run{ID: testRunUUID}, nil
                },
            },
            // application layer rejects >1 MiB patch
            expectedStatus: http.StatusRequestEntityTooLarge,
        },
    }

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &controlPlaneServer{
				store: tt.mockStore,
			}

			body, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tt.runID+"/diffs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleRunsDiffs(rec, req, tt.runID)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleRunsLogs(t *testing.T) {
	t.Parallel()

	testRunUUID := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}

	testStageUUID := pgtype.UUID{
		Bytes: [16]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
			0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30},
		Valid: true,
	}

	testBuildUUID := pgtype.UUID{
		Bytes: [16]byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38,
			0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40},
		Valid: true,
	}

	tests := []struct {
		name           string
		runID          string
		payload        CreateLogRequest
		mockStore      *mockIngestionStore
		expectedStatus int
	}{
		{
			name:  "valid log chunk",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateLogRequest{
				StageID: "21222324-2526-2728-292a-2b2c2d2e2f30",
				BuildID: "31323334-3536-3738-393a-3b3c3d3e3f40",
				ChunkNo: 0,
				Data:    []byte("gzipped log data"),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{ID: testRunUUID}, nil
				},
				createLogFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
					return store.Log{
						ID:      123,
						RunID:   testRunUUID,
						StageID: testStageUUID,
						BuildID: testBuildUUID,
						ChunkNo: arg.ChunkNo,
						Data:    arg.Data,
						CreatedAt: pgtype.Timestamptz{
							Valid: true,
						},
					}, nil
				},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:  "run not found",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateLogRequest{
				ChunkNo: 0,
				Data:    []byte("gzipped log data"),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{}, pgx.ErrNoRows
				},
			},
			expectedStatus: http.StatusNotFound,
		},
        {
            name:  "payload too large",
            runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
            payload: CreateLogRequest{
                ChunkNo: 0,
                Data:    make([]byte, maxIngestionPayloadSize+1),
            },
            mockStore: &mockIngestionStore{
                getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
                    return store.Run{ID: testRunUUID}, nil
                },
            },
            // application layer rejects >1 MiB chunk
            expectedStatus: http.StatusRequestEntityTooLarge,
        },
		{
			name:  "duplicate chunk",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateLogRequest{
				ChunkNo: 0,
				Data:    []byte("gzipped log data"),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{ID: testRunUUID}, nil
				},
				createLogFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
					return store.Log{}, errors.New("duplicate key value violates unique constraint")
				},
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &controlPlaneServer{
				store: tt.mockStore,
			}

			body, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tt.runID+"/logs", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleRunsLogs(rec, req, tt.runID)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleRunsArtifactBundles(t *testing.T) {
	t.Parallel()

	testRunUUID := pgtype.UUID{
		Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		Valid: true,
	}

	testBundleUUID := pgtype.UUID{
		Bytes: [16]byte{0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48,
			0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50},
		Valid: true,
	}

	testStageUUID := pgtype.UUID{
		Bytes: [16]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28,
			0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30},
		Valid: true,
	}

	bundleName := "artifacts.tar.gz"

	tests := []struct {
		name           string
		runID          string
		payload        CreateArtifactBundleRequest
		mockStore      *mockIngestionStore
		expectedStatus int
	}{
		{
			name:  "valid artifact bundle",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateArtifactBundleRequest{
				StageID: "21222324-2526-2728-292a-2b2c2d2e2f30",
				Name:    &bundleName,
				Bundle:  []byte("gzipped tar data"),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{ID: testRunUUID}, nil
				},
				createArtifactBundleFunc: func(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
					return store.ArtifactBundle{
						ID:      testBundleUUID,
						RunID:   testRunUUID,
						StageID: testStageUUID,
						Name:    arg.Name,
						Bundle:  arg.Bundle,
						CreatedAt: pgtype.Timestamptz{
							Valid: true,
						},
					}, nil
				},
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:  "run not found",
			runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
			payload: CreateArtifactBundleRequest{
				Bundle: []byte("gzipped tar data"),
			},
			mockStore: &mockIngestionStore{
				getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
					return store.Run{}, pgx.ErrNoRows
				},
			},
			expectedStatus: http.StatusNotFound,
		},
        {
            name:  "payload too large",
            runID: "01020304-0506-0708-090a-0b0c0d0e0f10",
            payload: CreateArtifactBundleRequest{
                Bundle: make([]byte, maxIngestionPayloadSize+1),
            },
            mockStore: &mockIngestionStore{
                getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
                    return store.Run{ID: testRunUUID}, nil
                },
            },
            // application layer rejects >1 MiB bundle
            expectedStatus: http.StatusRequestEntityTooLarge,
        },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &controlPlaneServer{
				store: tt.mockStore,
			}

			body, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("failed to marshal payload: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/v1/runs/"+tt.runID+"/artifact_bundles", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			server.handleRunsArtifactBundles(rec, req, tt.runID)

			if rec.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d: %s", tt.expectedStatus, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestIngestionAcceptsPayloadAtLimit(t *testing.T) {
    t.Parallel()

    testRunUUID := pgtype.UUID{Bytes: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}, Valid: true}
    stageUUID := pgtype.UUID{Bytes: [16]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f, 0x30}, Valid: true}
    buildUUID := pgtype.UUID{Bytes: [16]byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40}, Valid: true}

    runIDStr := "01020304-0506-0708-090a-0b0c0d0e0f10"
    stageIDStr := "21222324-2526-2728-292a-2b2c2d2e2f30"
    buildIDStr := "31323334-3536-3738-393a-3b3c3d3e3f40"

    // Prepare exactly 1 MiB binary blobs; after JSON base64 they exceed 1 MiB.
    atLimit := make([]byte, maxIngestionPayloadSize)

    storeOK := &mockIngestionStore{
        getRunFunc: func(ctx context.Context, id pgtype.UUID) (store.Run, error) {
            return store.Run{ID: testRunUUID}, nil
        },
        createDiffFunc: func(ctx context.Context, arg store.CreateDiffParams) (store.Diff, error) {
            return store.Diff{ID: stageUUID, RunID: testRunUUID, StageID: stageUUID, Patch: arg.Patch, CreatedAt: pgtype.Timestamptz{Valid: true}}, nil
        },
        createLogFunc: func(ctx context.Context, arg store.CreateLogParams) (store.Log, error) {
            return store.Log{ID: 1, RunID: testRunUUID, StageID: stageUUID, BuildID: buildUUID, ChunkNo: arg.ChunkNo, Data: arg.Data, CreatedAt: pgtype.Timestamptz{Valid: true}}, nil
        },
        createArtifactBundleFunc: func(ctx context.Context, arg store.CreateArtifactBundleParams) (store.ArtifactBundle, error) {
            name := "bundle"
            return store.ArtifactBundle{ID: stageUUID, RunID: testRunUUID, StageID: stageUUID, BuildID: buildUUID, Name: &name, Bundle: arg.Bundle, CreatedAt: pgtype.Timestamptz{Valid: true}}, nil
        },
    }

    server := &controlPlaneServer{store: storeOK}

    // Diff at limit
    diffBody, _ := json.Marshal(CreateDiffRequest{StageID: stageIDStr, Patch: atLimit})
    reqDiff := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/diffs", bytes.NewReader(diffBody))
    reqDiff.Header.Set("Content-Type", "application/json")
    recDiff := httptest.NewRecorder()
    server.handleRunsDiffs(recDiff, reqDiff, runIDStr)
    if recDiff.Code != http.StatusCreated {
        t.Fatalf("diff at limit: expected 201, got %d: %s", recDiff.Code, recDiff.Body.String())
    }

    // Log at limit
    logBody, _ := json.Marshal(CreateLogRequest{StageID: stageIDStr, BuildID: buildIDStr, ChunkNo: 0, Data: atLimit})
    reqLog := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/logs", bytes.NewReader(logBody))
    reqLog.Header.Set("Content-Type", "application/json")
    recLog := httptest.NewRecorder()
    server.handleRunsLogs(recLog, reqLog, runIDStr)
    if recLog.Code != http.StatusCreated {
        t.Fatalf("log at limit: expected 201, got %d: %s", recLog.Code, recLog.Body.String())
    }

    // Artifact bundle at limit
    bundleBody, _ := json.Marshal(CreateArtifactBundleRequest{StageID: stageIDStr, BuildID: buildIDStr, Bundle: atLimit})
    reqBundle := httptest.NewRequest(http.MethodPost, "/v1/runs/"+runIDStr+"/artifact_bundles", bytes.NewReader(bundleBody))
    reqBundle.Header.Set("Content-Type", "application/json")
    recBundle := httptest.NewRecorder()
    server.handleRunsArtifactBundles(recBundle, reqBundle, runIDStr)
    if recBundle.Code != http.StatusCreated {
        t.Fatalf("bundle at limit: expected 201, got %d: %s", recBundle.Code, recBundle.Body.String())
    }
}
