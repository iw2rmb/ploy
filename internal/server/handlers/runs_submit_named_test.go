package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/server/speccatalog"
	"github.com/iw2rmb/ploy/internal/store"
)

type runSpecCatalogStub struct {
	entry    speccatalog.Entry
	root     string
	err      error
	selector string
}

func (s *runSpecCatalogStub) WithResolvedSource(_ context.Context, selector string, use func(speccatalog.Entry, string) error) error {
	s.selector = selector
	if s.err != nil {
		return s.err
	}
	return use(s.entry, s.root)
}

type runBundleStoreStub struct {
	cids     []string
	archives [][]byte
}

type namedSnapshotStore struct {
	*handlerStore
	snapshots []store.Spec
}

func (s *namedSnapshotStore) CreateGitSpecSnapshot(_ context.Context, params store.CreateGitSpecSnapshotParams) (store.Spec, error) {
	created := store.Spec{
		ID: params.ID, Name: params.Name, Description: params.Description, Source: params.Source,
		Sha: params.Sha, SourceCommittedAt: params.SourceCommittedAt, Spec: params.Spec, CreatedBy: params.CreatedBy,
	}
	s.snapshots = append(s.snapshots, created)
	return created, nil
}

func (s *namedSnapshotStore) GetGitSpecSnapshot(_ context.Context, params store.GetGitSpecSnapshotParams) (store.Spec, error) {
	for _, snapshot := range s.snapshots {
		var source map[string]string
		if err := json.Unmarshal(snapshot.Source, &source); err != nil {
			return store.Spec{}, err
		}
		if snapshot.Name == params.Name && snapshot.Sha == params.Sha &&
			source["domain"] == params.Domain && source["repo"] == params.Repo && source["path"] == params.Path &&
			bytes.Equal(snapshot.Spec, params.Spec) {
			return snapshot, nil
		}
	}
	return store.Spec{}, pgx.ErrNoRows
}

func (s *runBundleStoreStub) Ensure(_ context.Context, cid string, archive []byte) (string, error) {
	s.cids = append(s.cids, cid)
	s.archives = append(s.archives, slices.Clone(archive))
	return "bundle-test", nil
}

func TestRunsCreateSingleRepo_NamedSpecCompilesOverridesAndPersistsSnapshot(t *testing.T) {
	t.Setenv("PLOY_CONTAINER_REGISTRY", "registry.example.com/ploy")
	root := t.TempDir()
	writeNamedRunTestFile(t, filepath.Join(root, "scenarios", "upgrade.yaml"), `
apiVersion: ploy.mig/v1alpha1
name: upgrade-java
description: Upgrade Java
steps:
  - name: rewrite
    image: $PLOY_CONTAINER_REGISTRY/mig:latest
    envs:
      MODE: $MISSING_MODE
    in:
      - ./input.txt:/in/input.txt
`)
	writeNamedRunTestFile(t, filepath.Join(root, "scenarios", "input.txt"), "input\n")
	committedAt := time.Date(2026, 8, 1, 10, 20, 30, 0, time.UTC)
	catalog := &runSpecCatalogStub{
		root: root,
		entry: speccatalog.Entry{
			Name: "upgrade-java", Description: "Upgrade Java", Source: "https://gitlab.example.com/platform/specs",
			Path: "scenarios/upgrade.yaml", SHA: "0123456789abcdef0123456789abcdef01234567", CommittedAt: committedAt,
		},
	}
	bundles := &runBundleStoreStub{}
	st := &handlerStore{}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{
		catalog: catalog, bundles: bundles, envAllowlist: []string{"PLOY_CONTAINER_REGISTRY"},
	})
	body := validRunRequestBodyWith(map[string]any{
		"spec":          nil,
		"spec_selector": "platform/specs:upgrade-java",
		"spec_overrides": map[string]any{
			"step_envs": map[string]any{"rewrite": []string{"MODE=strict", "EXTRA=1"}},
			"build_gate_forced": map[string]any{
				"pre": map[string]string{"language": "java", "release": "21", "tool": "gradle"},
			},
		},
	})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", body)

	assertStatus(t, rr, http.StatusCreated)
	if catalog.selector != "platform/specs:upgrade-java" {
		t.Fatalf("catalog selector = %q", catalog.selector)
	}
	if len(bundles.cids) != 1 || len(bundles.archives[0]) == 0 {
		t.Fatalf("persisted bundles = %d, want one materialized archive", len(bundles.cids))
	}
	if !st.createGitSpecSnapshot.called {
		t.Fatal("CreateGitSpecSnapshot was not called")
	}
	created := st.createGitSpecSnapshot.params
	if created.Name != "upgrade-java" || created.Description != "Upgrade Java" || created.Sha != catalog.entry.SHA || !created.SourceCommittedAt.Valid || !created.SourceCommittedAt.Time.Equal(committedAt) {
		t.Fatalf("named snapshot metadata = %+v", created)
	}
	var source map[string]string
	if err := json.Unmarshal(created.Source, &source); err != nil {
		t.Fatalf("decode source: %v", err)
	}
	if source["url"] != catalog.entry.Source || source["path"] != catalog.entry.Path || source["domain"] != "gitlab.example.com" || source["repo"] != "platform/specs" {
		t.Fatalf("source attribution = %#v", source)
	}
	var canonical map[string]any
	if err := json.Unmarshal(created.Spec, &canonical); err != nil {
		t.Fatalf("decode canonical spec: %v", err)
	}
	step := canonical["steps"].([]any)[0].(map[string]any)
	if step["image"] != "registry.example.com/ploy/mig:latest" {
		t.Fatalf("step image = %v", step["image"])
	}
	envs := step["envs"].(map[string]any)
	if envs["MODE"] != "strict" || envs["EXTRA"] != "1" {
		t.Fatalf("step envs = %#v", envs)
	}
	pre := canonical["build_gate"].(map[string]any)["pre"].(map[string]any)["stack"].(map[string]any)
	if pre["mode"] != "forced" || pre["language"] != "java" || pre["release"] != "21" || pre["tool"] != "gradle" {
		t.Fatalf("forced pre stack = %#v", pre)
	}
	if st.createWaveWithRuns.params.Wave.SpecID != st.createGitSpecSnapshot.val.ID {
		t.Fatalf("run spec id = %s, want persisted snapshot %s", st.createWaveWithRuns.params.Wave.SpecID, st.createGitSpecSnapshot.val.ID)
	}
}

func TestRunsCreateSingleRepo_NamedSpecRejectsNonAllowlistedEnvironment(t *testing.T) {
	t.Setenv("SERVER_ONLY_IMAGE", "registry.example.com/private/mig:latest")
	root := t.TempDir()
	writeNamedRunTestFile(t, filepath.Join(root, "upgrade.yaml"), `
apiVersion: ploy.mig/v1alpha1
name: upgrade-java
steps:
  - name: rewrite
    image: $SERVER_ONLY_IMAGE
`)
	catalog := &runSpecCatalogStub{root: root, entry: speccatalog.Entry{
		Name: "upgrade-java", Source: "https://git.example.com/team/specs", Path: "upgrade.yaml",
		SHA: "0123456789abcdef0123456789abcdef01234567", CommittedAt: time.Now().UTC(),
	}}
	st := &handlerStore{}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{
		catalog: catalog, envAllowlist: []string{"PLOY_CONTAINER_REGISTRY"},
	})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"spec": nil, "spec_selector": "upgrade-java",
	}))

	assertStatus(t, rr, http.StatusBadRequest)
	if !strings.Contains(rr.Body.String(), "unresolved environment variables: SERVER_ONLY_IMAGE") {
		t.Fatalf("body = %q, want unresolved environment variable", rr.Body.String())
	}
	if st.createSpec.called || st.createGitSpecSnapshot.called || st.createMig.called || st.createRun.called {
		t.Fatal("compile failure created durable rows")
	}
}

func TestNamedSpecEnvironmentSnapshotIsStable(t *testing.T) {
	lookups := 0
	lookup := snapshotNamedSpecEnvironment([]string{"SAFE_NAME"}, func(name string) (string, bool) {
		lookups++
		if lookups == 1 {
			return name + "-first-value", true
		}
		return name + "-changed-value", true
	})

	first, firstOK := lookup("SAFE_NAME")
	second, secondOK := lookup("SAFE_NAME")
	if !firstOK || !secondOK || first != second {
		t.Fatalf("snapshot values = %q/%q, present = %t/%t", first, second, firstOK, secondOK)
	}
	if _, ok := lookup("NOT_ALLOWED"); ok {
		t.Fatal("snapshot exposed non-allowlisted environment variable")
	}
	if lookups != 1 {
		t.Fatalf("source lookups = %d, want 1", lookups)
	}
}

func TestRunsCreateSingleRepo_NamedSpecOverrideVariantsPersistDistinctSnapshots(t *testing.T) {
	root := t.TempDir()
	writeNamedRunTestFile(t, filepath.Join(root, "upgrade.yaml"), `
apiVersion: ploy.mig/v1alpha1
name: upgrade-java
steps:
  - name: rewrite
    image: alpine:latest
    envs:
      MODE: base
`)
	catalog := &runSpecCatalogStub{root: root, entry: speccatalog.Entry{
		Name: "upgrade-java", Source: "https://git.example.com/team/specs", Path: "upgrade.yaml",
		SHA: "0123456789abcdef0123456789abcdef01234567", CommittedAt: time.Now().UTC(),
	}}
	st := &namedSnapshotStore{handlerStore: &handlerStore{}}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{catalog: catalog})

	for _, mode := range []string{"one", "two", "two"} {
		rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
			"spec": nil, "spec_selector": "upgrade-java",
			"spec_overrides": map[string]any{"step_envs": map[string]any{"rewrite": []string{"MODE=" + mode}}},
		}))
		assertStatus(t, rr, http.StatusCreated)
	}

	if len(st.snapshots) != 2 {
		t.Fatalf("persisted snapshots = %d, want one for each distinct canonical override result", len(st.snapshots))
	}
	if string(st.snapshots[0].Spec) == string(st.snapshots[1].Spec) {
		t.Fatal("override variants persisted identical canonical specs")
	}
}

func TestRunsCreateSingleRepo_NamedSpecFailuresDoNotCreateDurableRows(t *testing.T) {
	ambiguous := &speccatalog.AmbiguousError{Selector: "upgrade", Choices: []speccatalog.Entry{
		{Source: "https://git.example.com/one/specs", Path: "one.yaml"},
		{Source: "https://git.example.com/two/specs", Path: "two.yaml"},
	}}
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "no repositories", err: speccatalog.ErrNoRepositories, wantStatus: http.StatusServiceUnavailable, wantBody: "no spec repositories configured"},
		{name: "not found", err: speccatalog.ErrNotFound, wantStatus: http.StatusNotFound, wantBody: "named spec not found"},
		{name: "ambiguous", err: ambiguous, wantStatus: http.StatusConflict, wantBody: "one.yaml"},
		{name: "invalid selector", err: &speccatalog.InvalidSelectorError{Selector: "bad@sha"}, wantStatus: http.StatusBadRequest, wantBody: "invalid named spec selector"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := &handlerStore{}
			catalog := &runSpecCatalogStub{err: tc.err}
			handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{catalog: catalog})
			rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
				"spec": nil, "spec_selector": "upgrade",
			}))
			assertStatus(t, rr, tc.wantStatus)
			if !strings.Contains(rr.Body.String(), tc.wantBody) {
				t.Fatalf("body = %q, want %q", rr.Body.String(), tc.wantBody)
			}
			if st.createSpec.called || st.createGitSpecSnapshot.called || st.createMig.called || st.createRun.called {
				t.Fatal("catalog failure created durable rows")
			}
		})
	}
}

func TestRunsCreateSingleRepo_InvalidNamedSpecDoesNotCreateDurableRows(t *testing.T) {
	root := t.TempDir()
	writeNamedRunTestFile(t, filepath.Join(root, "invalid.yaml"), "apiVersion: ploy.mig/v1alpha1\nname: broken\nsteps: invalid\n")
	catalog := &runSpecCatalogStub{root: root, entry: speccatalog.Entry{
		Name: "broken", Source: "https://git.example.com/team/specs", Path: "invalid.yaml",
		SHA: "0123456789abcdef0123456789abcdef01234567", CommittedAt: time.Now(),
	}}
	st := &handlerStore{}
	handler := createSingleRepoRunHandler(st, nil, gitauth.Options{}, runSubmitSpecServices{catalog: catalog})

	rr := doRequest(t, handler, http.MethodPost, "/v1/runs", validRunRequestBodyWith(map[string]any{
		"spec": nil, "spec_selector": "broken",
	}))

	assertStatus(t, rr, http.StatusBadRequest)
	if st.createSpec.called || st.createGitSpecSnapshot.called || st.createMig.called || st.createRun.called {
		t.Fatal("compile failure created durable rows")
	}
}

func writeNamedRunTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
