package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
)

// createTestEventsService creates an events service for testing without a store.
// Use createTestEventsServiceWithStore for tests that need log/event persistence.
func createTestEventsService() (*server.EventsService, error) {
	return server.NewEventsService(server.EventsOptions{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	})
}

// createTestEventsServiceWithStore creates an events service with a store for testing.
// This is required for log handlers that persist via eventsService.CreateAndPublishLog.
func createTestEventsServiceWithStore(st store.Store) (*server.EventsService, error) {
	return server.NewEventsService(server.EventsOptions{
		BufferSize:  32,
		HistorySize: 256,
		Logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		Store:       st,
	})
}

// flushRecorder adapts httptest.ResponseRecorder to also implement http.Flusher.
type flushRecorder struct{ *httptest.ResponseRecorder }

func (f *flushRecorder) Flush() {}

// validSpecBody returns a canonical spec map for test request bodies.
func validSpecBody() map[string]any {
	return map[string]any{
		"version": "0.2.0",
		"env":     map[string]any{},
		"steps":   []any{map[string]any{"image": "docker.io/test/mig:latest"}},
	}
}

// validRunRequestBody returns a canonical single-repo run request body.
func validRunRequestBody() map[string]any {
	return map[string]any{
		"repo_url":   "https://github.com/org/repo",
		"base_ref":   "main",
		"target_ref": "feature",
		"spec":       validSpecBody(),
	}
}

// validRunRequestBodyWith returns a canonical run request body with overrides applied.
// A nil value deletes the key.
func validRunRequestBodyWith(overrides map[string]any) map[string]any {
	body := validRunRequestBody()
	for k, v := range overrides {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	return body
}

// validRunRequestBodyWithout returns a canonical run request body with the given keys removed.
func validRunRequestBodyWithout(keys ...string) map[string]any {
	body := validRunRequestBody()
	for _, k := range keys {
		delete(body, k)
	}
	return body
}

// newRunRepoJobsFixture creates a mock store and handler pre-configured for a single
// gate-type job with the given meta JSON. Returns the store, handler, runID, and repoID.
func newRunRepoJobsFixture(t *testing.T, metaJSON string) (*mockStore, http.Handler, domaintypes.RunID, domaintypes.RepoID) {
	t.Helper()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()
	jobID := domaintypes.NewJobID()

	st := &mockStore{
		getRunRepoResult: store.RunRepo{
			RunID:   runID,
			RepoID:  repoID,
			Attempt: 1,
		},
		listJobsByRunRepoAttemptResult: []store.Job{
			{
				ID:      jobID,
				RunID:   runID,
				RepoID:  repoID,
				Attempt: 1,
				Name:    "pre-gate",
				JobType: "pre_gate",
				Status:  domaintypes.JobStatusFail,
				Meta:    []byte(metaJSON),
			},
		},
	}
	handler := listRunRepoJobsHandler(st)
	return st, handler, runID, repoID
}

// allReposSelector returns a repo_selector body with mode "all".
func allReposSelector() map[string]any {
	return map[string]any{
		"repo_selector": map[string]any{
			"mode": "all",
		},
	}
}

// activeMigWithSpec returns a mockStore pre-configured with an active (non-archived)
// mig (ID "mod123"), a spec row, and one MigRepo. Eliminates repeated setup blocks
// across migs_runs and migs_spec tests.
func activeMigWithSpec(specID domaintypes.SpecID) *mockStore {
	return &mockStore{
		getModResult: store.Mig{
			ID:         "mod123",
			Name:       "test-mig",
			SpecID:     &specID,
			ArchivedAt: pgtype.Timestamptz{Valid: false},
		},
		getSpecResult: store.Spec{
			ID:   specID,
			Spec: []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`),
		},
		listMigReposByModResult: []store.MigRepo{
			{ID: "repo1", MigID: "mod123", RepoID: "repo1", BaseRef: "main", TargetRef: "feature1"},
		},
	}
}

var errMockDatabase = errors.New("mock database error")

// jobTestFixture holds the common identifiers and job used across jobs_complete tests.
type jobTestFixture struct {
	NodeIDStr string
	NodeID    domaintypes.NodeID
	RunID     domaintypes.RunID
	JobID     domaintypes.JobID
	Job       store.Job
}

// newJobFixture creates a running job fixture with default values.
// jobType defaults to domaintypes.JobTypeMod.
func newJobFixture(jobType domaintypes.JobType) jobTestFixture {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	runID := domaintypes.NewRunID()
	jobID := domaintypes.NewJobID()
	if jobType == "" {
		jobType = domaintypes.JobTypeMod
	}
	return jobTestFixture{
		NodeIDStr: nodeIDStr,
		NodeID:    nodeID,
		RunID:     runID,
		JobID:     jobID,
		Job: store.Job{
			ID:        jobID,
			RunID:     runID,
			NodeID:    &nodeID,
			Name:      jobType.String() + "-0",
			Status:    domaintypes.JobStatusRunning,
			JobType:   jobType,
			RepoShaIn: "0123456789abcdef0123456789abcdef01234567",
			Meta:      []byte(`{}`),
		},
	}
}

// newRepoScopedFixture creates a job fixture pre-configured with a repo ID,
// base ref "main", and attempt 1 — the common setup for repo-scoped tests.
func newRepoScopedFixture(jobType domaintypes.JobType) jobTestFixture {
	f := newJobFixture(jobType)
	f.Job.RepoID = domaintypes.NewRepoID()
	f.Job.RepoBaseRef = "main"
	f.Job.Attempt = 1
	return f
}

// jobStatusReq builds an HTTP request for GET /v1/jobs/{job_id}/status
// with the fixture's node identity header.
// If overrideNodeID is non-empty, it replaces the default node identity.
func (f jobTestFixture) jobStatusReq(overrideNodeID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+f.JobID.String()+"/status", nil)
	req.SetPathValue("job_id", f.JobID.String())
	nodeID := f.NodeIDStr
	if overrideNodeID != "" {
		nodeID = overrideNodeID
	}
	req.Header.Set(nodeUUIDHeader, nodeID)
	return req
}

// completeJobReq builds an HTTP request for POST /v1/jobs/{job_id}/complete
// with the given body map and worker auth context.
func (f jobTestFixture) completeJobReq(bodyMap map[string]any) *http.Request {
	body, _ := json.Marshal(bodyMap)
	req := httptest.NewRequest(http.MethodPost, "/v1/jobs/"+f.JobID.String()+"/complete", bytes.NewReader(body))
	req.SetPathValue("job_id", f.JobID.String())
	req.Header.Set(nodeUUIDHeader, f.NodeIDStr)
	ctx := auth.ContextWithIdentity(req.Context(), auth.Identity{
		Role:       auth.RoleWorker,
		CommonName: f.NodeIDStr,
	})
	return req.WithContext(ctx)
}

// newMockStoreForJob returns a mockStore pre-configured for a standard running job fixture.
// The store has getRunResult (Started), getJobResult, and listJobsByRunResult set.
// Pass functional options to override or extend the defaults.
func newMockStoreForJob(f jobTestFixture, opts ...func(*mockStore)) *mockStore {
	st := &mockStore{
		getRunResult:        store.Run{ID: f.RunID, Status: domaintypes.RunStatusStarted},
		getJobResult:        f.Job,
		listJobsByRunResult: []store.Job{f.Job},
	}
	for _, o := range opts {
		o(st)
	}
	return st
}

func withRepoAttemptJobs(jobs []store.Job) func(*mockStore) {
	return func(st *mockStore) { st.listJobsByRunRepoAttemptResult = jobs }
}

func withRunRepoStatusCounts(rows []store.CountRunReposByStatusRow) func(*mockStore) {
	return func(st *mockStore) { st.countRunReposByStatusResult = rows }
}

func withSpec(specID domaintypes.SpecID, specBytes []byte) func(*mockStore) {
	return func(st *mockStore) {
		st.getRunResult.SpecID = specID
		st.getSpecResult = store.Spec{ID: specID, Spec: specBytes}
	}
}

func withRunStatus(status domaintypes.RunStatus) func(*mockStore) {
	return func(st *mockStore) { st.getRunResult.Status = status }
}

func withJobResults(m map[domaintypes.JobID]store.Job) func(*mockStore) {
	return func(st *mockStore) { st.getJobResults = m }
}

func withPromoteResult(job store.Job) func(*mockStore) {
	return func(st *mockStore) { st.promoteJobByIDIfUnblockedResult = job }
}

func withGetRunErr(err error) func(*mockStore) {
	return func(st *mockStore) {
		st.getRunErr = err
		st.getRunResult = store.Run{}
	}
}

func withGetJobErr(err error) func(*mockStore) {
	return func(st *mockStore) {
		st.getJobErr = err
		st.getJobResult = store.Job{}
	}
}

func withListJobsByRun(jobs []store.Job) func(*mockStore) {
	return func(st *mockStore) { st.listJobsByRunResult = jobs }
}

func withArtifactBundles(bundles []store.ArtifactBundle) func(*mockStore) {
	return func(st *mockStore) { st.listArtifactBundlesMetaByRunAndJobResult = bundles }
}

func withResolveStackRow(row store.ResolveStackRowByLangToolRow) func(*mockStore) {
	return func(st *mockStore) { st.resolveStackRowByLangToolResult = row }
}

func withGetRunCreatedAt(t time.Time) func(*mockStore) {
	return func(st *mockStore) {
		st.getRunResult.CreatedAt = pgtype.Timestamptz{Time: t, Valid: true}
	}
}

// doRequestWithContentType sends a request with a custom Content-Type and string body.
// pathParams are key-value pairs: "mig_id", "mod123", "repo_id", "repo1".
func doRequestWithContentType(t *testing.T, handler http.Handler, method, path, contentType, body string, pathParams ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", contentType)
	for i := 0; i+1 < len(pathParams); i += 2 {
		req.SetPathValue(pathParams[i], pathParams[i+1])
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// doRequest sends a request to handler and returns the recorder.
// body: nil → no body; string → raw body; otherwise → JSON-marshaled.
// pathParams are key-value pairs: "mig_ref", "mod123", "run_id", "run1".
func doRequest(t *testing.T, handler http.Handler, method, path string, body any, pathParams ...string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	switch v := body.(type) {
	case nil:
		r = httptest.NewRequest(method, path, nil)
	case string:
		r = httptest.NewRequest(method, path, bytes.NewBufferString(v))
		r.Header.Set("Content-Type", "application/json")
	case []byte:
		r = httptest.NewRequest(method, path, bytes.NewReader(v))
		r.Header.Set("Content-Type", "application/json")
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(pathParams); i += 2 {
		r.SetPathValue(pathParams[i], pathParams[i+1])
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, r)
	return rr
}

// assertBodyContains fails if wantSubstr is non-empty and not found in the response body.
func assertBodyContains(t *testing.T, rr *httptest.ResponseRecorder, wantSubstr string) {
	t.Helper()
	if wantSubstr != "" && !bytes.Contains(rr.Body.Bytes(), []byte(wantSubstr)) {
		t.Errorf("body %q does not contain %q", rr.Body.String(), wantSubstr)
	}
}

// assertStatus fails the test if rr.Code != want.
func assertStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("expected status %d, got %d: %s", want, rr.Code, rr.Body.String())
	}
}

// assertJSONValue fails if the JSON body doesn't contain key with value want.
func assertJSONValue(t *testing.T, body, key, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got, _ := payload[key].(string)
	if got != want {
		t.Fatalf("response[%q] = %q, want %q", key, got, want)
	}
}

// assertCalled fails the test if called is false.
func assertCalled(t *testing.T, name string, called bool) {
	t.Helper()
	if !called {
		t.Fatalf("expected %s to be called", name)
	}
}

// assertNotCalled fails the test if called is true.
func assertNotCalled(t *testing.T, name string, called bool) {
	t.Helper()
	if called {
		t.Fatalf("did not expect %s to be called", name)
	}
}

// assertNoCompletion fails if either UpdateJobCompletion or UpdateJobCompletionWithMeta was called.
func assertNoCompletion(t *testing.T, st *mockStore) {
	t.Helper()
	if st.updateJobCompletionCalled || st.updateJobCompletionWithMetaCalled {
		t.Fatal("did not expect any completion persistence")
	}
}

// assertRepoError fails if UpdateRunRepoError was not called with the expected
// run/repo IDs and error substrings.
func assertRepoError(t *testing.T, st *mockStore, runID domaintypes.RunID, repoID domaintypes.RepoID, substrings ...string) {
	t.Helper()
	assertCalled(t, "UpdateRunRepoError", st.updateRunRepoErrorCalled)
	if st.updateRunRepoErrorParams.RunID != runID {
		t.Fatalf("expected RunID %s, got %s", runID, st.updateRunRepoErrorParams.RunID)
	}
	if st.updateRunRepoErrorParams.RepoID != repoID {
		t.Fatalf("expected RepoID %s, got %s", repoID, st.updateRunRepoErrorParams.RepoID)
	}
	if st.updateRunRepoErrorParams.LastError == nil {
		t.Fatal("expected LastError to be set")
	}
	msg := *st.updateRunRepoErrorParams.LastError
	for _, want := range substrings {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %s", want, msg)
		}
	}
}

// assertMetaKind fails if the persisted meta JSON doesn't have the expected kind.
func assertMetaKind(t *testing.T, metaBytes []byte, wantKind string) {
	t.Helper()
	var meta map[string]any
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("failed to unmarshal persisted meta: %v", err)
	}
	if kind, ok := meta["kind"].(string); !ok || kind != wantKind {
		t.Fatalf("expected meta.kind == %q, got %#v", wantKind, meta["kind"])
	}
}

// decodeBody decodes rr.Body as JSON into T.
func decodeBody[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rr.Body).Decode(&v); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return v
}

// newNodeFixture returns a node with generated ID, sensible defaults, and the given Drained flag.
func newNodeFixture(drained bool) (string, store.Node) {
	nodeIDStr := domaintypes.NewNodeKey()
	nodeID := domaintypes.NodeID(nodeIDStr)
	now := time.Now()
	return nodeIDStr, store.Node{
		ID:            nodeID,
		Name:          "worker-1",
		IpAddress:     netip.MustParseAddr("10.0.0.1"),
		Concurrency:   4,
		Drained:       drained,
		CreatedAt:     pgtype.Timestamptz{Time: now, Valid: true},
		LastHeartbeat: pgtype.Timestamptz{Time: now, Valid: true},
	}
}

// newTestServerWithRole creates an HTTP server with routes registered and
// the given auth role as the default for all requests.
func newTestServerWithRole(t *testing.T, role auth.Role) *server.HTTPServer {
	t.Helper()
	authz := auth.NewAuthorizer(auth.Options{AllowInsecure: true, DefaultRole: role})
	srv, err := server.NewHTTPServer(server.HTTPOptions{Authorizer: authz})
	if err != nil {
		t.Fatalf("http server: %v", err)
	}
	ev, err := server.NewEventsService(server.EventsOptions{})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	st := &mockStore{}
	bs := bsmock.New()
	bp := blobpersist.New(st, bs)
	RegisterRoutes(srv, st, bs, bp, ev, NewConfigHolder(config.GitLabConfig{}, nil), "test-secret")
	return srv
}

// mustTarGzPayload builds a gzipped tar archive from a map of filename → content.
func mustTarGzPayload(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))}); err != nil {
			t.Fatalf("write header %q: %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("write payload %q: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return b.Bytes()
}
