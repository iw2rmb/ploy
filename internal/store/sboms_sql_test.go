package store

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestSBOMConstraint_ModulePackageIdentityDefinedInSchema(t *testing.T) {
	t.Parallel()

	schema := normalizeWhitespace(schemaSQL)
	wantTable := normalizeWhitespace("CREATE TABLE IF NOT EXISTS sboms")
	wantIdentity := normalizeWhitespace("ON sboms(job_id, repo_id, COALESCE(module_id, ''), lib, ver)")
	if !strings.Contains(schema, wantTable) {
		t.Fatalf("schema missing sboms table definition")
	}
	if !strings.Contains(schema, wantIdentity) {
		t.Fatalf("schema missing sboms package identity")
	}

	start := strings.Index(schema, normalizeWhitespace("CREATE TABLE IF NOT EXISTS sboms ("))
	if start == -1 {
		t.Fatalf("schema missing sboms table start")
	}
	tableSection := schema[start:]
	end := strings.Index(tableSection, ");")
	if end == -1 {
		t.Fatalf("schema missing sboms table end")
	}
	if strings.Contains(tableSection[:end], "created_at") {
		t.Fatalf("sboms table must not define created_at; time attribution must come from jobs.finished_at via join")
	}
}

func TestListRunSBOMRowsByJobType_CurrentAttemptScope(t *testing.T) {
	ctx, db := newTestStore(t)

	fx := newV1Fixture(t, ctx, db, "https://github.com/org/repo-sbom", "main", []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`))
	oldPre := createSBOMJobForStoreTest(t, ctx, db, fx.Run, types.JobTypePreGate, 1, "old-pre")
	insertSBOMForStoreTest(t, ctx, db, oldPre, fx.Run.RepoID, "old-lib", "0.1.0")

	if err := incrementRunAttempt(ctx, db.(*PgStore).Queries, fx.Run.ID, nil); err != nil {
		t.Fatalf("IncrementRunAttempt() failed: %v", err)
	}
	run, err := db.GetRun(ctx, fx.Run.ID)
	if err != nil {
		t.Fatalf("GetRun() failed: %v", err)
	}

	currentPre := createSBOMJobForStoreTest(t, ctx, db, run, types.JobTypePreGate, run.Attempt, "current-pre")
	currentPost := createSBOMJobForStoreTest(t, ctx, db, run, types.JobTypePostGate, run.Attempt, "current-post")
	currentMig := createSBOMJobForStoreTest(t, ctx, db, run, types.JobTypeMig, run.Attempt, "current-mig")
	insertSBOMForStoreTest(t, ctx, db, currentPre, run.RepoID, "alpha", "1.0.0")
	insertSBOMForStoreTest(t, ctx, db, currentPre, run.RepoID, "beta", "2.0.0")
	module := "app"
	if _, err := db.InsertSBOMRows(ctx, []InsertSBOMRowsParams{
		{JobID: currentPre.ID, RepoID: run.RepoID, ModuleID: &module, Lib: "alpha", Ver: "1.0.0"},
		{JobID: currentPre.ID, RepoID: run.RepoID, Lib: "beta", Ver: "2.0.0"},
	}); err != nil {
		t.Fatal(err)
	}
	insertSBOMForStoreTest(t, ctx, db, currentPost, run.RepoID, "post-only", "1.0.0")
	insertSBOMForStoreTest(t, ctx, db, currentMig, run.RepoID, "mig-only", "1.0.0")

	other := newV1Fixture(t, ctx, db, "https://github.com/org/repo-sbom-other", "main", []byte(`{"steps":[{"image":"docker.io/test/mig:latest"}]}`))
	otherPre := createSBOMJobForStoreTest(t, ctx, db, other.Run, types.JobTypePreGate, other.Run.Attempt, "other-pre")
	insertSBOMForStoreTest(t, ctx, db, otherPre, other.Run.RepoID, "other-lib", "9.0.0")

	rows, err := db.ListRunSBOMRowsByJobType(ctx, ListRunSBOMRowsByJobTypeParams{
		RunID:   run.ID,
		JobType: types.JobTypePreGate,
	})
	if err != nil {
		t.Fatalf("ListRunSBOMRowsByJobType() failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows len=%d, want 2: %+v", len(rows), rows)
	}
	if rows[0].Lib != "alpha" || rows[0].Ver != "1.0.0" || rows[1].Lib != "beta" || rows[1].Ver != "2.0.0" {
		t.Fatalf("rows=%+v, want alpha/beta current pre rows", rows)
	}
	if rows[0].ModuleID != "" || rows[1].ModuleID != "" {
		t.Fatalf("aggregate rows contain module_id: %v", rows)
	}
	scopedRows, err := db.ListRunSBOMRowsByJobType(ctx, ListRunSBOMRowsByJobTypeParams{RunID: run.ID, JobType: types.JobTypePreGate, ByModule: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []ListRunSBOMRowsByJobTypeRow{{ModuleID: "", Lib: "beta", Ver: "2.0.0"}, {ModuleID: ".", Lib: "alpha", Ver: "1.0.0"}, {ModuleID: ".", Lib: "beta", Ver: "2.0.0"}, {ModuleID: "app", Lib: "alpha", Ver: "1.0.0"}}
	if !slices.Equal(scopedRows, want) {
		t.Fatalf("module rows %v, want %v", scopedRows, want)
	}
	jobRows, err := db.ListSBOMRowsByJob(ctx, currentPre.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobRows) != len(want) {
		t.Fatalf("job rows %v, want %v", jobRows, want)
	}
	for i, row := range jobRows {
		if ListRunSBOMRowsByJobTypeRow(row) != want[i] {
			t.Fatalf("job row %v, want %v", row, want[i])
		}
	}
}

func createSBOMJobForStoreTest(t *testing.T, ctx context.Context, db Store, run Run, jobType types.JobType, attempt int32, name string) Job {
	t.Helper()
	job, err := db.CreateJob(ctx, CreateJobParams{
		ID:          types.NewJobID(),
		RunID:       run.ID,
		RepoID:      run.RepoID,
		RepoBaseRef: run.RepoBaseRef,
		Attempt:     attempt,
		Name:        name,
		Status:      types.JobStatusSuccess,
		JobType:     jobType,
		JobImage:    "test-image",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("CreateJob(%s) failed: %v", name, err)
	}
	return job
}

func insertSBOMForStoreTest(t *testing.T, ctx context.Context, db Store, job Job, repoID types.RepoID, lib, ver string) {
	t.Helper()
	root := "."
	if _, err := db.InsertSBOMRows(ctx, []InsertSBOMRowsParams{{
		ModuleID: &root,
		JobID:    job.ID,
		RepoID:   repoID,
		Lib:      lib,
		Ver:      ver,
	}}); err != nil {
		t.Fatalf("InsertSBOMRows(%s %s) failed: %v", lib, ver, err)
	}
}

func TestListSBOMJVMVersionsMatchesWithinEligibleJobs(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/org/jvm", "main", []byte(`{}`))
	run := fx.Run
	if err := incrementRunAttempt(ctx, db.(*PgStore).Queries, run.ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateRunStatus(ctx, UpdateRunStatusParams{ID: run.ID, Status: types.RunStatusFail}); err != nil {
		t.Fatal(err)
	}

	const java21 = `{"gate":{"detected_stack":{"language":"java","release":"21"}}}`
	for _, tc := range []struct {
		name     string
		jobType  types.JobType
		status   types.JobStatus
		meta     string
		age      string
		attempt  int32
		packages []string
	}{
		{"older attempt", "pre_gate", "Success", java21, "1 month", 1, []string{"g:a@1", "g:a@2", "g:b@1.20", "g:b@1.2.3", "g:c@exact", "g:d@3"}},
		{"current attempt", "post_gate", "Success", java21, "1 day", 2, []string{"g:a@2", "g:b@1.2.5", "g:c@exact", "g:d@4"}},
		{"split before", "pre_gate", "Success", java21, "1 day", 2, []string{"g:x@1"}},
		{"split after", "post_gate", "Success", java21, "1 day", 2, []string{"g:y@2"}},
		{"failed", "pre_gate", "Fail", java21, "1 day", 2, []string{"g:a@fail"}},
		{"running", "post_gate", "Running", java21, "1 day", 2, []string{"g:a@running"}},
		{"other type", "mig", "Success", java21, "1 day", 2, []string{"g:a@mig"}},
		{"too old", "pre_gate", "Success", java21, "10 months", 2, []string{"g:a@expired"}},
		{"missing metadata", "pre_gate", "Success", "{}", "1 day", 2, []string{"g:a@missing"}},
		{"another JVM", "pre_gate", "Success", strings.Replace(java21, "21", "17", 1), "1 day", 2, []string{"g:a@17"}},
		{"another language", "pre_gate", "Success", strings.Replace(java21, "java", "go", 1), "1 day", 2, []string{"g:a@go"}},
	} {
		job := createSBOMJobForStoreTest(t, ctx, db, run, tc.jobType, tc.attempt, tc.name)
		if _, err := db.Pool().Exec(ctx, `UPDATE jobs SET status=$2, meta=$3, finished_at=now()-$4::text::interval WHERE id=$1`, job.ID, tc.status, tc.meta, tc.age); err != nil {
			t.Fatal(err)
		}
		if tc.name == "older attempt" {
			alpha, beta := "api", "web"
			if _, err := db.InsertSBOMRows(ctx, []InsertSBOMRowsParams{
				{JobID: job.ID, RepoID: run.RepoID, ModuleID: &alpha, Lib: "g:shared", Ver: "1"},
				{JobID: job.ID, RepoID: run.RepoID, ModuleID: &beta, Lib: "g:shared", Ver: "1"},
				{JobID: job.ID, RepoID: run.RepoID, ModuleID: &alpha, Lib: "g:split", Ver: "api"},
				{JobID: job.ID, RepoID: run.RepoID, ModuleID: &beta, Lib: "g:split", Ver: "web"},
				{JobID: job.ID, RepoID: run.RepoID, Lib: "g:legacy", Ver: "1"},
				{JobID: job.ID, RepoID: run.RepoID, Lib: "g:legacy-peer", Ver: "2"},
			}); err != nil {
				t.Fatal(err)
			}
		}
		if tc.name == "current attempt" {
			if _, err := db.InsertSBOMRows(ctx, []InsertSBOMRowsParams{{JobID: job.ID, RepoID: run.RepoID, Lib: "g:other-legacy", Ver: "3"}}); err != nil {
				t.Fatal(err)
			}
		}
		for _, pkg := range tc.packages {
			lib, ver, _ := strings.Cut(pkg, "@")
			insertSBOMForStoreTest(t, ctx, db, job, run.RepoID, lib, ver)
		}
	}
	for _, tc := range []struct {
		name    string
		request string
		want    []string
	}{
		{"unconstrained", `[{"lib":"g:a","kind":"any","membership":"for"}]`, []string{"g:a@1 g:a@2|", "g:a@2|"}},
		{"project matching versions", `[{"lib":"g:a","kind":"any","membership":"for"},{"lib":"g:b","kind":"prefix","value":"1.2.","membership":"with"},{"lib":"g:c","kind":"exact","value":"exact","membership":"with"},{"lib":"g:d","kind":"any","membership":"with"}]`, []string{"g:a@1 g:a@2|g:b@1.2.3 g:c@exact g:d@3", "g:a@2|g:b@1.2.5 g:c@exact g:d@4"}},
		{"case sensitive", `[{"lib":"G:a","kind":"any","membership":"for"}]`, nil},
		{"matches in each module", `[{"lib":"g:split","kind":"any","membership":"for"},{"lib":"g:shared","kind":"any","membership":"with"}]`, []string{"g:split@api|g:shared@1", "g:split@web|g:shared@1"}},
		{"constraint in different module", `[{"lib":"g:a","kind":"any","membership":"for"},{"lib":"g:shared","kind":"any","membership":"with"}]`, nil},
		{"legacy rows from one job match", `[{"lib":"g:legacy","kind":"any","membership":"for"},{"lib":"g:legacy-peer","kind":"exact","value":"2","membership":"with"}]`, []string{"g:legacy@1|g:legacy-peer@2"}},
		{"legacy and attributed rows cannot satisfy one request", `[{"lib":"g:legacy","kind":"any","membership":"for"},{"lib":"g:shared","kind":"any","membership":"with"}]`, nil},
		{"different legacy jobs cannot satisfy one request", `[{"lib":"g:legacy","kind":"any","membership":"for"},{"lib":"g:other-legacy","kind":"any","membership":"with"}]`, nil},
		{"different jobs", `[{"lib":"g:x","kind":"any","membership":"for"},{"lib":"g:y","kind":"any","membership":"for"}]`, nil},
		{"exact boundary", `[{"lib":"g:a","kind":"any","membership":"for"},{"lib":"g:b","kind":"exact","value":"1.2","membership":"with"}]`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.ListSBOMJVMVersions(ctx, ListSBOMJVMVersionsParams{Jvm: "21", Requested: []byte(tc.request)})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range rows {
				slices.Sort(r.Found)
				slices.Sort(r.With)
				got = append(got, strings.Join(r.Found, " ")+"|"+strings.Join(r.With, " "))
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestListSBOMJVMVersionsIncludesExactUTCCutoffAcrossTimezones(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/org/cutoff", "main", []byte(`{}`))
	for _, name := range []string{"at", "before"} {
		job := createSBOMJobForStoreTest(t, ctx, db, fx.Run, types.JobTypePreGate, 1, name)
		insertSBOMForStoreTest(t, ctx, db, job, fx.Run.RepoID, "g:a", name)
	}

	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	// Derive both fixture instants in the lookup statement so elapsed test time cannot move the boundary.
	_, err = tx.Exec(ctx, `CREATE TEMP VIEW jobs AS
		SELECT id, job_type, status, '{"gate":{"detected_stack":{"language":"java","release":"21"}}}'::jsonb AS meta,
			((statement_timestamp() AT TIME ZONE 'UTC' - interval '9 months') AT TIME ZONE 'UTC')
			- CASE WHEN name='before' THEN interval '1 microsecond' ELSE interval '0' END AS finished_at
		FROM ploy.jobs`)
	if err != nil {
		t.Fatal(err)
	}
	for _, zone := range []string{"UTC", "America/New_York", "Pacific/Auckland"} {
		if _, err := tx.Exec(ctx, `SELECT set_config('TimeZone',$1,true)`, zone); err != nil {
			t.Fatal(err)
		}
		rows, err := New(tx).ListSBOMJVMVersions(ctx, ListSBOMJVMVersionsParams{Jvm: "21", Requested: []byte(`[{"lib":"g:a","kind":"any","membership":"for"}]`)})
		if err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || !slices.Equal(rows[0].Found, []string{"g:a@at"}) {
			t.Errorf("%s got %v, want only the exact cutoff", zone, rows)
		}
	}
}

func TestReplaceRunningGateSBOMPreservesRowsOnError(t *testing.T) {
	ctx, db := newTestStore(t)
	fx := newV1Fixture(t, ctx, db, "https://github.com/test/gate-sbom", "main", []byte("{}"))
	job := createSBOMJobForStoreTest(t, ctx, db, fx.Run, types.JobTypePreGate, 1, "pre")
	api, web := "api", "web"
	rows := []InsertSBOMRowsParams{{JobID: job.ID, RepoID: job.RepoID, ModuleID: &api, Lib: "g:lib", Ver: "1"}, {JobID: job.ID, RepoID: job.RepoID, ModuleID: &web, Lib: "g:lib", Ver: "1"}, {JobID: job.ID, RepoID: job.RepoID, Lib: "g:legacy", Ver: "2"}}
	replace := func(resume int, rows []InsertSBOMRowsParams) error {
		return db.WithJobExecution(ctx, job.ID, resume, func(scoped Store, locked Job) error {
			if err := scoped.DeleteSBOMRowsByJob(ctx, locked.ID); err != nil {
				return err
			}
			_, err := scoped.InsertSBOMRows(ctx, rows)
			return err
		})
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE runs SET stats='{"resume_count":2}' WHERE id=$1`, job.RunID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := replace(2, rows); err != nil {
			t.Fatal(err)
		}
	}
	if err := replace(2, []InsertSBOMRowsParams{rows[0], rows[0]}); err == nil {
		t.Fatal("expected duplicate package constraint error")
	}
	if err := replace(1, nil); !errors.Is(err, ErrJobExecutionStale) {
		t.Fatalf("stale upload: %v", err)
	}
	var modules, legacy int
	if err := db.Pool().QueryRow(ctx, `SELECT count(DISTINCT module_id),count(*) FILTER (WHERE module_id IS NULL) FROM sboms WHERE job_id=$1`, job.ID).Scan(&modules, &legacy); err != nil || modules != 2 || legacy != 1 {
		t.Fatalf("after failed replacements modules=%d, legacy=%d, err=%v", modules, legacy, err)
	}
}
