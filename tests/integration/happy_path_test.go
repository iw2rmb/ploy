package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
)

// TestHappyPath_CreateRun tests the happy path integration flow:
// create run → simulate node appends (events/logs).
// This test requires a test database accessible via PLOY_TEST_PG_DSN.
func TestHappyPath_CreateRepoModRun(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping integration test")
	}

	ctx := context.Background()
	db, err := store.NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	// Step 1: Create a run in queued status.
	modSpec := []byte(`{"type":"integration-test","description":"Happy path test"}`)
	run, err := db.CreateRun(ctx, store.CreateRunParams{
		RepoUrl:   "https://github.com/example/happy-path-test",
		Spec:      modSpec,
		Status:    store.RunStatusQueued,
		BaseRef:   "main",
		TargetRef: "feature/happy-path",
	})
	if err != nil {
		t.Fatalf("CreateRun() failed: %v", err)
	}
	t.Logf("Created run: id=%v, repo_url=%s, status=%s", run.ID, run.RepoUrl, run.Status)

	// Verify the run was created with expected values.
	if run.RepoUrl != "https://github.com/example/happy-path-test" {
		t.Errorf("Expected run repo_url https://github.com/example/happy-path-test, got %s", run.RepoUrl)
	}
	if run.Status != store.RunStatusQueued {
		t.Errorf("Expected status queued, got %s", run.Status)
	}
	if run.BaseRef != "main" {
		t.Errorf("Expected base_ref 'main', got %s", run.BaseRef)
	}
	if run.TargetRef != "feature/happy-path" {
		t.Errorf("Expected target_ref 'feature/happy-path', got %s", run.TargetRef)
	}

	// Step 4: Simulate node appends - Create events.
	now := time.Now().UTC()
	eventParams := store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		Level:   "info",
		Message: "Run started via integration test",
		Meta:    []byte(`{"source":"integration-test","action":"start"}`),
	}
	event, err := db.CreateEvent(ctx, eventParams)
	if err != nil {
		t.Fatalf("CreateEvent() failed: %v", err)
	}
	t.Logf("Created event: id=%d, run_id=%v, level=%s, message=%s", event.ID, event.RunID, event.Level, event.Message)

	// Verify the event was created with expected values.
	if event.RunID.Bytes != run.ID.Bytes {
		t.Errorf("Expected event run_id %v, got %v", run.ID, event.RunID)
	}
	if event.Level != "info" {
		t.Errorf("Expected event level 'info', got %s", event.Level)
	}
	if event.Message != "Run started via integration test" {
		t.Errorf("Expected message 'Run started via integration test', got %s", event.Message)
	}

	// Create a second event to simulate multiple appends.
	event2Params := store.CreateEventParams{
		RunID: run.ID,
		Time: pgtype.Timestamptz{
			Time:  now.Add(1 * time.Second),
			Valid: true,
		},
		Level:   "debug",
		Message: "Processing build steps",
		Meta:    []byte(`{"source":"integration-test","action":"build"}`),
	}
	event2, err := db.CreateEvent(ctx, event2Params)
	if err != nil {
		t.Fatalf("CreateEvent() #2 failed: %v", err)
	}
	t.Logf("Created event #2: id=%d, level=%s", event2.ID, event2.Level)

	// Step 5: Simulate node appends - Create logs.
	logData := []byte("INFO: Starting integration test run\nINFO: Cloning repository\nINFO: Building project\n")
	logParams := store.CreateLogParams{
		RunID:   run.ID,
		ChunkNo: 0,
		Data:    logData,
	}
	log, err := db.CreateLog(ctx, logParams)
	if err != nil {
		t.Fatalf("CreateLog() failed: %v", err)
	}
	t.Logf("Created log: id=%d, run_id=%v, chunk_no=%d, data_len=%d", log.ID, log.RunID, log.ChunkNo, len(log.Data))

	// Verify the log was created with expected values.
	if log.RunID.Bytes != run.ID.Bytes {
		t.Errorf("Expected log run_id %v, got %v", run.ID, log.RunID)
	}
	if log.ChunkNo != 0 {
		t.Errorf("Expected chunk_no 0, got %d", log.ChunkNo)
	}
	if string(log.Data) != string(logData) {
		t.Errorf("Expected log data %s, got %s", logData, log.Data)
	}

	// Create a second log chunk to simulate streaming appends.
	log2Data := []byte("INFO: Tests passing\nINFO: Build complete\n")
	log2Params := store.CreateLogParams{
		RunID:   run.ID,
		ChunkNo: 1,
		Data:    log2Data,
	}
	log2, err := db.CreateLog(ctx, log2Params)
	if err != nil {
		t.Fatalf("CreateLog() #2 failed: %v", err)
	}
	t.Logf("Created log #2: id=%d, chunk_no=%d", log2.ID, log2.ChunkNo)

	// Step 6: Verify data was persisted correctly by fetching back.
	// Verify run can be retrieved.
	fetchedRun, err := db.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}
	if fetchedRun.ID.Bytes != run.ID.Bytes {
		t.Errorf("Fetched run ID mismatch: expected %v, got %v", run.ID, fetchedRun.ID)
	}
	if fetchedRun.Status != store.RunStatusQueued {
		t.Errorf("Fetched run status: expected queued, got %s", fetchedRun.Status)
	}

	// Verify events can be listed by run.
	events, err := db.ListEventsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListEventsByRun() failed: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
	// Verify events are ordered correctly (by time).
	if len(events) >= 2 {
		if events[0].ID != event.ID {
			t.Errorf("First event ID mismatch: expected %d, got %d", event.ID, events[0].ID)
		}
		if events[1].ID != event2.ID {
			t.Errorf("Second event ID mismatch: expected %d, got %d", event2.ID, events[1].ID)
		}
		if events[0].Level != "info" {
			t.Errorf("First event level: expected 'info', got %s", events[0].Level)
		}
		if events[1].Level != "debug" {
			t.Errorf("Second event level: expected 'debug', got %s", events[1].Level)
		}
	}

	// Also verify ListEventsByRunSince returns only newer events.
	eventsSince, err := db.ListEventsByRunSince(ctx, store.ListEventsByRunSinceParams{RunID: run.ID, ID: event.ID})
	if err != nil {
		t.Fatalf("ListEventsByRunSince() failed: %v", err)
	}
	if len(eventsSince) != 1 || eventsSince[0].ID != event2.ID {
		t.Errorf("ListEventsByRunSince expected [event2], got len=%d firstID=%v", len(eventsSince), firstIDOrZero(eventsSince))
	}

	// Verify logs can be listed by run.
	logs, err := db.ListLogsByRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListLogsByRun() failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(logs))
	}
	// Verify logs are ordered by chunk_no.
	if len(logs) >= 2 {
		if logs[0].ID != log.ID {
			t.Errorf("First log ID mismatch: expected %d, got %d", log.ID, logs[0].ID)
		}
		if logs[1].ID != log2.ID {
			t.Errorf("Second log ID mismatch: expected %d, got %d", log2.ID, logs[1].ID)
		}
		if logs[0].ChunkNo != 0 {
			t.Errorf("First log chunk_no: expected 0, got %d", logs[0].ChunkNo)
		}
		if logs[1].ChunkNo != 1 {
			t.Errorf("Second log chunk_no: expected 1, got %d", logs[1].ChunkNo)
		}
	}

	// Also verify ListLogsByRunSince returns only newer chunks.
	logsSince, err := db.ListLogsByRunSince(ctx, store.ListLogsByRunSinceParams{RunID: run.ID, ID: log.ID})
	if err != nil {
		t.Fatalf("ListLogsByRunSince() failed: %v", err)
	}
	if len(logsSince) != 1 || logsSince[0].ID != log2.ID {
		t.Errorf("ListLogsByRunSince expected [log2], got len=%d firstID=%v", len(logsSince), firstLogIDOrZero(logsSince))
	}

	t.Log("Happy path integration test completed successfully")
}

// firstIDOrZero helps compact test error messages.
func firstIDOrZero(events []store.Event) int64 {
	if len(events) == 0 {
		return 0
	}
	return events[0].ID
}

// firstLogIDOrZero helps compact test error messages.
func firstLogIDOrZero(logs []store.Log) int64 {
	if len(logs) == 0 {
		return 0
	}
	return logs[0].ID
}
