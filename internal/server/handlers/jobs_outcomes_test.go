package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bsmock "github.com/iw2rmb/ploy/internal/blobstore/mock"
	types "github.com/iw2rmb/ploy/internal/domain/types"
	migsapi "github.com/iw2rmb/ploy/internal/migs/api"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/server/httpserver"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5/pgtype"
)

type outcomeStore struct {
	*handlerStore
	bundles map[types.JobID][]store.ArtifactBundle
}

func (s *outcomeStore) ListArtifactBundlesByRunAndJob(_ context.Context, p store.ListArtifactBundlesByRunAndJobParams) ([]store.ArtifactBundle, error) {
	return s.bundles[*p.JobID], nil
}

func outcomeBundle(t *testing.T, bs *bsmock.Store, job store.Job, name, filename, body string, created time.Time) store.ArtifactBundle {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	if err := tw.WriteHeader(&tar.Header{Name: filename, Mode: 0600, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	key := job.ID.String() + name + created.String()
	if _, err := bs.Put(context.Background(), key, "application/gzip", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	return store.ArtifactBundle{RunID: job.RunID, JobID: &job.ID, Name: &name, ObjectKey: &key, CreatedAt: pgtype.Timestamptz{Time: created, Valid: true}}
}

func TestJobOutcomeDownloadsPreserveJSONAndExecutionIdentity(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, tc := range []struct {
		name, endpoint, file, body string
		jobType                    types.JobType
		stale, missing, queued     bool
		want                       int
	}{
		{name: "pre SBOM", endpoint: "sbom", file: "sbom.spdx.json", body: `{"packages":[]}`, jobType: types.JobTypePreGate, want: 200},
		{name: "failed post CVEs", endpoint: "cves", file: "grype.json", body: `{"matches":[]}`, jobType: types.JobTypePostGate, want: 200},
		{name: "post SBOM", endpoint: "sbom", file: "sbom.spdx.json", body: `{"packages":[]}`, jobType: types.JobTypePostGate, want: 200},
		{name: "missing", endpoint: "cves", jobType: types.JobTypePreGate, missing: true, want: 404},
		{name: "previous execution", endpoint: "sbom", file: "sbom.spdx.json", body: `{}`, jobType: types.JobTypePreGate, stale: true, want: 404},
		{name: "queued retry", endpoint: "sbom", file: "sbom.spdx.json", body: `{}`, jobType: types.JobTypePreGate, queued: true, want: 404},
		{name: "migration", endpoint: "sbom", jobType: types.JobTypeMig, want: 400},
		{name: "pre diff", endpoint: "sbom-diff", jobType: types.JobTypePreGate, want: 400},
		{name: "invalid JSON", endpoint: "cves", file: "grype.json", body: `{`, jobType: types.JobTypePostGate, want: 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job := store.Job{ID: types.NewJobID(), RunID: types.NewRunID(), JobType: tc.jobType, Status: types.JobStatusFail, StartedAt: pgtype.Timestamptz{Time: now, Valid: !tc.queued}}
			st := &outcomeStore{handlerStore: &handlerStore{}, bundles: map[types.JobID][]store.ArtifactBundle{}}
			st.getJob.val = job
			bs := bsmock.New()
			created := now.Add(time.Second)
			if tc.stale {
				created = now.Add(-time.Second)
			}
			if !tc.missing {
				st.bundles[job.ID] = []store.ArtifactBundle{outcomeBundle(t, bs, job, tc.endpoint, tc.file, tc.body, created)}
			}
			req := httptest.NewRequest(http.MethodGet, "/v1/jobs/"+job.ID.String()+"/"+tc.endpoint, nil)
			req.SetPathValue("job_id", job.ID.String())
			rr := httptest.NewRecorder()
			getJobOutcomeHandler(st, bs, tc.endpoint).ServeHTTP(rr, req)
			assertStatus(t, rr, tc.want)
			if tc.want == 200 {
				if rr.Body.String() != tc.body || rr.Header().Get("Content-Type") != "application/json" || !strings.Contains(rr.Header().Get("Content-Disposition"), tc.file) {
					t.Fatalf("unexpected download: %v %s", rr.Header(), rr.Body.String())
				}
			}
		})
	}
}

func TestPostGateDiffUsesRequestedAttemptAndHandlesEmptyOrMissingBaseline(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, tc := range []struct {
		name, before, after string
		missing             bool
		want                int
		changes             int
	}{
		{"changed", `{"packages":[{"name":"alpha","versionInfo":"1"}]}`, `{"packages":[{"name":"alpha","versionInfo":"2"}]}`, false, 200, 1},
		{"empty", `{"packages":[]}`, `{"packages":[]}`, false, 200, 0},
		{"missing baseline", "", `{"packages":[]}`, true, 404, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			post := store.Job{ID: types.NewJobID(), RunID: types.NewRunID(), RepoID: types.NewRepoID(), Attempt: 2, JobType: types.JobTypePostGate, StartedAt: pgtype.Timestamptz{Time: now, Valid: true}}
			pre := post
			pre.ID = types.NewJobID()
			pre.JobType = types.JobTypePreGate
			st := &outcomeStore{handlerStore: &handlerStore{}, bundles: map[types.JobID][]store.ArtifactBundle{}}
			st.getJob.val = post
			st.listJobsByRunAttempt.val = []store.Job{pre, post}
			bs := bsmock.New()
			st.bundles[post.ID] = []store.ArtifactBundle{outcomeBundle(t, bs, post, "sbom", "sbom.spdx.json", tc.after, now)}
			if !tc.missing {
				st.bundles[pre.ID] = []store.ArtifactBundle{outcomeBundle(t, bs, pre, "sbom", "sbom.spdx.json", tc.before, now)}
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("job_id", post.ID.String())
			rr := httptest.NewRecorder()
			getJobOutcomeHandler(st, bs, "sbom-diff").ServeHTTP(rr, req)
			assertStatus(t, rr, tc.want)
			if st.listJobsByRunAttempt.params.Attempt != 2 || st.listJobsByRunAttempt.params.RunID != post.RunID {
				t.Fatal("wrong baseline selection")
			}
			if tc.want == 200 {
				var got migsapi.JobSBOMDiffResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.JobID != post.ID || got.BaselineJobID != pre.ID || len(got.Packages) != tc.changes || got.Packages == nil {
					t.Fatalf("unexpected diff: %+v", got)
				}
				if tc.changes == 1 && (got.Packages[0].VersionPre != "1" || got.Packages[0].VersionPost != "2" || got.Packages[0].Change != "changed") {
					t.Fatalf("wrong delta: %+v", got.Packages)
				}
			}
		})
	}
}

func TestJobOutcomeRoutesAcceptOnlyControlPlaneQueryTokens(t *testing.T) {
	t.Parallel()
	secret := "outcome-test-secret"
	for _, role := range []auth.Role{auth.RoleControlPlane, auth.RoleWorker, auth.RoleCLIAdmin, ""} {
		t.Run(string(role), func(t *testing.T) {
			st := &handlerStore{}
			st.getJob.val = store.Job{ID: types.NewJobID(), JobType: types.JobTypePreGate}
			srv, err := httpserver.NewServer(httpserver.Options{Authorizer: auth.NewAuthorizer(auth.Options{TokenSecret: secret})})
			if err != nil {
				t.Fatal(err)
			}
			bs := bsmock.New()
			ev, err := events.NewService(events.Options{})
			if err != nil {
				t.Fatal(err)
			}
			registerJobRoutes(srv, routeDeps{st: st, bs: bs, bp: blobpersist.New(st, bs), eventsService: ev})
			token := ""
			if role != "" {
				token, err = auth.GenerateAPIToken(secret, string(role), time.Now().Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, endpoint := range []string{"sbom", "cves", "sbom-diff"} {
				rr := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/jobs/"+st.getJob.val.ID.String()+"/"+endpoint+"?auth_token="+token, nil))
				want := http.StatusForbidden
				if role == "" {
					want = http.StatusUnauthorized
				}
				if role == auth.RoleControlPlane || role == auth.RoleCLIAdmin {
					want = http.StatusNotFound
					if endpoint == "sbom-diff" {
						want = http.StatusBadRequest
					}
				}
				assertStatus(t, rr, want)
			}
		})
	}
}
