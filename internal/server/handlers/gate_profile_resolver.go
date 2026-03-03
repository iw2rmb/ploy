package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

type GateProfileResolver interface {
	ResolveGateProfileForJob(ctx context.Context, job store.Job) (*GateProfileResolution, error)
}

type GateProfileResolution struct {
	ProfileID int64
	Payload   []byte
	ExactHit  bool
}

type gateProfileResolverStore interface {
	ResolveStackIDByImage(ctx context.Context, image string) (int64, error)
	ResolveAnyStackID(ctx context.Context) (int64, error)
	GetExactGateProfile(ctx context.Context, repoID domaintypes.RepoID, repoSHA string, stackID int64) (gateProfileRow, error)
	GetLatestRepoGateProfile(ctx context.Context, repoID domaintypes.RepoID, stackID int64) (gateProfileRow, error)
	GetDefaultGateProfile(ctx context.Context, stackID int64) (gateProfileRow, error)
	UpsertExactGateProfile(ctx context.Context, repoID domaintypes.RepoID, repoSHA string, stackID int64, objectKey string) (gateProfileRow, error)
	UpsertGateJobProfileLink(ctx context.Context, jobID domaintypes.JobID, profileID int64) error
}

type gateProfileRow struct {
	ID        int64
	RepoID    domaintypes.RepoID
	RepoSHA   string
	RepoSHA8  string
	StackID   int64
	ObjectKey string
}

type dbGateProfileResolver struct {
	st gateProfileResolverStore
	bs blobstore.Store
}

func NewDBGateProfileResolver(st store.Store, bs blobstore.Store) GateProfileResolver {
	if st == nil || bs == nil {
		return nil
	}
	return &dbGateProfileResolver{
		st: &sqlGateProfileResolverStore{st: st},
		bs: bs,
	}
}

func (r *dbGateProfileResolver) ResolveGateProfileForJob(ctx context.Context, job store.Job) (*GateProfileResolution, error) {
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if !sha40Pattern.MatchString(repoSHAIn) {
		return nil, nil
	}

	stackID, err := r.resolveStackID(ctx, job.JobImage)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve stack id: %w", err)
	}

	exact, err := r.st.GetExactGateProfile(ctx, job.RepoID, repoSHAIn, stackID)
	if err == nil {
		payload, loadErr := r.loadObject(ctx, exact.ObjectKey)
		if loadErr != nil {
			return nil, fmt.Errorf("load exact gate profile %d: %w", exact.ID, loadErr)
		}
		if linkErr := r.st.UpsertGateJobProfileLink(ctx, job.ID, exact.ID); linkErr != nil {
			return nil, fmt.Errorf("upsert gates link: %w", linkErr)
		}
		return &GateProfileResolution{
			ProfileID: exact.ID,
			Payload:   payload,
			ExactHit:  true,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup exact gate profile: %w", err)
	}

	fallback, err := r.st.GetLatestRepoGateProfile(ctx, job.RepoID, stackID)
	if errors.Is(err, pgx.ErrNoRows) {
		fallback, err = r.st.GetDefaultGateProfile(ctx, stackID)
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup fallback gate profile: %w", err)
	}

	payload, err := r.loadObject(ctx, fallback.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("load fallback gate profile %d: %w", fallback.ID, err)
	}

	objectKey := fmt.Sprintf(
		"gate-profiles/repos/%s/%s/stack-%d/profile-%d.json",
		job.RepoID.String(),
		repoSHAIn,
		stackID,
		time.Now().UTC().UnixNano(),
	)
	if _, err := r.bs.Put(ctx, objectKey, "application/json", payload); err != nil {
		return nil, fmt.Errorf("copy fallback gate profile blob: %w", err)
	}

	exact, err = r.st.UpsertExactGateProfile(ctx, job.RepoID, repoSHAIn, stackID, objectKey)
	if err != nil {
		return nil, fmt.Errorf("upsert exact gate profile: %w", err)
	}
	if linkErr := r.st.UpsertGateJobProfileLink(ctx, job.ID, exact.ID); linkErr != nil {
		return nil, fmt.Errorf("upsert gates link: %w", linkErr)
	}
	return &GateProfileResolution{
		ProfileID: exact.ID,
		Payload:   payload,
		ExactHit:  false,
	}, nil
}

func (r *dbGateProfileResolver) resolveStackID(ctx context.Context, image string) (int64, error) {
	if trimmed := strings.TrimSpace(image); trimmed != "" {
		if stackID, err := r.st.ResolveStackIDByImage(ctx, trimmed); err == nil {
			return stackID, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
	}
	return r.st.ResolveAnyStackID(ctx)
}

func (r *dbGateProfileResolver) loadObject(ctx context.Context, key string) ([]byte, error) {
	rc, _, err := r.bs.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rc.Close()
	}()
	return io.ReadAll(rc)
}

type sqlGateProfileResolverStore struct {
	st store.Store
}

func (s *sqlGateProfileResolverStore) ResolveStackIDByImage(ctx context.Context, image string) (int64, error) {
	var id int64
	err := s.st.Pool().QueryRow(ctx, `
SELECT id
FROM stacks
WHERE image = $1
ORDER BY id ASC
LIMIT 1
`, image).Scan(&id)
	return id, err
}

func (s *sqlGateProfileResolverStore) ResolveAnyStackID(ctx context.Context) (int64, error) {
	var id int64
	err := s.st.Pool().QueryRow(ctx, `
SELECT id
FROM stacks
ORDER BY id ASC
LIMIT 1
`).Scan(&id)
	return id, err
}

func (s *sqlGateProfileResolverStore) GetExactGateProfile(ctx context.Context, repoID domaintypes.RepoID, repoSHA string, stackID int64) (gateProfileRow, error) {
	var row gateProfileRow
	err := s.st.Pool().QueryRow(ctx, `
SELECT id, COALESCE(repo_id, ''), COALESCE(repo_sha, ''), COALESCE(repo_sha8, ''), stack_id, url
FROM gate_profiles
WHERE repo_id = $1
  AND repo_sha = $2
  AND stack_id = $3
LIMIT 1
`, repoID, repoSHA, stackID).Scan(
		&row.ID,
		&row.RepoID,
		&row.RepoSHA,
		&row.RepoSHA8,
		&row.StackID,
		&row.ObjectKey,
	)
	return row, err
}

func (s *sqlGateProfileResolverStore) GetLatestRepoGateProfile(ctx context.Context, repoID domaintypes.RepoID, stackID int64) (gateProfileRow, error) {
	var row gateProfileRow
	err := s.st.Pool().QueryRow(ctx, `
SELECT id, COALESCE(repo_id, ''), COALESCE(repo_sha, ''), COALESCE(repo_sha8, ''), stack_id, url
FROM gate_profiles
WHERE repo_id = $1
  AND stack_id = $2
ORDER BY updated_at DESC, id DESC
LIMIT 1
`, repoID, stackID).Scan(
		&row.ID,
		&row.RepoID,
		&row.RepoSHA,
		&row.RepoSHA8,
		&row.StackID,
		&row.ObjectKey,
	)
	return row, err
}

func (s *sqlGateProfileResolverStore) GetDefaultGateProfile(ctx context.Context, stackID int64) (gateProfileRow, error) {
	var row gateProfileRow
	err := s.st.Pool().QueryRow(ctx, `
SELECT id, COALESCE(repo_id, ''), COALESCE(repo_sha, ''), COALESCE(repo_sha8, ''), stack_id, url
FROM gate_profiles
WHERE repo_id IS NULL
  AND repo_sha IS NULL
  AND stack_id = $1
ORDER BY updated_at DESC, id DESC
LIMIT 1
`, stackID).Scan(
		&row.ID,
		&row.RepoID,
		&row.RepoSHA,
		&row.RepoSHA8,
		&row.StackID,
		&row.ObjectKey,
	)
	return row, err
}

func (s *sqlGateProfileResolverStore) UpsertExactGateProfile(ctx context.Context, repoID domaintypes.RepoID, repoSHA string, stackID int64, objectKey string) (gateProfileRow, error) {
	var row gateProfileRow
	err := s.st.Pool().QueryRow(ctx, `
INSERT INTO gate_profiles (
  repo_id,
  repo_sha,
  repo_sha8,
  stack_id,
  url
)
VALUES ($1, $2, SUBSTRING($2, 1, 8), $3, $4)
ON CONFLICT (repo_id, repo_sha, stack_id)
DO UPDATE SET
  url = EXCLUDED.url,
  repo_sha8 = EXCLUDED.repo_sha8,
  updated_at = NOW()
RETURNING id, COALESCE(repo_id, ''), COALESCE(repo_sha, ''), COALESCE(repo_sha8, ''), stack_id, url
`, repoID, repoSHA, stackID, objectKey).Scan(
		&row.ID,
		&row.RepoID,
		&row.RepoSHA,
		&row.RepoSHA8,
		&row.StackID,
		&row.ObjectKey,
	)
	return row, err
}

func (s *sqlGateProfileResolverStore) UpsertGateJobProfileLink(ctx context.Context, jobID domaintypes.JobID, profileID int64) error {
	_, err := s.st.Pool().Exec(ctx, `
INSERT INTO gates (job_id, profile_id)
VALUES ($1, $2)
ON CONFLICT (job_id)
DO UPDATE SET profile_id = EXCLUDED.profile_id
`, jobID, profileID)
	return err
}
