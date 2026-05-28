package handlers

import (
	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

func buildCreateJobResult(result store.Job, params store.CreateJobParams) store.Job {
	if result.ID.IsZero() {
		result.ID = types.NewJobID()
	}
	result.RunID = params.RunID
	result.RepoID = params.RepoID
	result.RepoBaseRef = params.RepoBaseRef
	result.Attempt = params.Attempt
	result.Name = params.Name
	result.Status = params.Status
	result.JobType = params.JobType
	result.JobImage = params.JobImage
	result.NextID = params.NextID
	result.RepoShaIn = params.RepoShaIn
	result.Meta = params.Meta
	return result
}

// listPaged returns the (offset, limit) slice of items, mirroring SQL paging
// semantics used by all sqlc-generated ListX(params) queries.
func listPaged[T any](items []T, offset, limit int32) []T {
	if int(offset) >= len(items) {
		return []T{}
	}
	end := int(offset) + int(limit)
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}

// defaultRunRepo fills zero-valued result fields from CreateRunRepoParams,
// matching the defaulting semantics shared by runStore and migStore mocks.
func defaultRunRepo(result store.RunRepo, params store.CreateRunRepoParams) store.RunRepo {
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.RunID.IsZero() {
		result.RunID = params.RunID
	}
	if result.RepoID.IsZero() {
		result.RepoID = params.RepoID
	}
	if result.RepoBaseRef == "" {
		result.RepoBaseRef = params.RepoBaseRef
	}
	if result.Status == "" {
		result.Status = types.RunRepoStatusQueued
	}
	if result.Attempt == 0 {
		result.Attempt = 1
	}
	return result
}

// defaultMigRepo fills zero-valued result fields from the MigRepo input shape
// shared by CreateMigRepoParams and UpsertMigRepoParams.
func defaultMigRepo(result store.MigRepo, id types.MigRepoID, migID types.MigID, baseRef string) store.MigRepo {
	if result.ID.IsZero() {
		result.ID = id
	}
	if result.MigID.IsZero() {
		result.MigID = migID
	}
	if result.RepoID.IsZero() {
		result.RepoID = types.NewRepoID()
	}
	if result.BaseRef == "" {
		result.BaseRef = baseRef
	}
	return result
}

// defaultRun fills zero-valued result fields from CreateRunParams.
func defaultRun(result store.Run, params store.CreateRunParams) store.Run {
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.MigID.IsZero() {
		result.MigID = params.MigID
	}
	if result.SpecID.IsZero() {
		result.SpecID = params.SpecID
	}
	result.CreatedBy = params.CreatedBy
	return result
}

// defaultRepo returns a synthetic Repo for a non-zero id, mirroring the
// convention used by both runStore and migStore GetRepo mocks. Callers should
// consult their own repoByID override map before falling back here.
func defaultRepo(id types.RepoID) (store.Repo, error) {
	if !id.IsZero() {
		return store.Repo{ID: id, Url: "https://github.com/user/repo.git"}, nil
	}
	return store.Repo{}, pgx.ErrNoRows
}
