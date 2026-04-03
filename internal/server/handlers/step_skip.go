package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type migStepOpsPayload struct {
	Step         contracts.MigStep          `json:"step"`
	GlobalEnv    map[string]string          `json:"global_env,omitempty"`
	EffectiveEnv map[string]string          `json:"effective_env,omitempty"`
	Artifacts    []string                   `json:"artifact_paths,omitempty"`
	BuildGate    *contracts.BuildGateConfig `json:"build_gate,omitempty"`
}

func resolveAndPersistMigStepSkip(
	ctx context.Context,
	st store.Store,
	job store.Job,
	mergedSpec []byte,
) (*contracts.MigStepSkipMetadata, error) {
	if domaintypes.JobType(job.JobType) != domaintypes.JobTypeMig {
		return nil, nil
	}

	pgStore, ok := st.(*store.PgStore)
	if !ok || pgStore == nil {
		return nil, nil
	}

	spec, err := contracts.ParseMigSpecJSON(mergedSpec)
	if err != nil {
		return nil, fmt.Errorf("parse merged spec for step cache: %w", err)
	}

	stepIndex, err := migStepIndexFromJobNameForClaim(job.Name, len(spec.Steps))
	if err != nil {
		return nil, err
	}
	stepCfg := spec.Steps[stepIndex]

	effectiveEnv := make(map[string]string, len(spec.Envs)+len(stepCfg.Envs))
	for k, v := range spec.Envs {
		effectiveEnv[k] = v
	}
	for k, v := range stepCfg.Envs {
		effectiveEnv[k] = v
	}

	opsPayload := migStepOpsPayload{
		Step:         stepCfg,
		GlobalEnv:    spec.Envs,
		EffectiveEnv: effectiveEnv,
		Artifacts:    spec.ArtifactPaths,
		BuildGate:    spec.BuildGate,
	}

	opsJSON, hashHex, err := canonicalizeAndHashJSON(opsPayload)
	if err != nil {
		return nil, fmt.Errorf("canonicalize step ops: %w", err)
	}

	var refJobID *string
	var skip *contracts.MigStepSkipMetadata
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if !stepCfg.Always && sha40Pattern.MatchString(repoSHAIn) {
		row, err := pgStore.ResolveReusableStepByHash(ctx, store.ResolveReusableStepByHashParams{
			RepoID:    job.RepoID,
			RepoShaIn: repoSHAIn,
			Hash:      hashHex,
		})
		if err == nil {
			ref := row.RefJobID.String()
			refJobID = &ref
			if sha40Pattern.MatchString(strings.TrimSpace(row.RefRepoShaOut)) {
				skip = &contracts.MigStepSkipMetadata{
					Enabled:       true,
					RefJobID:      row.RefJobID,
					RefRepoSHAOut: strings.TrimSpace(row.RefRepoShaOut),
					Hash:          hashHex,
				}
			}
		} else if !isNoRowsError(err) {
			return nil, fmt.Errorf("resolve reusable step: %w", err)
		}
	}

	if err := pgStore.UpsertStep(ctx, store.UpsertStepParams{
		JobID:    job.ID.String(),
		Ops:      opsJSON,
		Hash:     hashHex,
		RefJobID: refJobID,
	}); err != nil {
		return nil, fmt.Errorf("upsert steps cache row: %w", err)
	}

	return skip, nil
}

func migStepIndexFromJobNameForClaim(jobName string, stepsLen int) (int, error) {
	name := strings.TrimSpace(jobName)
	if stepsLen <= 1 {
		return 0, nil
	}
	if !strings.HasPrefix(name, "mig-") {
		return 0, fmt.Errorf("mig job_name must start with mig- for multi-step runs, got %q", name)
	}
	raw := strings.TrimPrefix(name, "mig-")
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse mig index from job_name %q: %w", name, err)
	}
	if idx < 0 || idx >= stepsLen {
		return 0, fmt.Errorf("mig index out of range for job_name %q: idx=%d steps_len=%d", name, idx, stepsLen)
	}
	return idx, nil
}

func canonicalizeAndHashJSON(v any) ([]byte, string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, "", err
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	if err := writeCanonicalJSON(&buf, decoded); err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:]), nil
}

func writeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyJSON, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(keyJSON)
			buf.WriteByte(':')
			if err := writeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
		return nil
	case []any:
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonicalJSON(buf, item); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return nil
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
		return nil
	}
}
