package store

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// TestV1SQLCQueries_Mods verifies that the v1 migs queries are wired and match
// the expected filter semantics.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_Mods(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"

	modID1 := types.NewMigID()
	modName1 := "v1-sqlc-mig-alpha-" + modID1.String()
	mod1, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID1,
		Name:      modName1,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig(mod1) failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mod1.ID) }()

	modID2 := types.NewMigID()
	modName2 := "v1-sqlc-mig-beta-" + modID2.String()
	mod2, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID2,
		Name:      modName2,
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig(mod2) failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mod2.ID) }()

	gotByName, err := db.GetMigByName(ctx, modName1)
	if err != nil {
		t.Fatalf("GetMigByName() failed: %v", err)
	}
	if gotByName.ID != mod1.ID {
		t.Fatalf("GetMigByName() returned wrong mig id: got=%q want=%q", gotByName.ID, mod1.ID)
	}

	if err := db.ArchiveMig(ctx, mod2.ID); err != nil {
		t.Fatalf("ArchiveMig() failed: %v", err)
	}

	// List all: archived_only=nil.
	all, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: nil,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMigs(all) failed: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("ListMigs(all) returned too few rows: got=%d want>=2", len(all))
	}

	// Archived only.
	archivedOnly := true
	archived, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: &archivedOnly,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMigs(archived) failed: %v", err)
	}
	foundArchived := false
	for _, m := range archived {
		if m.ID == mod2.ID {
			foundArchived = true
		}
		if !m.ArchivedAt.Valid {
			t.Fatalf("ListMigs(archived) returned unarchived row: id=%q name=%q", m.ID, m.Name)
		}
	}
	if !foundArchived {
		t.Fatalf("ListMigs(archived) did not return expected archived mig: id=%q", mod2.ID)
	}

	// Active only.
	activeOnly := false
	active, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: &activeOnly,
		NameFilter:   nil,
	})
	if err != nil {
		t.Fatalf("ListMigs(active) failed: %v", err)
	}
	for _, m := range active {
		if m.ArchivedAt.Valid {
			t.Fatalf("ListMigs(active) returned archived row: id=%q name=%q", m.ID, m.Name)
		}
	}

	// Name filtering.
	nameFilter := "alpha"
	filtered, err := db.ListMigs(ctx, ListMigsParams{
		Limit:        100,
		Offset:       0,
		ArchivedOnly: nil,
		NameFilter:   &nameFilter,
	})
	if err != nil {
		t.Fatalf("ListMigs(name_filter) failed: %v", err)
	}
	foundAlpha := false
	for _, m := range filtered {
		if m.ID == mod1.ID {
			foundAlpha = true
		}
		if m.ID == mod2.ID {
			t.Fatalf("ListMigs(name_filter) unexpectedly included mod2: id=%q name=%q", m.ID, m.Name)
		}
	}
	if !foundAlpha {
		t.Fatalf("ListMigs(name_filter) did not return expected mod1: id=%q", mod1.ID)
	}

	if err := db.UnarchiveMig(ctx, mod2.ID); err != nil {
		t.Fatalf("UnarchiveMig() failed: %v", err)
	}
}

// TestV1SQLCQueries_ModRepos verifies v1 mig_repos lookup/upsert/delete behavior.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_ModRepos(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"
	modID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        modID,
		Name:      "v1-sqlc-mig-repos-" + modID.String(),
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mig.ID) }()

	repoURL := "https://github.com/iw2rmb/ploy-test-repo.git"

	repoID1 := types.NewMigRepoID()
	inserted, err := db.UpsertMigRepo(ctx, UpsertMigRepoParams{
		ID:        repoID1,
		MigID:     mig.ID,
		RepoUrl:   repoURL,
		BaseRef:   "main",
		TargetRef: "feature",
	})
	if err != nil {
		t.Fatalf("UpsertMigRepo(insert) failed: %v", err)
	}

	// Conflict path: provide a different id but same (mig_id, repo_url). ID must remain stable.
	repoID2 := types.NewMigRepoID()
	updated, err := db.UpsertMigRepo(ctx, UpsertMigRepoParams{
		ID:        repoID2,
		MigID:     mig.ID,
		RepoUrl:   repoURL,
		BaseRef:   "trunk",
		TargetRef: "feature-2",
	})
	if err != nil {
		t.Fatalf("UpsertMigRepo(update) failed: %v", err)
	}
	if updated.ID != inserted.ID {
		t.Fatalf("UpsertMigRepo(update) changed id: got=%q want=%q", updated.ID, inserted.ID)
	}
	if updated.BaseRef != "trunk" || updated.TargetRef != "feature-2" {
		t.Fatalf("UpsertMigRepo(update) did not update refs: base=%q target=%q", updated.BaseRef, updated.TargetRef)
	}

	got, err := db.GetMigRepoByURL(ctx, GetMigRepoByURLParams{
		MigID:   mig.ID,
		RepoUrl: repoURL,
	})
	if err != nil {
		t.Fatalf("GetMigRepoByURL() failed: %v", err)
	}
	if got.ID != inserted.ID {
		t.Fatalf("GetMigRepoByURL() returned wrong id: got=%q want=%q", got.ID, inserted.ID)
	}

	if err := db.DeleteMigRepo(ctx, inserted.ID); err != nil {
		t.Fatalf("DeleteMigRepo() failed: %v", err)
	}

	_, err = db.GetMigRepoByURL(ctx, GetMigRepoByURLParams{
		MigID:   mig.ID,
		RepoUrl: repoURL,
	})
	if err == nil {
		t.Fatal("expected GetMigRepoByURL() after DeleteMigRepo to fail, but it succeeded")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("expected pgx.ErrNoRows after DeleteMigRepo, got %T: %v", err, err)
	}
}

// TestV1SQLCQueries_Specs verifies ListSpecs ordering by created_at DESC.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_Specs(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"

	specID1 := types.NewSpecID()
	spec1, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID1,
		Name:      "v1-sqlc-spec-1",
		Spec:      []byte(`{"steps":[]}`),
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec(spec1) failed: %v", err)
	}
	defer func() { _, _ = db.Pool().Exec(ctx, "DELETE FROM specs WHERE id=$1", spec1.ID) }()

	specID2 := types.NewSpecID()
	spec2, err := db.CreateSpec(ctx, CreateSpecParams{
		ID:        specID2,
		Name:      "v1-sqlc-spec-2",
		Spec:      []byte(`{"steps":[]}`),
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateSpec(spec2) failed: %v", err)
	}
	defer func() { _, _ = db.Pool().Exec(ctx, "DELETE FROM specs WHERE id=$1", spec2.ID) }()

	older := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := db.Pool().Exec(ctx, "UPDATE specs SET created_at=$2 WHERE id=$1", spec1.ID, older); err != nil {
		t.Fatalf("set spec1 created_at failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE specs SET created_at=$2 WHERE id=$1", spec2.ID, newer); err != nil {
		t.Fatalf("set spec2 created_at failed: %v", err)
	}

	got, err := db.ListSpecs(ctx, ListSpecsParams{Limit: 100, Offset: 0})
	if err != nil {
		t.Fatalf("ListSpecs() failed: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("ListSpecs() returned too few rows: got=%d want>=2", len(got))
	}

	// Find our two specs and assert relative ordering.
	idx1, idx2 := -1, -1
	for i, s := range got {
		switch s.ID {
		case spec1.ID:
			idx1 = i
		case spec2.ID:
			idx2 = i
		}
	}
	if idx1 == -1 || idx2 == -1 {
		t.Fatalf("ListSpecs() did not include expected specs: idx1=%d idx2=%d", idx1, idx2)
	}
	if idx2 >= idx1 {
		t.Fatalf("expected spec2 (newer) to appear before spec1 (older): idx2=%d idx1=%d", idx2, idx1)
	}
}

// TestV1SQLCQueries_MigRepoPrepLifecycle verifies prep claim/state/profile queries.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_MigRepoPrepLifecycle(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"
	migID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        migID,
		Name:      "v1-sqlc-prep-" + migID.String(),
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mig.ID) }()

	repo1ID := types.NewMigRepoID()
	repo1, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        repo1ID,
		MigID:     mig.ID,
		RepoUrl:   "https://github.com/iw2rmb/ploy-prep-1.git",
		BaseRef:   "main",
		TargetRef: "feature-1",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(repo1) failed: %v", err)
	}
	if repo1.PrepStatus != PrepStatusPending {
		t.Fatalf("repo1 prep status mismatch: got=%q want=%q", repo1.PrepStatus, PrepStatusPending)
	}
	if repo1.PrepAttempts != 0 {
		t.Fatalf("repo1 prep attempts mismatch: got=%d want=0", repo1.PrepAttempts)
	}

	repo2ID := types.NewMigRepoID()
	repo2, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        repo2ID,
		MigID:     mig.ID,
		RepoUrl:   "https://github.com/iw2rmb/ploy-prep-2.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(repo2) failed: %v", err)
	}

	oldest := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	secondOldest := oldest.Add(time.Second)
	if _, err := db.Pool().Exec(ctx, "UPDATE mig_repos SET prep_updated_at=$2 WHERE id=$1", repo1.ID, oldest); err != nil {
		t.Fatalf("set repo1 prep_updated_at failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE mig_repos SET prep_updated_at=$2 WHERE id=$1", repo2.ID, secondOldest); err != nil {
		t.Fatalf("set repo2 prep_updated_at failed: %v", err)
	}

	claimed1, err := db.ClaimNextPrepRepo(ctx)
	if err != nil {
		t.Fatalf("ClaimNextPrepRepo(1) failed: %v", err)
	}
	if claimed1.ID != repo1.ID {
		t.Fatalf("ClaimNextPrepRepo(1) claimed wrong repo: got=%q want=%q", claimed1.ID, repo1.ID)
	}
	if claimed1.PrepStatus != PrepStatusRunning {
		t.Fatalf("claimed1 prep status mismatch: got=%q want=%q", claimed1.PrepStatus, PrepStatusRunning)
	}
	if claimed1.PrepAttempts != 1 {
		t.Fatalf("claimed1 prep attempts mismatch: got=%d want=1", claimed1.PrepAttempts)
	}

	claimed2, err := db.ClaimNextPrepRepo(ctx)
	if err != nil {
		t.Fatalf("ClaimNextPrepRepo(2) failed: %v", err)
	}
	if claimed2.ID != repo2.ID {
		t.Fatalf("ClaimNextPrepRepo(2) claimed wrong repo: got=%q want=%q", claimed2.ID, repo2.ID)
	}

	prepErr := "prep command failed"
	failureCode := "command_not_found"
	if err := db.UpdateMigRepoPrepState(ctx, UpdateMigRepoPrepStateParams{
		ID:              repo1.ID,
		PrepStatus:      PrepStatusFailed,
		PrepLastError:   &prepErr,
		PrepFailureCode: &failureCode,
	}); err != nil {
		t.Fatalf("UpdateMigRepoPrepState() failed: %v", err)
	}

	gotRepo1, err := db.GetMigRepo(ctx, repo1.ID)
	if err != nil {
		t.Fatalf("GetMigRepo(repo1) failed: %v", err)
	}
	if gotRepo1.PrepStatus != PrepStatusFailed {
		t.Fatalf("repo1 final prep status mismatch: got=%q want=%q", gotRepo1.PrepStatus, PrepStatusFailed)
	}
	if gotRepo1.PrepLastError == nil || *gotRepo1.PrepLastError != prepErr {
		t.Fatalf("repo1 prep_last_error mismatch: got=%v want=%q", gotRepo1.PrepLastError, prepErr)
	}
	if gotRepo1.PrepFailureCode == nil || *gotRepo1.PrepFailureCode != failureCode {
		t.Fatalf("repo1 prep_failure_code mismatch: got=%v want=%q", gotRepo1.PrepFailureCode, failureCode)
	}

	profile := []byte(`{"schema_version":1}`)
	artifacts := []byte(`{"log_refs":["logs://prep/run"]}`)
	if err := db.SaveMigRepoPrepProfile(ctx, SaveMigRepoPrepProfileParams{
		ID:            repo2.ID,
		PrepProfile:   profile,
		PrepArtifacts: artifacts,
	}); err != nil {
		t.Fatalf("SaveMigRepoPrepProfile() failed: %v", err)
	}

	gotRepo2, err := db.GetMigRepo(ctx, repo2.ID)
	if err != nil {
		t.Fatalf("GetMigRepo(repo2) failed: %v", err)
	}
	if gotRepo2.PrepStatus != PrepStatusReady {
		t.Fatalf("repo2 final prep status mismatch: got=%q want=%q", gotRepo2.PrepStatus, PrepStatusReady)
	}
	if !bytes.Equal(gotRepo2.PrepProfile, profile) {
		t.Fatalf("repo2 prep_profile mismatch: got=%s want=%s", string(gotRepo2.PrepProfile), string(profile))
	}
	if !bytes.Equal(gotRepo2.PrepArtifacts, artifacts) {
		t.Fatalf("repo2 prep_artifacts mismatch: got=%s want=%s", string(gotRepo2.PrepArtifacts), string(artifacts))
	}
}

// TestV1SQLCQueries_PrepRuns verifies attempt-level prep evidence persistence.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_PrepRuns(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"
	migID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        migID,
		Name:      "v1-sqlc-prep-runs-" + migID.String(),
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mig.ID) }()

	repoID := types.NewMigRepoID()
	repo, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        repoID,
		MigID:     mig.ID,
		RepoUrl:   "https://github.com/iw2rmb/ploy-prep-runs.git",
		BaseRef:   "main",
		TargetRef: "feature",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo() failed: %v", err)
	}

	started, err := db.CreatePrepRun(ctx, CreatePrepRunParams{
		RepoID:     repo.ID,
		Attempt:    1,
		Status:     PrepStatusRunning,
		ResultJson: []byte(`{"phase":"init"}`),
		LogsRef:    nil,
	})
	if err != nil {
		t.Fatalf("CreatePrepRun() failed: %v", err)
	}
	if started.RepoID != repo.ID {
		t.Fatalf("CreatePrepRun repo mismatch: got=%q want=%q", started.RepoID, repo.ID)
	}
	if started.Status != PrepStatusRunning {
		t.Fatalf("CreatePrepRun status mismatch: got=%q want=%q", started.Status, PrepStatusRunning)
	}
	if !started.StartedAt.Valid {
		t.Fatal("CreatePrepRun started_at must be set")
	}
	if started.FinishedAt.Valid {
		t.Fatal("CreatePrepRun finished_at must be NULL")
	}

	logsRef := "logs://prep/attempt-1"
	finishedPayload := []byte(`{"failure_code":"timeout"}`)
	finished, err := db.FinishPrepRun(ctx, FinishPrepRunParams{
		RepoID:     repo.ID,
		Attempt:    1,
		Status:     PrepStatusFailed,
		ResultJson: finishedPayload,
		LogsRef:    &logsRef,
	})
	if err != nil {
		t.Fatalf("FinishPrepRun() failed: %v", err)
	}
	if finished.Status != PrepStatusFailed {
		t.Fatalf("FinishPrepRun status mismatch: got=%q want=%q", finished.Status, PrepStatusFailed)
	}
	if !finished.FinishedAt.Valid {
		t.Fatal("FinishPrepRun finished_at must be set")
	}
	if !bytes.Equal(finished.ResultJson, finishedPayload) {
		t.Fatalf("FinishPrepRun result_json mismatch: got=%s want=%s", string(finished.ResultJson), string(finishedPayload))
	}
	if finished.LogsRef == nil || *finished.LogsRef != logsRef {
		t.Fatalf("FinishPrepRun logs_ref mismatch: got=%v want=%q", finished.LogsRef, logsRef)
	}
}

// TestV1SQLCQueries_ClaimNextPrepRetryRepo verifies retry-eligible prep claim ordering.
//
// This test is skipped if PLOY_TEST_PG_DSN is not set.
func TestV1SQLCQueries_ClaimNextPrepRetryRepo(t *testing.T) {
	dsn := os.Getenv("PLOY_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PLOY_TEST_PG_DSN not set; skipping store integration test")
	}

	ctx := context.Background()
	db, err := NewStore(ctx, dsn)
	if err != nil {
		t.Fatalf("NewStore() failed: %v", err)
	}
	defer db.Close()

	createdBy := "test-user"
	migID := types.NewMigID()
	mig, err := db.CreateMig(ctx, CreateMigParams{
		ID:        migID,
		Name:      "v1-sqlc-prep-retry-" + migID.String(),
		SpecID:    nil,
		CreatedBy: &createdBy,
	})
	if err != nil {
		t.Fatalf("CreateMig() failed: %v", err)
	}
	defer func() { _ = db.DeleteMig(ctx, mig.ID) }()

	repo1, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        types.NewMigRepoID(),
		MigID:     mig.ID,
		RepoUrl:   "https://github.com/iw2rmb/ploy-prep-retry-1.git",
		BaseRef:   "main",
		TargetRef: "feature-1",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(repo1) failed: %v", err)
	}

	repo2, err := db.CreateMigRepo(ctx, CreateMigRepoParams{
		ID:        types.NewMigRepoID(),
		MigID:     mig.ID,
		RepoUrl:   "https://github.com/iw2rmb/ploy-prep-retry-2.git",
		BaseRef:   "main",
		TargetRef: "feature-2",
	})
	if err != nil {
		t.Fatalf("CreateMigRepo(repo2) failed: %v", err)
	}

	retryErr := "retry me"
	retryCode := "timeout"
	if err := db.UpdateMigRepoPrepState(ctx, UpdateMigRepoPrepStateParams{
		ID:              repo1.ID,
		PrepStatus:      PrepStatusRetryScheduled,
		PrepLastError:   &retryErr,
		PrepFailureCode: &retryCode,
	}); err != nil {
		t.Fatalf("UpdateMigRepoPrepState(repo1) failed: %v", err)
	}
	if err := db.UpdateMigRepoPrepState(ctx, UpdateMigRepoPrepStateParams{
		ID:              repo2.ID,
		PrepStatus:      PrepStatusRetryScheduled,
		PrepLastError:   &retryErr,
		PrepFailureCode: &retryCode,
	}); err != nil {
		t.Fatalf("UpdateMigRepoPrepState(repo2) failed: %v", err)
	}

	oldest := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := oldest.Add(10 * time.Second)
	if _, err := db.Pool().Exec(ctx, "UPDATE mig_repos SET prep_updated_at=$2 WHERE id=$1", repo1.ID, oldest); err != nil {
		t.Fatalf("set repo1 prep_updated_at failed: %v", err)
	}
	if _, err := db.Pool().Exec(ctx, "UPDATE mig_repos SET prep_updated_at=$2 WHERE id=$1", repo2.ID, newer); err != nil {
		t.Fatalf("set repo2 prep_updated_at failed: %v", err)
	}

	cutoff := pgtype.Timestamptz{Time: oldest.Add(5 * time.Second), Valid: true}
	claimed, err := db.ClaimNextPrepRetryRepo(ctx, cutoff)
	if err != nil {
		t.Fatalf("ClaimNextPrepRetryRepo() failed: %v", err)
	}
	if claimed.ID != repo1.ID {
		t.Fatalf("ClaimNextPrepRetryRepo() claimed wrong repo: got=%q want=%q", claimed.ID, repo1.ID)
	}
	if claimed.PrepStatus != PrepStatusRunning {
		t.Fatalf("claimed status mismatch: got=%q want=%q", claimed.PrepStatus, PrepStatusRunning)
	}
	if claimed.PrepAttempts != 1 {
		t.Fatalf("claimed attempts mismatch: got=%d want=1", claimed.PrepAttempts)
	}

	_, err = db.ClaimNextPrepRetryRepo(ctx, cutoff)
	if err == nil {
		t.Fatal("ClaimNextPrepRetryRepo() expected no eligible rows")
	}
	if err != pgx.ErrNoRows {
		t.Fatalf("ClaimNextPrepRetryRepo() error = %v, want %v", err, pgx.ErrNoRows)
	}
}
