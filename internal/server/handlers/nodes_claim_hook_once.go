package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
)

func resolveHookRuntimeDecision(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	job store.Job,
	mergedSpec json.RawMessage,
	jobType domaintypes.JobType,
) (*contracts.HookRuntimeDecision, error) {
	if jobType != domaintypes.JobTypeHook {
		return nil, nil
	}
	migSpec, err := contracts.ParseMigSpecJSON(mergedSpec)
	if err != nil {
		return nil, fmt.Errorf("parse merged spec for hook runtime: %w", err)
	}
	source := hookSourceFromJobMeta(job.Meta)
	hookIndex := -1
	if source == "" {
		var err error
		hookIndex, err = hookIndexFromJobName(job.Name, len(migSpec.Hooks))
		if err != nil {
			return nil, err
		}
	}
	if source == "" {
		source = strings.TrimSpace(migSpec.Hooks[hookIndex])
	}
	if source == "" {
		return nil, fmt.Errorf("hook source is empty for index %d", hookIndex)
	}
	hookSpec, err := loadRuntimeHookSpec(ctx, st, bs, source, migSpec.BundleMap)
	if err != nil {
		return nil, &ClaimJobTerminalError{
			Message: fmt.Sprintf("load hook spec for source %q", source),
			Err:     err,
		}
	}
	hash, err := hook.DeterministicHookHash(hookSpec)
	if err != nil {
		return nil, fmt.Errorf("compute deterministic hook hash for source %q: %w", source, err)
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, fmt.Errorf("hook hash is empty for source %q", source)
	}

	runtimeDecision := &contracts.HookRuntimeDecision{
		HookHash:      hash,
		HookShouldRun: true,
	}
	matchInput, matchErr := buildHookMatchInput(ctx, st, job)
	if matchErr != nil {
		return nil, fmt.Errorf("build hook runtime match input: %w", matchErr)
	}
	populateHookRuntimeMatchedTransition(runtimeDecision, hookSpec, matchInput)
	return runtimeDecision, nil
}

func buildHookMatchInput(ctx context.Context, st store.Store, job store.Job) (hook.MatchInput, error) {
	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   job.RunID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return hook.MatchInput{}, fmt.Errorf("list jobs for hook runtime input: %w", err)
	}

	current, previous, err := resolveHookSBOMSnapshots(ctx, st, jobs)
	if err != nil {
		return hook.MatchInput{}, err
	}

	return hook.MatchInput{
		Stack:        resolveHookRuntimeStack(jobs),
		CurrentSBOM:  current,
		PreviousSBOM: previous,
	}, nil
}

func resolveHookRuntimeStack(jobs []store.Job) hook.RuntimeStack {
	var (
		selected store.Job
		found    bool
		stack    hook.RuntimeStack
	)
	for _, candidate := range jobs {
		if !isGateJobTypeForClaim(domaintypes.JobType(candidate.JobType)) || candidate.Status != domaintypes.JobStatusSuccess || len(candidate.Meta) == 0 {
			continue
		}
		meta, err := contracts.UnmarshalJobMeta(candidate.Meta)
		if err != nil || meta.GateMetadata == nil {
			continue
		}
		exp := meta.GateMetadata.DetectedStackExpectation()
		if exp == nil {
			continue
		}
		if !found || sbomJobIsMoreRecent(candidate, selected) {
			selected = candidate
			stack = hook.RuntimeStack{
				Language: strings.TrimSpace(exp.Language),
				Tool:     strings.TrimSpace(exp.Tool),
				Release:  strings.TrimSpace(exp.Release),
			}
			found = true
		}
	}
	return stack
}

func resolveHookSBOMSnapshots(
	ctx context.Context,
	st store.Store,
	jobs []store.Job,
) ([]hook.SBOMPackage, []hook.SBOMPackage, error) {
	latest, previous := latestTwoSuccessfulSBOMJobs(jobs)
	if latest == nil {
		return nil, nil, nil
	}

	currentRows, err := listSBOMRowsByEffectiveJob(ctx, st, *latest)
	if err != nil {
		return nil, nil, fmt.Errorf("list current sbom rows for job %s: %w", latest.ID, err)
	}
	current := toHookSBOMPackages(currentRows)

	if previous == nil {
		return current, nil, nil
	}
	previousRows, err := listSBOMRowsByEffectiveJob(ctx, st, *previous)
	if err != nil {
		return nil, nil, fmt.Errorf("list previous sbom rows for job %s: %w", previous.ID, err)
	}
	return current, toHookSBOMPackages(previousRows), nil
}

func latestTwoSuccessfulSBOMJobs(jobs []store.Job) (*store.Job, *store.Job) {
	var latest *store.Job
	var previous *store.Job
	for i := range jobs {
		if jobs[i].JobType != domaintypes.JobTypeSBOM || jobs[i].Status != domaintypes.JobStatusSuccess {
			continue
		}
		candidate := &jobs[i]
		if latest == nil || sbomJobIsMoreRecent(*candidate, *latest) {
			previous = latest
			latest = candidate
			continue
		}
		if previous == nil || sbomJobIsMoreRecent(*candidate, *previous) {
			previous = candidate
		}
	}
	return latest, previous
}

func toHookSBOMPackages(rows []store.Sbom) []hook.SBOMPackage {
	if len(rows) == 0 {
		return nil
	}
	out := make([]hook.SBOMPackage, 0, len(rows))
	for _, row := range rows {
		out = append(out, hook.SBOMPackage{
			Name:    strings.TrimSpace(row.Lib),
			Version: strings.TrimSpace(row.Ver),
		})
	}
	return out
}

func populateHookRuntimeMatchedTransition(
	decision *contracts.HookRuntimeDecision,
	spec hook.Spec,
	input hook.MatchInput,
) {
	if decision == nil {
		return
	}
	prevByName := make(map[string][]string)
	for _, pkg := range input.PreviousSBOM {
		name := strings.TrimSpace(pkg.Name)
		if name == "" {
			continue
		}
		prevByName[name] = append(prevByName[name], strings.TrimSpace(pkg.Version))
	}
	curByName := make(map[string][]string)
	for _, pkg := range input.CurrentSBOM {
		name := strings.TrimSpace(pkg.Name)
		if name == "" {
			continue
		}
		curByName[name] = append(curByName[name], strings.TrimSpace(pkg.Version))
	}

	lang := strings.TrimSpace(input.Stack.Language)
	tool := strings.TrimSpace(input.Stack.Tool)
	for _, cond := range spec.SBOM.OnChange {
		name := strings.TrimSpace(cond.Name)
		if name == "" {
			continue
		}
		prevVersions := prevByName[name]
		curVersions := curByName[name]
		if len(prevVersions) == 0 || len(curVersions) == 0 {
			continue
		}
		for _, pv := range prevVersions {
			if !hook.MatchVersionConstraint(lang, tool, pv, strings.TrimSpace(cond.From)) {
				continue
			}
			for _, cv := range curVersions {
				if !hook.MatchVersionConstraint(lang, tool, cv, strings.TrimSpace(cond.To)) {
					continue
				}
				if hook.CompareVersions(lang, tool, pv, cv) == 0 {
					continue
				}
				decision.MatchedPredicate = "on_change"
				decision.MatchedPackage = name
				decision.PreviousVersion = pv
				decision.CurrentVersion = cv
				return
			}
		}
	}
}

func applyHookOnceLedgerDecision(
	ctx context.Context,
	st store.Store,
	job store.Job,
	decision *contracts.HookRuntimeDecision,
) (*contracts.HookRuntimeDecision, error) {
	exists, err := st.HasHookOnceLedger(ctx, store.HasHookOnceLedgerParams{
		RunID:    job.RunID,
		RepoID:   job.RepoID,
		HookHash: decision.HookHash,
	})
	if err != nil {
		return nil, fmt.Errorf("check hook once ledger: %w", err)
	}
	if !exists {
		return decision, nil
	}

	ledger, err := st.GetHookOnceLedger(ctx, store.GetHookOnceLedgerParams{
		RunID:    job.RunID,
		RepoID:   job.RepoID,
		HookHash: decision.HookHash,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return decision, nil
		}
		return nil, fmt.Errorf("get hook once ledger: %w", err)
	}

	// Skip only after a successful execution for this run/repo/hash exists.
	if ledger.FirstSuccessJobID == nil {
		return decision, nil
	}
	decision.HookShouldRun = false
	decision.HookOnceSkipMarked = !ledger.OnceSkipMarked
	return decision, nil
}

func hookIndexFromJobName(jobName string, hooksLen int) (int, error) {
	name := strings.TrimSpace(jobName)
	if hooksLen <= 0 {
		return 0, fmt.Errorf("hook job requires at least one declared hook source")
	}
	idx := strings.LastIndex(name, "-hook-")
	if idx <= 0 {
		return 0, fmt.Errorf("hook job_name must contain %q, got %q", "-hook-", name)
	}
	raw := strings.TrimSpace(name[idx+len("-hook-"):])
	hookIndex, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse hook index from job_name %q: %w", name, err)
	}
	if hookIndex < 0 || hookIndex >= hooksLen {
		return 0, fmt.Errorf("hook index out of range for job_name %q: idx=%d hooks_len=%d", name, hookIndex, hooksLen)
	}
	return hookIndex, nil
}

func loadRuntimeHookSpec(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	source string,
	bundleMap map[string]string,
) (hook.Spec, error) {
	source = strings.TrimSpace(source)
	if canonicalHookSourcePattern.MatchString(source) {
		return loadHookSpecFromBundleHash(ctx, st, bs, source, bundleMap)
	}
	if isHTTPSHookSource(source) {
		return loadRuntimeHookSpecFromLoader(source, ".")
	}
	return hook.Spec{}, fmt.Errorf("unsupported hook source %q: local hook sources must be precompiled into hash entries", source)
}

func loadRuntimeHookSpecFromLoader(source string, specRoot string) (hook.Spec, error) {
	specs, err := hook.NewLoader(nil).LoadFromMigSpec(contracts.MigSpec{
		Hooks: []string{source},
	}, specRoot)
	if err != nil {
		return hook.Spec{}, err
	}
	if len(specs) == 0 {
		return hook.Spec{}, fmt.Errorf("no resolved hook spec for source %q", source)
	}
	return specs[0], nil
}

func loadHookSpecFromBundleHash(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	hash string,
	bundleMap map[string]string,
) (hook.Spec, error) {
	if bs == nil {
		return hook.Spec{}, fmt.Errorf("blob store is required to load hook bundle %q", hash)
	}
	bundleID := strings.TrimSpace(bundleMap[hash])
	if bundleID == "" {
		return hook.Spec{}, fmt.Errorf("bundle_map[%q] is missing", hash)
	}

	bundle, err := probeSpecBundleIntegrity(ctx, st, bs, bundleID)
	if err != nil {
		return hook.Spec{}, fmt.Errorf("verify spec bundle for hash %q: %w", hash, err)
	}

	key := strings.TrimSpace(*bundle.ObjectKey)
	reader, _, err := bs.Get(ctx, key)
	if err != nil {
		return hook.Spec{}, fmt.Errorf("download spec bundle blob %q: %w", key, err)
	}
	defer func() { _ = reader.Close() }()

	data, err := io.ReadAll(reader)
	if err != nil {
		return hook.Spec{}, fmt.Errorf("read spec bundle %q: %w", bundleID, err)
	}
	manifestData, manifestSource, err := extractHookManifestFromBundle(data)
	if err != nil {
		return hook.Spec{}, fmt.Errorf("extract hook manifest from bundle %q: %w", bundleID, err)
	}

	specDoc, err := hook.LoadSpecYAML(manifestData, manifestSource)
	if err != nil {
		return hook.Spec{}, err
	}
	return specDoc, nil
}

func extractHookManifestFromBundle(data []byte) ([]byte, string, error) {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	var directManifest []byte
	type manifestEntry struct {
		path string
		data []byte
	}
	var hookYAMLs []manifestEntry

	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, "", fmt.Errorf("read tar header: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}

		name := strings.TrimSpace(header.Name)
		payload, readErr := io.ReadAll(tarReader)
		if readErr != nil {
			return nil, "", fmt.Errorf("read tar entry %q: %w", name, readErr)
		}
		if name == "content" {
			directManifest = payload
			continue
		}
		if strings.HasPrefix(name, "content/") && strings.HasSuffix(name, "/hook.yaml") {
			hookYAMLs = append(hookYAMLs, manifestEntry{path: name, data: payload})
		}
	}

	if len(directManifest) > 0 && len(hookYAMLs) > 0 {
		return nil, "", fmt.Errorf("bundle contains both direct content manifest and directory hook.yaml files")
	}
	if len(directManifest) > 0 {
		return directManifest, "bundle:content", nil
	}
	if len(hookYAMLs) == 0 {
		return nil, "", fmt.Errorf("no hook manifest found in bundle")
	}
	if len(hookYAMLs) != 1 {
		return nil, "", fmt.Errorf("expected exactly 1 hook.yaml in bundle, found %d", len(hookYAMLs))
	}
	return hookYAMLs[0].data, "bundle:" + hookYAMLs[0].path, nil
}

func hookSourceFromJobMeta(metaRaw []byte) string {
	if len(metaRaw) == 0 {
		return ""
	}
	meta, err := contracts.UnmarshalJobMeta(metaRaw)
	if err != nil || meta == nil {
		return ""
	}
	return strings.TrimSpace(meta.HookSource)
}
