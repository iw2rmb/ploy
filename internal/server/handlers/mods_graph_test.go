package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/graph"
)

// testTicketIDKSUID is a synthetic KSUID-like ID (27 characters) used for tests.
const testTicketIDKSUID = "123456789012345678901234567"

// TestGetModGraphHandler_Success verifies successful graph retrieval.
func TestGetModGraphHandler_Success(t *testing.T) {
	runID := testTicketIDKSUID
	job1ID := uuid.New()
	job2ID := uuid.New()
	job3ID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			RepoUrl:   "https://github.com/user/repo.git",
			Status:    store.RunStatusRunning,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{
				ID:        job1ID.String(),
				RunID:     runID,
				Name:      "pre-gate",
				Status:    store.JobStatusSucceeded,
				ModType:   "pre_gate",
				StepIndex: 1000,
			},
			{
				ID:        job2ID.String(),
				RunID:     runID,
				Name:      "mod-0",
				Status:    store.JobStatusRunning,
				ModType:   "mod",
				ModImage:  "mods-orw:latest",
				StepIndex: 2000,
			},
			{
				ID:        job3ID.String(),
				RunID:     runID,
				Name:      "post-gate",
				Status:    store.JobStatusCreated,
				ModType:   "post_gate",
				StepIndex: 3000,
			},
		},
	}

	handler := getModGraphHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID+"/graph", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify JSON structure.
	var result graph.WorkflowGraph
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify run ID.
	if result.RunID != runID {
		t.Errorf("RunID = %q, want %q", result.RunID, runID)
	}

	// Verify 3 nodes.
	if len(result.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(result.Nodes))
	}

	// Verify node types.
	preGate := result.Nodes[job1ID.String()]
	if preGate == nil {
		t.Fatal("pre-gate node not found")
	}
	if preGate.Type != graph.NodeTypePreGate {
		t.Errorf("pre-gate Type = %v, want %v", preGate.Type, graph.NodeTypePreGate)
	}
	if preGate.Status != graph.NodeStatusSucceeded {
		t.Errorf("pre-gate Status = %v, want %v", preGate.Status, graph.NodeStatusSucceeded)
	}

	mod0 := result.Nodes[job2ID.String()]
	if mod0 == nil {
		t.Fatal("mod-0 node not found")
	}
	if mod0.Type != graph.NodeTypeMod {
		t.Errorf("mod-0 Type = %v, want %v", mod0.Type, graph.NodeTypeMod)
	}
	if mod0.Image != "mods-orw:latest" {
		t.Errorf("mod-0 Image = %q, want %q", mod0.Image, "mods-orw:latest")
	}

	postGate := result.Nodes[job3ID.String()]
	if postGate == nil {
		t.Fatal("post-gate node not found")
	}
	if postGate.Type != graph.NodeTypePostGate {
		t.Errorf("post-gate Type = %v, want %v", postGate.Type, graph.NodeTypePostGate)
	}

	// Verify edges (linear chain).
	if len(preGate.ChildIDs) != 1 || preGate.ChildIDs[0] != job2ID.String() {
		t.Errorf("pre-gate should have child mod-0, got %v", preGate.ChildIDs)
	}
	if len(mod0.ParentIDs) != 1 || mod0.ParentIDs[0] != job1ID.String() {
		t.Errorf("mod-0 should have parent pre-gate, got %v", mod0.ParentIDs)
	}

	// Verify roots and leaves.
	if len(result.RootIDs) != 1 || result.RootIDs[0] != job1ID.String() {
		t.Errorf("RootIDs should be [%s], got %v", job1ID.String(), result.RootIDs)
	}
	if len(result.LeafIDs) != 1 || result.LeafIDs[0] != job3ID.String() {
		t.Errorf("LeafIDs should be [%s], got %v", job3ID.String(), result.LeafIDs)
	}

	// Should be linear.
	if !result.Linear {
		t.Error("graph should be linear")
	}

	// Verify store methods called.
	if !st.getRunCalled {
		t.Error("GetRun should be called")
	}
	if !st.listJobsByRunCalled {
		t.Error("ListJobsByRun should be called")
	}
}

// TestGetModGraphHandler_WithHealing verifies graph with healing jobs.
func TestGetModGraphHandler_WithHealing(t *testing.T) {
	runID := testTicketIDKSUID
	preGateID := uuid.New()
	heal1ID := uuid.New()
	reGateID := uuid.New()
	mod0ID := uuid.New()
	postGateID := uuid.New()
	now := time.Now()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			Status:    store.RunStatusRunning,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listJobsByRunResult: []store.Job{
			{ID: preGateID.String(), RunID: runID, Name: "pre-gate", Status: store.JobStatusFailed, ModType: "pre_gate", StepIndex: 1000},
			{ID: heal1ID.String(), RunID: runID, Name: "heal-1", Status: store.JobStatusSucceeded, ModType: "heal", StepIndex: 1500, ModImage: "mods-codex:latest"},
			{ID: reGateID.String(), RunID: runID, Name: "re-gate", Status: store.JobStatusSucceeded, ModType: "re_gate", StepIndex: 1750},
			{ID: mod0ID.String(), RunID: runID, Name: "mod-0", Status: store.JobStatusRunning, ModType: "mod", StepIndex: 2000},
			{ID: postGateID.String(), RunID: runID, Name: "post-gate", Status: store.JobStatusCreated, ModType: "post_gate", StepIndex: 3000},
		},
	}

	handler := getModGraphHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID+"/graph", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result graph.WorkflowGraph
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify 5 nodes.
	if len(result.Nodes) != 5 {
		t.Errorf("expected 5 nodes, got %d", len(result.Nodes))
	}

	// Verify healing node.
	heal1 := result.Nodes[heal1ID.String()]
	if heal1 == nil {
		t.Fatal("heal-1 node not found")
	}
	if heal1.Type != graph.NodeTypeHeal {
		t.Errorf("heal-1 Type = %v, want %v", heal1.Type, graph.NodeTypeHeal)
	}

	// Verify re-gate node.
	reGate := result.Nodes[reGateID.String()]
	if reGate == nil {
		t.Fatal("re-gate node not found")
	}
	if reGate.Type != graph.NodeTypeReGate {
		t.Errorf("re-gate Type = %v, want %v", reGate.Type, graph.NodeTypeReGate)
	}
}

// TestGetModGraphHandler_TicketNotFound verifies 404 for nonexistent ticket.
func TestGetModGraphHandler_TicketNotFound(t *testing.T) {
	st := &mockStore{
		getRunErr: pgx.ErrNoRows,
	}

	handler := getModGraphHandler(st)

	ticketID := testTicketIDKSUID
	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+ticketID+"/graph", nil)
	req.SetPathValue("id", ticketID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetModGraphHandler_InvalidTicketID verifies 400 for invalid UUID.
func TestGetModGraphHandler_InvalidTicketID(t *testing.T) {
	st := &mockStore{}
	handler := getModGraphHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/not-a-uuid/graph", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetModGraphHandler_MissingTicketID verifies 400 when ID is empty.
func TestGetModGraphHandler_MissingTicketID(t *testing.T) {
	st := &mockStore{}
	handler := getModGraphHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods//graph", nil)
	req.SetPathValue("id", "")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetModGraphHandler_EmptyJobs verifies graph for ticket with no jobs.
func TestGetModGraphHandler_EmptyJobs(t *testing.T) {
	runID := testTicketIDKSUID
	now := time.Now()

	st := &mockStore{
		getRunResult: store.Run{
			ID:        runID,
			Status:    store.RunStatusQueued,
			CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
		},
		listJobsByRunResult: []store.Job{}, // No jobs.
	}

	handler := getModGraphHandler(st)

	req := httptest.NewRequest(http.MethodGet, "/v1/mods/"+runID+"/graph", nil)
	req.SetPathValue("id", runID)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var result graph.WorkflowGraph
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have 0 nodes.
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.RootIDs) != 0 {
		t.Errorf("expected 0 roots, got %d", len(result.RootIDs))
	}
	if len(result.LeafIDs) != 0 {
		t.Errorf("expected 0 leaves, got %d", len(result.LeafIDs))
	}
}
