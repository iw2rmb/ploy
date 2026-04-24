package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/jackc/pgx/v5"
)

type gateProfileStackRow struct {
	ID      int64
	Lang    string
	Tool    string
	Release string
}

func persistSuccessfulGateProfile(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	job store.Job,
	rawMeta []byte,
	specRaw []byte,
) error {
	if bs == nil {
		return nil
	}
	jobType := domaintypes.JobType(job.JobType)
	if jobType != domaintypes.JobTypePreGate &&
		jobType != domaintypes.JobTypePostGate {
		return nil
	}

	meta, err := contracts.UnmarshalJobMeta(rawMeta)
	if err != nil || meta == nil || meta.GateMetadata == nil {
		return nil
	}
	if len(meta.GateMetadata.StaticChecks) == 0 || !meta.GateMetadata.StaticChecks[0].Passed {
		return nil
	}

	repoSHA, err := resolveGateProfileRepoSHA(job)
	if err != nil {
		return err
	}

	target, err := resolveGateProfileTarget(specRaw, jobType)
	if err != nil {
		return err
	}

	stackRow, err := resolveGateProfileStackRow(ctx, st, job, meta.GateMetadata)
	if err != nil {
		return err
	}

	payload, err := buildSuccessfulGateProfilePayload(job.RepoID, target, stackRow, meta.GateMetadata)
	if err != nil {
		return err
	}
	return persistGateProfilePayload(ctx, st, bs, job, repoSHA, stackRow.ID, payload)
}

func resolveGateProfileRepoSHA(job store.Job) (string, error) {
	repoSHAOut := strings.TrimSpace(job.RepoShaOut)
	if sha40Pattern.MatchString(repoSHAOut) {
		return repoSHAOut, nil
	}
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if sha40Pattern.MatchString(repoSHAIn) {
		return repoSHAIn, nil
	}
	return "", fmt.Errorf("gate profile persistence requires repo_sha_out or repo_sha_in")
}

func resolveGateProfileTarget(specRaw []byte, jobType domaintypes.JobType) (string, error) {
	target := contracts.GateProfileTargetAllTests
	phasePolicy, err := gatePhasePolicyForJobSpec(specRaw, jobType)
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(phasePolicy.Target) {
	case "":
		return target, nil
	case contracts.GateProfileTargetBuild, contracts.GateProfileTargetUnit, contracts.GateProfileTargetAllTests:
		return strings.TrimSpace(phasePolicy.Target), nil
	default:
		return "", fmt.Errorf("unsupported build_gate target %q for successful gate profile persistence", phasePolicy.Target)
	}
}

func resolveGateProfileStackRow(
	ctx context.Context,
	st store.Store,
	job store.Job,
	gateMeta *contracts.BuildGateStageMetadata,
) (gateProfileStackRow, error) {
	exp := gateMeta.DetectedStackExpectation()

	if exp != nil {
		lang := strings.TrimSpace(exp.Language)
		tool := strings.TrimSpace(exp.Tool)
		release := strings.TrimSpace(exp.Release)
		if lang != "" && tool != "" && release != "" {
			row, err := queryStackRowByExpectation(ctx, st, lang, tool, release)
			if err == nil {
				return row, nil
			}
			if err != nil && err != pgx.ErrNoRows {
				return gateProfileStackRow{}, err
			}
		}
		if lang != "" && tool != "" {
			row, err := queryStackRowByLangTool(ctx, st, lang, tool)
			if err == nil {
				return row, nil
			}
			if err != nil && err != pgx.ErrNoRows {
				return gateProfileStackRow{}, err
			}
		}
	}

	if image := strings.TrimSpace(job.JobImage); image != "" {
		row, err := queryStackRowByImage(ctx, st, image)
		if err == nil {
			return row, nil
		}
		if err != nil && err != pgx.ErrNoRows {
			return gateProfileStackRow{}, err
		}
	}

	return gateProfileStackRow{}, fmt.Errorf("unable to resolve stack for successful gate profile persistence")
}

func queryStackRowByExpectation(ctx context.Context, st store.Store, lang, tool, release string) (gateProfileStackRow, error) {
	got, err := st.ResolveStackRowByLangToolRelease(ctx, store.ResolveStackRowByLangToolReleaseParams{
		Lang:    lang,
		Tool:    tool,
		Release: release,
	})
	if err != nil {
		return gateProfileStackRow{}, err
	}
	return gateProfileStackRow{
		ID:      got.ID,
		Lang:    got.Lang,
		Tool:    got.Tool,
		Release: got.Release,
	}, nil
}

func queryStackRowByLangTool(ctx context.Context, st store.Store, lang, tool string) (gateProfileStackRow, error) {
	got, err := st.ResolveStackRowByLangTool(ctx, store.ResolveStackRowByLangToolParams{
		Lang: lang,
		Tool: tool,
	})
	if err != nil {
		return gateProfileStackRow{}, err
	}
	return gateProfileStackRow{
		ID:      got.ID,
		Lang:    got.Lang,
		Tool:    got.Tool,
		Release: got.Release,
	}, nil
}

func queryStackRowByImage(ctx context.Context, st store.Store, image string) (gateProfileStackRow, error) {
	got, err := st.ResolveStackRowByImage(ctx, image)
	if err != nil {
		return gateProfileStackRow{}, err
	}
	return gateProfileStackRow{
		ID:      got.ID,
		Lang:    got.Lang,
		Tool:    got.Tool,
		Release: got.Release,
	}, nil
}

func buildSuccessfulGateProfilePayload(
	repoID domaintypes.RepoID,
	target string,
	stackRow gateProfileStackRow,
	gateMeta *contracts.BuildGateStageMetadata,
) ([]byte, error) {
	if gateMeta == nil {
		return nil, fmt.Errorf("gate metadata is required")
	}
	language := strings.TrimSpace(stackRow.Lang)
	tool := strings.TrimSpace(stackRow.Tool)
	release := strings.TrimSpace(stackRow.Release)
	if exp := gateMeta.DetectedStackExpectation(); exp != nil {
		if trimmed := strings.TrimSpace(exp.Language); trimmed != "" {
			language = trimmed
		}
		if trimmed := strings.TrimSpace(exp.Tool); trimmed != "" {
			tool = trimmed
		}
		if trimmed := strings.TrimSpace(exp.Release); trimmed != "" {
			release = trimmed
		}
	}
	if language == "" || tool == "" {
		return nil, fmt.Errorf("detected stack must include language and tool")
	}

	command, err := executedGateTargetCommand(gateMeta)
	if err != nil {
		return nil, err
	}

	targetPassed := &contracts.GateProfileTarget{
		Status:  contracts.PrepTargetStatusPassed,
		Command: command,
		Env:     map[string]string{},
	}
	targetNotAttempted := func() *contracts.GateProfileTarget {
		return &contracts.GateProfileTarget{
			Status: contracts.PrepTargetStatusNotAttempted,
			Env:    map[string]string{},
		}
	}

	profile := contracts.GateProfile{
		SchemaVersion: 1,
		RepoID:        repoID.String(),
		RunnerMode:    contracts.PrepRunnerModeSimple,
		Stack: contracts.GateProfileStack{
			Language: language,
			Tool:     tool,
			Release:  release,
		},
		Targets: contracts.GateProfileTargets{
			Active:   target,
			Build:    targetNotAttempted(),
			Unit:     targetNotAttempted(),
			AllTests: targetNotAttempted(),
		},
		Orchestration: contracts.GateProfileOrchestration{
			Pre:  []json.RawMessage{},
			Post: []json.RawMessage{},
		},
	}
	switch target {
	case contracts.GateProfileTargetBuild:
		profile.Targets.Build = targetPassed
	case contracts.GateProfileTargetUnit:
		profile.Targets.Unit = targetPassed
	case contracts.GateProfileTargetAllTests:
		profile.Targets.AllTests = targetPassed
	default:
		return nil, fmt.Errorf("unsupported gate target %q", target)
	}

	raw, err := json.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("marshal successful gate profile payload: %w", err)
	}
	if _, err := contracts.ParseGateProfileJSON(raw); err != nil {
		return nil, fmt.Errorf("validate successful gate profile payload: %w", err)
	}
	return raw, nil
}

func executedGateTargetCommand(gateMeta *contracts.BuildGateStageMetadata) (string, error) {
	if gateMeta == nil {
		return "", fmt.Errorf("gate metadata is required")
	}
	command := strings.TrimSpace(gateMeta.ExecutedCommand)
	if command == "" {
		return "", fmt.Errorf("gate metadata executed_command is required for successful gate profile persistence")
	}
	return command, nil
}

func upsertSuccessfulGateProfileRow(
	ctx context.Context,
	st store.Store,
	repoID domaintypes.RepoID,
	repoSHA string,
	stackID int64,
	objectKey string,
) (int64, error) {
	got, err := st.UpsertExactGateProfile(ctx, store.UpsertExactGateProfileParams{
		RepoID:  repoID.String(),
		RepoSha: repoSHA,
		StackID: stackID,
		Url:     objectKey,
	})
	if err != nil {
		return 0, fmt.Errorf("upsert successful gate profile row: %w", err)
	}
	return got.ID, nil
}

func upsertSuccessfulGateJobProfileLink(ctx context.Context, st store.Store, jobID domaintypes.JobID, profileID int64) error {
	err := st.UpsertGateJobProfileLink(ctx, store.UpsertGateJobProfileLinkParams{
		JobID:     jobID.String(),
		ProfileID: profileID,
	})
	if err != nil {
		return fmt.Errorf("upsert gate job profile link: %w", err)
	}
	return nil
}

func persistGateProfilePayload(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	job store.Job,
	repoSHA string,
	stackID int64,
	payload []byte,
) error {
	objectKey := fmt.Sprintf(
		"gate-profiles/repos/%s/%s/stack-%d/profile-%d.json",
		job.RepoID.String(),
		repoSHA,
		stackID,
		time.Now().UTC().UnixNano(),
	)
	if _, err := bs.Put(ctx, objectKey, "application/json", payload); err != nil {
		return fmt.Errorf("put gate profile blob: %w", err)
	}

	profileID, err := upsertSuccessfulGateProfileRow(ctx, st, job.RepoID, repoSHA, stackID, objectKey)
	if err != nil {
		return err
	}
	return upsertSuccessfulGateJobProfileLink(ctx, st, job.ID, profileID)
}
