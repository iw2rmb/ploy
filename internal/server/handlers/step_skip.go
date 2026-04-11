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

	cacheKey, err := computeJobCacheKey(
		domaintypes.JobTypeMig,
		job.Name,
		job.JobImage,
		job.RepoShaIn,
		"",
		mergedSpec,
	)
	if err != nil {
		return nil, fmt.Errorf("compute step cache key: %w", err)
	}
	cacheKey = strings.TrimSpace(cacheKey)
	if cacheKey == "" {
		return nil, nil
	}
	if err := pgStore.UpdateJobCacheKey(ctx, store.UpdateJobCacheKeyParams{
		ID:       job.ID,
		CacheKey: cacheKey,
	}); err != nil {
		return nil, fmt.Errorf("persist step cache key: %w", err)
	}

	var skip *contracts.MigStepSkipMetadata
	repoSHAIn := strings.TrimSpace(job.RepoShaIn)
	if !stepCfg.Always && sha40Pattern.MatchString(repoSHAIn) {
		row, err := pgStore.ResolveReusableStepByCacheKey(ctx, store.ResolveReusableStepByCacheKeyParams{
			RepoID:   job.RepoID,
			CacheKey: cacheKey,
		})
		if err == nil {
			if sha40Pattern.MatchString(strings.TrimSpace(row.RefRepoShaOut)) {
				if err := persistCacheMirrorSourceJob(ctx, pgStore, job.ID, row.RefJobID); err != nil {
					return nil, fmt.Errorf("persist cache mirror source job: %w", err)
				}
				skip = &contracts.MigStepSkipMetadata{
					Enabled:       true,
					RefRepoSHAOut: strings.TrimSpace(row.RefRepoShaOut),
					Hash:          cacheKey,
				}
			}
		} else if !isNoRowsError(err) {
			return nil, fmt.Errorf("resolve reusable step: %w", err)
		}
	}

	return skip, nil
}

func persistCacheMirrorSourceJob(
	ctx context.Context,
	pgStore *store.PgStore,
	targetJobID domaintypes.JobID,
	sourceJobID domaintypes.JobID,
) error {
	if pgStore == nil || targetJobID.IsZero() || sourceJobID.IsZero() {
		return nil
	}
	if targetJobID == sourceJobID {
		return fmt.Errorf("source_job_id must not equal target job id %s", targetJobID)
	}

	jobRow, err := pgStore.GetJob(ctx, targetJobID)
	if err != nil {
		return fmt.Errorf("load target job: %w", err)
	}

	var meta *contracts.JobMeta
	if len(jobRow.Meta) == 0 {
		meta = contracts.NewMigJobMeta()
	} else {
		meta, err = contracts.UnmarshalJobMeta(jobRow.Meta)
		if err != nil {
			return fmt.Errorf("parse target job meta: %w", err)
		}
	}

	if meta.CacheMirror != nil && meta.CacheMirror.SourceJobID == sourceJobID {
		return nil
	}

	meta.CacheMirror = &contracts.CacheMirrorMetadata{SourceJobID: sourceJobID}
	metaBytes, err := contracts.MarshalJobMeta(meta)
	if err != nil {
		return fmt.Errorf("marshal target job meta: %w", err)
	}

	if err := pgStore.UpdateJobMeta(ctx, store.UpdateJobMetaParams{
		ID:   targetJobID,
		Meta: metaBytes,
	}); err != nil {
		return fmt.Errorf("update target job meta: %w", err)
	}
	return nil
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
