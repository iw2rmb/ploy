package store

// Domain-scoped store interfaces for handler-level dependency injection.
// Each interface captures the minimal set of store methods used by one domain.
// The full Store interface embeds Querier and satisfies all domain interfaces.

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

// ModStore provides mod project and mod-repo CRUD operations.
type ModStore interface {
	GetMod(ctx context.Context, id types.ModID) (Mod, error)
	GetModByName(ctx context.Context, name string) (Mod, error)
	CreateMod(ctx context.Context, arg CreateModParams) (Mod, error)
	ListMods(ctx context.Context, arg ListModsParams) ([]Mod, error)
	DeleteMod(ctx context.Context, id types.ModID) error
	ArchiveMod(ctx context.Context, id types.ModID) error
	UnarchiveMod(ctx context.Context, id types.ModID) error
	ListModReposByMod(ctx context.Context, modID types.ModID) ([]ModRepo, error)
	GetModRepo(ctx context.Context, id types.ModRepoID) (ModRepo, error)
	GetModRepoByURL(ctx context.Context, arg GetModRepoByURLParams) (ModRepo, error)
	CreateModRepo(ctx context.Context, arg CreateModRepoParams) (ModRepo, error)
	UpsertModRepo(ctx context.Context, arg UpsertModRepoParams) (ModRepo, error)
	DeleteModRepo(ctx context.Context, id types.ModRepoID) error
	HasModRepoHistory(ctx context.Context, repoID types.ModRepoID) (bool, error)
	UpdateModRepoRefs(ctx context.Context, arg UpdateModRepoRefsParams) error
	ListFailedRepoIDsByMod(ctx context.Context, modID types.ModID) ([]types.ModRepoID, error)
	ListDistinctRepos(ctx context.Context, filter string) ([]ListDistinctReposRow, error)
	ListRunsForRepo(ctx context.Context, arg ListRunsForRepoParams) ([]ListRunsForRepoRow, error)
}

// SpecStore provides spec CRUD and mod-spec linkage.
type SpecStore interface {
	CreateSpec(ctx context.Context, arg CreateSpecParams) (Spec, error)
	GetSpec(ctx context.Context, id types.SpecID) (Spec, error)
	UpdateModSpec(ctx context.Context, arg UpdateModSpecParams) error
}

// RunStore provides run lifecycle operations.
type RunStore interface {
	GetRun(ctx context.Context, id types.RunID) (Run, error)
	CreateRun(ctx context.Context, arg CreateRunParams) (Run, error)
	ListRuns(ctx context.Context, arg ListRunsParams) ([]Run, error)
	DeleteRun(ctx context.Context, id types.RunID) error
	UpdateRunStatus(ctx context.Context, arg UpdateRunStatusParams) error
	UpdateRunCompletion(ctx context.Context, arg UpdateRunCompletionParams) error
	UpdateRunResume(ctx context.Context, id types.RunID) error
	UpdateRunStatsMRURL(ctx context.Context, arg UpdateRunStatsMRURLParams) error
	CancelRunV1(ctx context.Context, runID types.RunID) error
	GetRunTiming(ctx context.Context, id types.RunID) (RunsTiming, error)
	ListRunsTimings(ctx context.Context, arg ListRunsTimingsParams) ([]RunsTiming, error)
	CountRunReposByStatus(ctx context.Context, runID types.RunID) ([]CountRunReposByStatusRow, error)
}

// RunRepoStore provides run-repo lifecycle operations.
type RunRepoStore interface {
	GetRunRepo(ctx context.Context, arg GetRunRepoParams) (RunRepo, error)
	CreateRunRepo(ctx context.Context, arg CreateRunRepoParams) (RunRepo, error)
	ListRunReposByRun(ctx context.Context, runID types.RunID) ([]RunRepo, error)
	ListQueuedRunReposByRun(ctx context.Context, runID types.RunID) ([]RunRepo, error)
	ListRunReposWithURLByRun(ctx context.Context, runID types.RunID) ([]ListRunReposWithURLByRunRow, error)
	UpdateRunRepoStatus(ctx context.Context, arg UpdateRunRepoStatusParams) error
	UpdateRunRepoRefs(ctx context.Context, arg UpdateRunRepoRefsParams) error
	UpdateRunRepoError(ctx context.Context, arg UpdateRunRepoErrorParams) error
	IncrementRunRepoAttempt(ctx context.Context, arg IncrementRunRepoAttemptParams) error
	GetLatestRunRepoByModAndRepoStatus(ctx context.Context, arg GetLatestRunRepoByModAndRepoStatusParams) (GetLatestRunRepoByModAndRepoStatusRow, error)
}

// JobStore provides job lifecycle operations.
type JobStore interface {
	GetJob(ctx context.Context, id types.JobID) (Job, error)
	CreateJob(ctx context.Context, arg CreateJobParams) (Job, error)
	ListJobsByRun(ctx context.Context, runID types.RunID) ([]Job, error)
	ListJobsByRunRepoAttempt(ctx context.Context, arg ListJobsByRunRepoAttemptParams) ([]Job, error)
	ListCreatedJobsByRunRepoAttempt(ctx context.Context, arg ListCreatedJobsByRunRepoAttemptParams) ([]Job, error)
	CountJobsByRun(ctx context.Context, runID types.RunID) (int64, error)
	CountJobsByRunAndStatus(ctx context.Context, arg CountJobsByRunAndStatusParams) (int64, error)
	CountJobsByRunRepoAttemptGroupByStatus(ctx context.Context, arg CountJobsByRunRepoAttemptGroupByStatusParams) ([]CountJobsByRunRepoAttemptGroupByStatusRow, error)
	UpdateJobStatus(ctx context.Context, arg UpdateJobStatusParams) error
	UpdateJobCompletion(ctx context.Context, arg UpdateJobCompletionParams) error
	UpdateJobCompletionWithMeta(ctx context.Context, arg UpdateJobCompletionWithMetaParams) error
	UpdateJobMeta(ctx context.Context, arg UpdateJobMetaParams) error
	UpdateJobImageName(ctx context.Context, arg UpdateJobImageNameParams) error
	ClaimJob(ctx context.Context, nodeID types.NodeID) (Job, error)
	ScheduleNextJob(ctx context.Context, arg ScheduleNextJobParams) (Job, error)
	GetAdjacentJobIndices(ctx context.Context, id types.JobID) (GetAdjacentJobIndicesRow, error)
}

// ArtifactStore provides artifact bundle operations.
type ArtifactStore interface {
	CreateArtifactBundle(ctx context.Context, arg CreateArtifactBundleParams) (ArtifactBundle, error)
	GetArtifactBundle(ctx context.Context, id pgtype.UUID) (ArtifactBundle, error)
	ListArtifactBundlesMetaByCID(ctx context.Context, cid *string) ([]ArtifactBundle, error)
	ListArtifactBundlesMetaByRun(ctx context.Context, runID types.RunID) ([]ArtifactBundle, error)
	ListArtifactBundlesMetaByRunAndJob(ctx context.Context, arg ListArtifactBundlesMetaByRunAndJobParams) ([]ArtifactBundle, error)
}

// NodeStore provides node management operations.
type NodeStore interface {
	GetNode(ctx context.Context, id types.NodeID) (Node, error)
	CreateNode(ctx context.Context, arg CreateNodeParams) (Node, error)
	ListNodes(ctx context.Context) ([]Node, error)
	UpdateNodeHeartbeat(ctx context.Context, arg UpdateNodeHeartbeatParams) error
	UpdateNodeDrained(ctx context.Context, arg UpdateNodeDrainedParams) error
	UpdateNodeCertMetadata(ctx context.Context, arg UpdateNodeCertMetadataParams) error
}

// AuthStore provides authentication token operations.
type AuthStore interface {
	InsertAPIToken(ctx context.Context, arg InsertAPITokenParams) error
	ListAPITokens(ctx context.Context, clusterID *string) ([]ListAPITokensRow, error)
	RevokeAPIToken(ctx context.Context, tokenID string) error
	InsertBootstrapToken(ctx context.Context, arg InsertBootstrapTokenParams) error
	GetBootstrapToken(ctx context.Context, tokenID string) (GetBootstrapTokenRow, error)
	CheckBootstrapTokenRevoked(ctx context.Context, tokenID string) (pgtype.Timestamptz, error)
	UpdateBootstrapTokenLastUsed(ctx context.Context, tokenID string) error
	MarkBootstrapTokenCertIssued(ctx context.Context, tokenID string) error
}

// ConfigStore provides global environment configuration operations.
type ConfigStore interface {
	ListGlobalEnv(ctx context.Context) ([]ConfigEnv, error)
	GetGlobalEnv(ctx context.Context, key string) (ConfigEnv, error)
	UpsertGlobalEnv(ctx context.Context, arg UpsertGlobalEnvParams) error
	DeleteGlobalEnv(ctx context.Context, key string) error
}
