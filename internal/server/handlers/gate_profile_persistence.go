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
	"github.com/jackc/pgx/v5/pgxpool"
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
		jobType != domaintypes.JobTypePostGate &&
		jobType != domaintypes.JobTypeReGate {
		return nil
	}

	meta, err := contracts.UnmarshalJobMeta(rawMeta)
	if err != nil || meta == nil || meta.Gate == nil {
		return nil
	}
	if len(meta.Gate.StaticChecks) == 0 || !meta.Gate.StaticChecks[0].Passed {
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

	stackRow, err := resolveGateProfileStackRow(ctx, st, job, meta.Gate)
	if err != nil {
		return err
	}

	payload, err := buildSuccessfulGateProfilePayload(job.RepoID, target, stackRow, meta.Gate)
	if err != nil {
		return err
	}

	objectKey := fmt.Sprintf(
		"gate-profiles/repos/%s/%s/stack-%d/profile-%d.json",
		job.RepoID.String(),
		repoSHA,
		stackRow.ID,
		time.Now().UTC().UnixNano(),
	)
	if _, err := bs.Put(ctx, objectKey, "application/json", payload); err != nil {
		return fmt.Errorf("put gate profile blob: %w", err)
	}

	profileID, err := upsertSuccessfulGateProfileRow(ctx, st, job.RepoID, repoSHA, stackRow.ID, objectKey)
	if err != nil {
		return err
	}
	if err := upsertSuccessfulGateJobProfileLink(ctx, st, job.ID, profileID); err != nil {
		return err
	}
	return nil
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
	pool := st.Pool()
	exp := gateMeta.DetectedStackExpectation()

	if exp != nil {
		lang := strings.TrimSpace(exp.Language)
		tool := strings.TrimSpace(exp.Tool)
		release := strings.TrimSpace(exp.Release)
		if lang != "" && tool != "" && release != "" {
			row, err := queryStackRowByExpectation(ctx, pool, lang, tool, release)
			if err == nil {
				return row, nil
			}
			if err != nil && err != pgx.ErrNoRows {
				return gateProfileStackRow{}, err
			}
		}
		if lang != "" && tool != "" {
			row, err := queryStackRowByLangTool(ctx, pool, lang, tool)
			if err == nil {
				return row, nil
			}
			if err != nil && err != pgx.ErrNoRows {
				return gateProfileStackRow{}, err
			}
		}
	}

	if image := strings.TrimSpace(job.JobImage); image != "" {
		row, err := queryStackRowByImage(ctx, pool, image)
		if err == nil {
			return row, nil
		}
		if err != nil && err != pgx.ErrNoRows {
			return gateProfileStackRow{}, err
		}
	}

	return gateProfileStackRow{}, fmt.Errorf("unable to resolve stack for successful gate profile persistence")
}

func queryStackRowByExpectation(ctx context.Context, pool *pgxpool.Pool, lang, tool, release string) (gateProfileStackRow, error) {
	var row gateProfileStackRow
	err := pool.QueryRow(ctx, `
SELECT id, lang, COALESCE(tool, ''), release
FROM ploy.stacks
WHERE lang = $1
  AND COALESCE(tool, '') = $2
  AND release = $3
ORDER BY id ASC
LIMIT 1
`, lang, tool, release).Scan(
		&row.ID,
		&row.Lang,
		&row.Tool,
		&row.Release,
	)
	return row, err
}

func queryStackRowByLangTool(ctx context.Context, pool *pgxpool.Pool, lang, tool string) (gateProfileStackRow, error) {
	var row gateProfileStackRow
	err := pool.QueryRow(ctx, `
SELECT id, lang, COALESCE(tool, ''), release
FROM ploy.stacks
WHERE lang = $1
  AND COALESCE(tool, '') = $2
ORDER BY id ASC
LIMIT 1
`, lang, tool).Scan(
		&row.ID,
		&row.Lang,
		&row.Tool,
		&row.Release,
	)
	return row, err
}

func queryStackRowByImage(ctx context.Context, pool *pgxpool.Pool, image string) (gateProfileStackRow, error) {
	var row gateProfileStackRow
	err := pool.QueryRow(ctx, `
SELECT id, lang, COALESCE(tool, ''), release
FROM ploy.stacks
WHERE image = $1
ORDER BY id ASC
LIMIT 1
`, image).Scan(
		&row.ID,
		&row.Lang,
		&row.Tool,
		&row.Release,
	)
	return row, err
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

	command, err := defaultGateTargetCommand(tool, target)
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

func defaultGateTargetCommand(tool, target string) (string, error) {
	tool = strings.ToLower(strings.TrimSpace(tool))
	target = strings.TrimSpace(target)

	switch tool {
	case "maven":
		switch target {
		case contracts.GateProfileTargetBuild:
			return "mvn --ff -B -q -e -DskipTests=true -Dstyle.color=never -f /workspace/pom.xml clean install", nil
		case contracts.GateProfileTargetUnit, contracts.GateProfileTargetAllTests:
			return "mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install", nil
		}
	case "gradle":
		switch target {
		case contracts.GateProfileTargetBuild:
			return "gradle -q --stacktrace --build-cache build -x test -p /workspace", nil
		case contracts.GateProfileTargetUnit, contracts.GateProfileTargetAllTests:
			return "gradle -q --stacktrace --build-cache test -p /workspace", nil
		}
	case "go":
		return "go test ./...", nil
	case "cargo", "rust":
		return "cargo test", nil
	}

	return "", fmt.Errorf("unsupported tool/target for successful gate profile payload: %s/%s", tool, target)
}

func upsertSuccessfulGateProfileRow(
	ctx context.Context,
	st store.Store,
	repoID domaintypes.RepoID,
	repoSHA string,
	stackID int64,
	objectKey string,
) (int64, error) {
	var profileID int64
	err := st.Pool().QueryRow(ctx, `
INSERT INTO ploy.gate_profiles (
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
  updated_at = now()
RETURNING id
`, repoID, repoSHA, stackID, objectKey).Scan(&profileID)
	if err != nil {
		return 0, fmt.Errorf("upsert successful gate profile row: %w", err)
	}
	return profileID, nil
}

func upsertSuccessfulGateJobProfileLink(ctx context.Context, st store.Store, jobID domaintypes.JobID, profileID int64) error {
	_, err := st.Pool().Exec(ctx, `
INSERT INTO ploy.gates (
  job_id,
  profile_id
)
VALUES ($1, $2)
ON CONFLICT (job_id)
DO UPDATE SET
  profile_id = EXCLUDED.profile_id
`, jobID, profileID)
	if err != nil {
		return fmt.Errorf("upsert gate job profile link: %w", err)
	}
	return nil
}
