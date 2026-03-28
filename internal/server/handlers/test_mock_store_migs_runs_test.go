package handlers

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/jackc/pgx/v5"
)

func (m *mockStore) CreateSpec(ctx context.Context, params store.CreateSpecParams) (store.Spec, error) {
	m.createSpecCalled = true
	m.createSpecParams = params

	result := m.createSpecResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.Spec == nil {
		result.Spec = params.Spec
	}
	result.CreatedBy = params.CreatedBy
	return result, m.createSpecErr
}

func (m *mockStore) CreateMig(ctx context.Context, params store.CreateMigParams) (store.Mig, error) {
	m.createMigCalled = true
	m.createMigParams = params

	result := m.createMigResult
	if result.ID.IsZero() {
		result.ID = params.ID
	}
	if result.Name == "" {
		result.Name = params.Name
	}
	result.SpecID = params.SpecID
	result.CreatedBy = params.CreatedBy
	return result, m.createMigErr
}

func (m *mockStore) UpdateMigSpec(ctx context.Context, params store.UpdateMigSpecParams) error {
	_, err := m.updateModSpec.record(params)
	return err
}

func (m *mockStore) ListMigs(ctx context.Context, params store.ListMigsParams) ([]store.Mig, error) {
	m.listMigsCalled = true
	m.listMigsParams = params
	// Simulate pagination: return empty list when offset exceeds available results.
	if int(params.Offset) >= len(m.listMigsResult) {
		return []store.Mig{}, m.listMigsErr
	}
	// Apply simple pagination simulation: return slice starting at offset, up to limit.
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listMigsResult) {
		end = len(m.listMigsResult)
	}
	return m.listMigsResult[params.Offset:end], m.listMigsErr
}

func (m *mockStore) GetMig(ctx context.Context, id types.MigID) (store.Mig, error) {
	m.getModCalled = true
	m.getModParam = id.String()
	if m.getModErr != nil {
		return store.Mig{}, m.getModErr
	}
	result := m.getModResult
	if result.ID.IsZero() {
		result.ID = id
	}
	if result.Name == "" {
		result.Name = "mig-" + id.String()
	}
	return result, nil
}

func (m *mockStore) GetMigByName(ctx context.Context, name string) (store.Mig, error) {
	m.getModByNameCalled = true
	m.getModByNameParam = name
	if m.getModByNameErr != nil {
		return store.Mig{}, m.getModByNameErr
	}

	// Default behavior: not found unless explicitly configured.
	result := m.getModByNameResult
	if result.ID.IsZero() && result.Name == "" {
		return store.Mig{}, pgx.ErrNoRows
	}
	if result.Name == "" {
		result.Name = name
	}
	return result, nil
}

func (m *mockStore) DeleteMig(ctx context.Context, id types.MigID) error {
	_, err := m.deleteMig.record(id.String())
	return err
}

func (m *mockStore) ArchiveMig(ctx context.Context, id types.MigID) error {
	_, err := m.archiveMig.record(id.String())
	return err
}

func (m *mockStore) UnarchiveMig(ctx context.Context, id types.MigID) error {
	_, err := m.unarchiveMig.record(id.String())
	return err
}

func (m *mockStore) CreateRun(ctx context.Context, params store.CreateRunParams) (store.Run, error) {
	m.createRunCalled = true
	m.createRunParams = params

	err := m.createRunErr
	if len(m.createRunErrs) > 0 {
		idx := m.createRunErrCallCount
		if idx >= len(m.createRunErrs) {
			idx = len(m.createRunErrs) - 1
		}
		m.createRunErrCallCount++
		err = m.createRunErrs[idx]
	}

	// When multiple results are configured, return them in order.
	if len(m.createRunResults) > 0 {
		idx := m.createRunCallCount
		if idx >= len(m.createRunResults) {
			idx = len(m.createRunResults) - 1
		}
		m.createRunCallCount++
		return m.createRunResults[idx], err
	}
	result := m.createRunResult
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
	return result, err
}

func (m *mockStore) GetRun(ctx context.Context, id types.RunID) (store.Run, error) {
	return m.getRun.record(id.String())
}

func (m *mockStore) GetSpec(ctx context.Context, id types.SpecID) (store.Spec, error) {
	return m.getSpec.record(id.String())
}

func (m *mockStore) GetRunTiming(ctx context.Context, id types.RunID) (store.RunsTiming, error) {
	return m.getRunTiming.record(id.String())
}

func (m *mockStore) ListRunsTimings(ctx context.Context, arg store.ListRunsTimingsParams) ([]store.RunsTiming, error) {
	return m.listRunsTimings.ret()
}

func (m *mockStore) DeleteRun(ctx context.Context, id types.RunID) error {
	_, err := m.deleteRun.record(id.String())
	return err
}

func (m *mockStore) ClaimRun(ctx context.Context, nodeID *string) (store.Run, error) {
	return m.claimRun.ret()
}

func (m *mockStore) AckRunStart(ctx context.Context, id string) error {
	return m.ackRunStart.err
}

func (m *mockStore) UpdateRunCompletion(ctx context.Context, params store.UpdateRunCompletionParams) error {
	return m.updateRunCompletion.err
}

func (m *mockStore) UpdateRunStatus(ctx context.Context, params store.UpdateRunStatusParams) error {
	_, err := m.updateRunStatus.record(params)
	return err
}

func (m *mockStore) CancelRunV1(ctx context.Context, runID types.RunID) error {
	_, err := m.cancelRunV1.record(runID.String())
	return err
}

func (m *mockStore) UpdateRunResume(ctx context.Context, id types.RunID) error {
	return m.updateRunResume.err
}

func (m *mockStore) UpdateRunStatsMRURL(ctx context.Context, params store.UpdateRunStatsMRURLParams) error {
	_, err := m.updateRunStatsMRURL.record(params)
	return err
}

func (m *mockStore) ListRuns(ctx context.Context, params store.ListRunsParams) ([]store.Run, error) {
	// Simulate pagination: return empty list when offset exceeds available results.
	if int(params.Offset) >= len(m.listRunsResult) {
		return []store.Run{}, m.listRunsErr
	}
	// Apply simple pagination simulation: return slice starting at offset, up to limit.
	end := int(params.Offset) + int(params.Limit)
	if end > len(m.listRunsResult) {
		end = len(m.listRunsResult)
	}
	return m.listRunsResult[params.Offset:end], m.listRunsErr
}

func (m *mockStore) ListRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listRunReposByRun.record(runID.String())
}

func (m *mockStore) CountRunReposByStatus(ctx context.Context, runID types.RunID) ([]store.CountRunReposByStatusRow, error) {
	return m.countRunReposByStatus.ret()
}

func (m *mockStore) UpdateRunRepoRefs(ctx context.Context, params store.UpdateRunRepoRefsParams) error {
	_, err := m.updateRunRepoRefs.record(params)
	return err
}

func (m *mockStore) UpdateRunRepoStatus(ctx context.Context, params store.UpdateRunRepoStatusParams) error {
	m.updateRunRepoStatusCalled = true
	m.updateRunRepoStatusParams = append(m.updateRunRepoStatusParams, params)
	return m.updateRunRepoStatusErr
}

func (m *mockStore) UpdateRunRepoError(ctx context.Context, params store.UpdateRunRepoErrorParams) error {
	_, err := m.updateRunRepoError.record(params)
	return err
}

func (m *mockStore) CreateRunRepo(ctx context.Context, params store.CreateRunRepoParams) (store.RunRepo, error) {
	m.createRunRepoCalled = true
	m.createRunRepoParams = params
	result := m.createRunRepoResult
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
	if result.RepoTargetRef == "" {
		result.RepoTargetRef = params.RepoTargetRef
	}
	if result.Status == "" {
		result.Status = types.RunRepoStatusQueued
	}
	if result.Attempt == 0 {
		result.Attempt = 1
	}
	return result, m.createRunRepoErr
}

func (m *mockStore) GetRunRepo(ctx context.Context, arg store.GetRunRepoParams) (store.RunRepo, error) {
	m.getRunRepoCalled = true
	m.getRunRepoParam = arg
	if m.getRunRepoErr != nil {
		return store.RunRepo{}, m.getRunRepoErr
	}
	if len(m.getRunRepoResults) > 0 {
		idx := m.getRunRepoCalls
		if idx >= len(m.getRunRepoResults) {
			idx = len(m.getRunRepoResults) - 1
		}
		m.getRunRepoCalls++
		return m.getRunRepoResults[idx], nil
	}
	return m.getRunRepoResult, nil
}

func (m *mockStore) IncrementRunRepoAttempt(ctx context.Context, arg store.IncrementRunRepoAttemptParams) error {
	_, err := m.incrementRunRepoAttempt.record(arg)
	return err
}

func (m *mockStore) ListQueuedRunReposByRun(ctx context.Context, runID types.RunID) ([]store.RunRepo, error) {
	return m.listQueuedRunReposByRun.record(runID.String())
}

func (m *mockStore) ListRunReposWithURLByRun(ctx context.Context, runID types.RunID) ([]store.ListRunReposWithURLByRunRow, error) {
	return m.listRunReposWithURLByRun.record(runID.String())
}

func (m *mockStore) GetLatestRunRepoByMigAndRepoStatus(ctx context.Context, arg store.GetLatestRunRepoByMigAndRepoStatusParams) (store.GetLatestRunRepoByMigAndRepoStatusRow, error) {
	return m.getLatestRunRepoByModAndRepoStatus.record(arg)
}
