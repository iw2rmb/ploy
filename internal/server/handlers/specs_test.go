package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type specStore struct {
	store.Store

	getNamedSpecByNameSourceSHA mockCallSeq[store.GetNamedSpecByNameSourceSHAParams, store.Spec]
	createNamedSpec             mockCall[store.CreateNamedSpecParams, store.Spec]
	listLatestNamedSpecs        mockCall[store.ListLatestNamedSpecsParams, []store.ListLatestNamedSpecsRow]
	resolveByName               mockCall[string, []store.ResolveLatestNamedSpecByNameRow]
	resolveByRepoName           mockCall[store.ResolveLatestNamedSpecByRepoNameParams, []store.ResolveLatestNamedSpecByRepoNameRow]
	resolveByDomainRepoName     mockCall[store.ResolveLatestNamedSpecByDomainRepoNameParams, []store.ResolveLatestNamedSpecByDomainRepoNameRow]
}

func (s *specStore) GetNamedSpecByNameSourceSHA(ctx context.Context, arg store.GetNamedSpecByNameSourceSHAParams) (store.Spec, error) {
	return s.getNamedSpecByNameSourceSHA.record(arg)
}

func (s *specStore) CreateNamedSpec(ctx context.Context, arg store.CreateNamedSpecParams) (store.Spec, error) {
	if s.createNamedSpec.val.ID.IsZero() {
		s.createNamedSpec.val = specFromCreateNamedParams(arg)
	}
	return s.createNamedSpec.record(arg)
}

func (s *specStore) ListLatestNamedSpecs(ctx context.Context, arg store.ListLatestNamedSpecsParams) ([]store.ListLatestNamedSpecsRow, error) {
	return s.listLatestNamedSpecs.record(arg)
}

func (s *specStore) ResolveLatestNamedSpecByName(ctx context.Context, name string) ([]store.ResolveLatestNamedSpecByNameRow, error) {
	return s.resolveByName.record(name)
}

func (s *specStore) ResolveLatestNamedSpecByRepoName(ctx context.Context, arg store.ResolveLatestNamedSpecByRepoNameParams) ([]store.ResolveLatestNamedSpecByRepoNameRow, error) {
	return s.resolveByRepoName.record(arg)
}

func (s *specStore) ResolveLatestNamedSpecByDomainRepoName(ctx context.Context, arg store.ResolveLatestNamedSpecByDomainRepoNameParams) ([]store.ResolveLatestNamedSpecByDomainRepoNameRow, error) {
	return s.resolveByDomainRepoName.record(arg)
}

func TestNamedSpecs_Publish(t *testing.T) {
	now := time.Date(2026, 6, 18, 10, 20, 30, 0, time.UTC)
	baseBody := validNamedSpecPublishBody(now, nil)
	existing := testNamedStoreSpec("specold1", "upgrade-java", "github.com", "acme/service", "0123456789abcdef0123456789abcdef01234567", now)

	tests := []struct {
		name       string
		store      *specStore
		body       map[string]any
		wantStatus int
		verify     func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "created",
			store: func() *specStore {
				st := &specStore{}
				st.getNamedSpecByNameSourceSHA.errs = []error{pgx.ErrNoRows}
				return st
			}(),
			body:       baseBody,
			wantStatus: http.StatusCreated,
			verify: func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "CreateNamedSpec", st.createNamedSpec.called)
				if st.createNamedSpec.params.Name != "upgrade-java" || st.createNamedSpec.params.Sha == "" {
					t.Fatalf("create params mismatch: %+v", st.createNamedSpec.params)
				}
				resp := decodeBody[domainapi.NamedSpecSummary](t, rr)
				if resp.Skipped {
					t.Fatal("created response skipped=true, want false")
				}
			},
		},
		{
			name: "skipped existing",
			store: func() *specStore {
				st := &specStore{}
				st.getNamedSpecByNameSourceSHA.vals = []store.Spec{existing}
				return st
			}(),
			body:       baseBody,
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertNotCalled(t, "CreateNamedSpec", st.createNamedSpec.called)
				resp := decodeBody[domainapi.NamedSpecSummary](t, rr)
				if !resp.Skipped {
					t.Fatal("existing response skipped=false, want true")
				}
			},
		},
		{
			name: "unique race reloads existing",
			store: func() *specStore {
				st := &specStore{}
				st.getNamedSpecByNameSourceSHA.errs = []error{pgx.ErrNoRows, nil}
				st.getNamedSpecByNameSourceSHA.vals = []store.Spec{{}, existing}
				st.createNamedSpec.err = &pgconn.PgError{Code: "23505"}
				return st
			}(),
			body:       baseBody,
			wantStatus: http.StatusOK,
		},
		{
			name: "unique conflict without same row",
			store: func() *specStore {
				st := &specStore{}
				st.getNamedSpecByNameSourceSHA.errs = []error{pgx.ErrNoRows, pgx.ErrNoRows}
				st.createNamedSpec.err = &pgconn.PgError{Code: "23505"}
				return st
			}(),
			body:       baseBody,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "invalid name",
			store:      &specStore{},
			body:       validNamedSpecPublishBody(now, map[string]any{"name": "Upgrade"}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid source credentials",
			store:      &specStore{},
			body:       validNamedSpecPublishBody(now, map[string]any{"source": map[string]any{"domain": "github.com", "repo": "acme@service"}}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "description mismatch",
			store:      &specStore{},
			body:       validNamedSpecPublishBody(now, map[string]any{"description": "different"}),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing api version",
			store:      &specStore{},
			body:       validNamedSpecPublishBody(now, map[string]any{"spec": map[string]any{"name": "upgrade-java", "steps": []any{map[string]any{"image": "img"}}}}),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doRequest(t, publishNamedSpecHandler(tt.store), http.MethodPost, "/v1/specs", tt.body)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

func TestNamedSpecs_List(t *testing.T) {
	now := time.Date(2026, 6, 18, 10, 21, 0, 0, time.UTC)
	tests := []struct {
		name       string
		query      string
		store      *specStore
		wantStatus int
		verify     func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder)
	}{
		{
			name: "default named",
			store: func() *specStore {
				st := &specStore{}
				st.listLatestNamedSpecs.val = []store.ListLatestNamedSpecsRow{testNamedListRow("spec001", "upgrade-java", "github.com", "acme/service", now)}
				return st
			}(),
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ListLatestNamedSpecs", st.listLatestNamedSpecs.called)
				if st.listLatestNamedSpecs.params.Limit != 50 || st.listLatestNamedSpecs.params.Offset != 0 {
					t.Fatalf("pagination=%+v, want default", st.listLatestNamedSpecs.params)
				}
				resp := decodeBody[domainapi.NamedSpecListResponse](t, rr)
				if len(resp.Specs) != 1 || resp.Specs[0].Source.Domain != "github.com" {
					t.Fatalf("response mismatch: %+v", resp)
				}
			},
		},
		{name: "named false rejected", store: &specStore{}, query: "named=false", wantStatus: http.StatusBadRequest},
		{name: "bad limit rejected", store: &specStore{}, query: "named=true&limit=bad", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/v1/specs"
			if tt.query != "" {
				path += "?" + tt.query
			}
			rr := doRequest(t, listNamedSpecsHandler(tt.store), http.MethodGet, path, nil)
			assertStatus(t, rr, tt.wantStatus)
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

func TestNamedSpecs_Resolve(t *testing.T) {
	now := time.Date(2026, 6, 18, 10, 21, 0, 0, time.UTC)
	tests := []struct {
		name       string
		selector   string
		store      *specStore
		wantStatus int
		wantBody   string
		verify     func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder)
	}{
		{
			name:     "exact by domain repo name",
			selector: "github.com/acme/service:upgrade-java",
			store: func() *specStore {
				st := &specStore{}
				st.resolveByDomainRepoName.val = []store.ResolveLatestNamedSpecByDomainRepoNameRow{testNamedDomainRepoRow("spec001", "upgrade-java", "github.com", "acme/service", now)}
				return st
			}(),
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ResolveLatestNamedSpecByDomainRepoName", st.resolveByDomainRepoName.called)
				resp := decodeBody[domainapi.NamedSpecResolveResponse](t, rr)
				if resp.Name != "upgrade-java" || len(resp.Spec) == 0 {
					t.Fatalf("response mismatch: %+v", resp)
				}
			},
		},
		{
			name:     "ambiguous by name",
			selector: "upgrade-java",
			store: func() *specStore {
				st := &specStore{}
				st.resolveByName.val = []store.ResolveLatestNamedSpecByNameRow{
					testNamedNameRow("spec002", "upgrade-java", "gitlab.example.com", "acme/service", now),
					testNamedNameRow("spec001", "upgrade-java", "github.com", "acme/service", now),
				}
				return st
			}(),
			wantStatus: http.StatusConflict,
			wantBody:   "github.com/acme/service:upgrade-java, gitlab.example.com/acme/service:upgrade-java",
		},
		{
			name:       "none",
			selector:   "missing",
			store:      &specStore{},
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "invalid grammar",
			selector:   "github.com/acme/service:Bad",
			store:      &specStore{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:     "repo name selector uses repo query",
			selector: "acme/service:upgrade-java",
			store: func() *specStore {
				st := &specStore{}
				st.resolveByRepoName.val = []store.ResolveLatestNamedSpecByRepoNameRow{testNamedRepoRow("spec003", "upgrade-java", "github.com", "acme/service", now)}
				return st
			}(),
			wantStatus: http.StatusOK,
			verify: func(t *testing.T, st *specStore, rr *httptest.ResponseRecorder) {
				t.Helper()
				assertCalled(t, "ResolveLatestNamedSpecByRepoName", st.resolveByRepoName.called)
				if st.resolveByRepoName.params.Repo != "acme/service" {
					t.Fatalf("repo param=%q, want acme/service", st.resolveByRepoName.params.Repo)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := doRequest(t, resolveNamedSpecHandler(tt.store), http.MethodGet, "/v1/specs/resolve?selector="+tt.selector, nil)
			assertStatus(t, rr, tt.wantStatus)
			if tt.wantBody != "" && !strings.Contains(rr.Body.String(), tt.wantBody) {
				t.Fatalf("body %q does not contain %q", rr.Body.String(), tt.wantBody)
			}
			if tt.verify != nil {
				tt.verify(t, tt.store, rr)
			}
		})
	}
}

func validNamedSpecPublishBody(committedAt time.Time, overrides map[string]any) map[string]any {
	body := map[string]any{
		"name":                "upgrade-java",
		"description":         "Upgrade Java",
		"source":              map[string]any{"domain": "github.com", "repo": "acme/service"},
		"sha":                 "0123456789abcdef0123456789abcdef01234567",
		"source_committed_at": committedAt.Format(time.RFC3339),
		"spec": map[string]any{
			"apiVersion":  "ploy.mig/v1alpha1",
			"name":        "upgrade-java",
			"description": "Upgrade Java",
			"steps":       []any{map[string]any{"image": "img"}},
		},
	}
	for k, v := range overrides {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	return body
}

func specFromCreateNamedParams(arg store.CreateNamedSpecParams) store.Spec {
	return store.Spec{ID: arg.ID, Name: arg.Name, Description: arg.Description, Source: arg.Source, Sha: arg.Sha, SourceCommittedAt: arg.SourceCommittedAt, Spec: arg.Spec, CreatedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}
}

func testNamedStoreSpec(id, name, domain, repo, sha string, at time.Time) store.Spec {
	return store.Spec{ID: types.SpecID(id), Name: name, Description: "Upgrade Java", Source: mustNamedSourceRaw(domain, repo), Sha: sha, SourceCommittedAt: pgtype.Timestamptz{Time: at, Valid: true}, Spec: []byte(`{"apiVersion":"ploy.mig/v1alpha1"}`), CreatedAt: pgtype.Timestamptz{Time: at.Add(time.Minute), Valid: true}}
}

func testNamedListRow(id, name, domain, repo string, at time.Time) store.ListLatestNamedSpecsRow {
	return store.ListLatestNamedSpecsRow{ID: id, Name: name, Description: "Upgrade Java", Source: mustNamedSourceRaw(domain, repo), Sha: "0123456789abcdef0123456789abcdef01234567", SourceCommittedAt: pgtype.Timestamptz{Time: at, Valid: true}, Spec: []byte(`{}`), CreatedAt: pgtype.Timestamptz{Time: at.Add(time.Minute), Valid: true}}
}

func testNamedNameRow(id, name, domain, repo string, at time.Time) store.ResolveLatestNamedSpecByNameRow {
	return store.ResolveLatestNamedSpecByNameRow{ID: id, Name: name, Description: "Upgrade Java", Source: mustNamedSourceRaw(domain, repo), Sha: "0123456789abcdef0123456789abcdef01234567", SourceCommittedAt: pgtype.Timestamptz{Time: at, Valid: true}, Spec: []byte(`{}`), CreatedAt: pgtype.Timestamptz{Time: at.Add(time.Minute), Valid: true}}
}

func testNamedRepoRow(id, name, domain, repo string, at time.Time) store.ResolveLatestNamedSpecByRepoNameRow {
	return store.ResolveLatestNamedSpecByRepoNameRow{ID: id, Name: name, Description: "Upgrade Java", Source: mustNamedSourceRaw(domain, repo), Sha: "0123456789abcdef0123456789abcdef01234567", SourceCommittedAt: pgtype.Timestamptz{Time: at, Valid: true}, Spec: []byte(`{}`), CreatedAt: pgtype.Timestamptz{Time: at.Add(time.Minute), Valid: true}}
}

func testNamedDomainRepoRow(id, name, domain, repo string, at time.Time) store.ResolveLatestNamedSpecByDomainRepoNameRow {
	return store.ResolveLatestNamedSpecByDomainRepoNameRow{ID: id, Name: name, Description: "Upgrade Java", Source: mustNamedSourceRaw(domain, repo), Sha: "0123456789abcdef0123456789abcdef01234567", SourceCommittedAt: pgtype.Timestamptz{Time: at, Valid: true}, Spec: []byte(`{}`), CreatedAt: pgtype.Timestamptz{Time: at.Add(time.Minute), Valid: true}}
}

func mustNamedSourceRaw(domain, repo string) []byte {
	raw, err := json.Marshal(domainapi.NamedSpecSource{Domain: domain, Repo: repo})
	if err != nil {
		panic(err)
	}
	return raw
}
