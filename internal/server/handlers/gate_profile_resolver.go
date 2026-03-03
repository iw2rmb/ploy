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
	ResolveGateProfileForJob(ctx context.Context, job store.Job, constraints GateProfileLookupConstraints) (*GateProfileResolution, error)
}

type GateProfileResolution struct {
	ProfileID int64
	Payload   []byte
	ExactHit  bool
}

type GateProfileLookupConstraints struct {
	StrictStack *GateProfileLookupStack
}

type GateProfileLookupStack struct {
	Language string
	Tool     string
	Release  string
}

type gateProfileResolverStore interface {
	ResolveStackIDByImage(ctx context.Context, image string) (int64, error)
	ResolveStackIDByRequiredStack(ctx context.Context, language, tool, release string) (int64, error)
	ResolveStackIDByRepoSHA(ctx context.Context, repoID domaintypes.RepoID, repoSHA string) (int64, error)
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

func (r *dbGateProfileResolver) ResolveGateProfileForJob(ctx context.Context, job store.Job, constraints GateProfileLookupConstraints) (*GateProfileResolution, error) {
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if !sha40Pattern.MatchString(repoSHAIn) {
		return nil, nil
	}

	stackID, err := r.resolveStackID(ctx, job, constraints)
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

func normalizeStrictStackLookup(constraints GateProfileLookupConstraints) *GateProfileLookupStack {
	if constraints.StrictStack == nil {
		return nil
	}
	language := strings.TrimSpace(constraints.StrictStack.Language)
	release := strings.TrimSpace(constraints.StrictStack.Release)
	if language == "" || release == "" {
		return nil
	}
	return &GateProfileLookupStack{
		Language: language,
		Tool:     strings.TrimSpace(constraints.StrictStack.Tool),
		Release:  release,
	}
}

func (r *dbGateProfileResolver) resolveStackID(ctx context.Context, job store.Job, constraints GateProfileLookupConstraints) (int64, error) {
	if strict := normalizeStrictStackLookup(constraints); strict != nil {
		stackID, err := r.st.ResolveStackIDByRequiredStack(ctx, strict.Language, strict.Tool, strict.Release)
		if err == nil {
			return stackID, nil
		}
		return 0, err
	}

	if trimmed := strings.TrimSpace(job.JobImage); trimmed != "" {
		if stackID, err := r.st.ResolveStackIDByImage(ctx, trimmed); err == nil {
			return stackID, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
	}
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if repoSHAIn != "" {
		if stackID, err := r.st.ResolveStackIDByRepoSHA(ctx, job.RepoID, repoSHAIn); err == nil {
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
	return s.st.ResolveStackIDByImage(ctx, image)
}

func (s *sqlGateProfileResolverStore) ResolveStackIDByRequiredStack(ctx context.Context, language, tool, release string) (int64, error) {
	return s.st.ResolveStackIDByRequiredStack(ctx, store.ResolveStackIDByRequiredStackParams{
		Lang:    language,
		Tool:    tool,
		Release: release,
	})
}

func (s *sqlGateProfileResolverStore) ResolveStackIDByRepoSHA(ctx context.Context, repoID domaintypes.RepoID, repoSHA string) (int64, error) {
	return s.st.ResolveStackIDByRepoSHA(ctx, store.ResolveStackIDByRepoSHAParams{
		RepoID:  repoID.String(),
		RepoSha: repoSHA,
	})
}

func (s *sqlGateProfileResolverStore) ResolveAnyStackID(ctx context.Context) (int64, error) {
	return s.st.ResolveAnyStackID(ctx)
}

func (s *sqlGateProfileResolverStore) GetExactGateProfile(ctx context.Context, repoID domaintypes.RepoID, repoSHA string, stackID int64) (gateProfileRow, error) {
	got, err := s.st.GetExactGateProfile(ctx, store.GetExactGateProfileParams{
		RepoID:  repoID.String(),
		RepoSha: repoSHA,
		StackID: stackID,
	})
	if err != nil {
		return gateProfileRow{}, err
	}
	return gateProfileRow{
		ID:        got.ID,
		RepoID:    domaintypes.RepoID(got.RepoID),
		RepoSHA:   got.RepoSha,
		RepoSHA8:  got.RepoSha8,
		StackID:   got.StackID,
		ObjectKey: got.Url,
	}, nil
}

func (s *sqlGateProfileResolverStore) GetLatestRepoGateProfile(ctx context.Context, repoID domaintypes.RepoID, stackID int64) (gateProfileRow, error) {
	got, err := s.st.GetLatestRepoGateProfile(ctx, store.GetLatestRepoGateProfileParams{
		RepoID:  repoID.String(),
		StackID: stackID,
	})
	if err != nil {
		return gateProfileRow{}, err
	}
	return gateProfileRow{
		ID:        got.ID,
		RepoID:    domaintypes.RepoID(got.RepoID),
		RepoSHA:   got.RepoSha,
		RepoSHA8:  got.RepoSha8,
		StackID:   got.StackID,
		ObjectKey: got.Url,
	}, nil
}

func (s *sqlGateProfileResolverStore) GetDefaultGateProfile(ctx context.Context, stackID int64) (gateProfileRow, error) {
	got, err := s.st.GetDefaultGateProfile(ctx, stackID)
	if err != nil {
		return gateProfileRow{}, err
	}
	return gateProfileRow{
		ID:        got.ID,
		RepoID:    domaintypes.RepoID(got.RepoID),
		RepoSHA:   got.RepoSha,
		RepoSHA8:  got.RepoSha8,
		StackID:   got.StackID,
		ObjectKey: got.Url,
	}, nil
}

func (s *sqlGateProfileResolverStore) UpsertExactGateProfile(ctx context.Context, repoID domaintypes.RepoID, repoSHA string, stackID int64, objectKey string) (gateProfileRow, error) {
	got, err := s.st.UpsertExactGateProfile(ctx, store.UpsertExactGateProfileParams{
		RepoID:  repoID.String(),
		RepoSha: repoSHA,
		StackID: stackID,
		Url:     objectKey,
	})
	if err != nil {
		return gateProfileRow{}, err
	}
	return gateProfileRow{
		ID:        got.ID,
		RepoID:    domaintypes.RepoID(got.RepoID),
		RepoSHA:   got.RepoSha,
		RepoSHA8:  got.RepoSha8,
		StackID:   got.StackID,
		ObjectKey: got.Url,
	}, nil
}

func (s *sqlGateProfileResolverStore) UpsertGateJobProfileLink(ctx context.Context, jobID domaintypes.JobID, profileID int64) error {
	return s.st.UpsertGateJobProfileLink(ctx, store.UpsertGateJobProfileLinkParams{
		JobID:     jobID.String(),
		ProfileID: profileID,
	})
}
