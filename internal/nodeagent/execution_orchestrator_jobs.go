// execution_orchestrator_jobs.go contains mig and healing job implementations,
// the shared standard job executor, and workspace lifecycle helpers.
package nodeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/hook"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

const preGateCanonicalSBOMFileName = "sbom.spdx.json"

const (
	preGateCycleName         = "pre-gate"
	postGateCycleName        = "post-gate"
	hookJobNameDelimiter     = "-hook-"
	preGateHookJobNamePrefix = "pre-gate-hook-"
)

func gateCycleRootDir(runID types.RunID, cycleName string) string {
	return filepath.Join(runCacheDir(runID), "gate-cycles", strings.TrimSpace(cycleName))
}

func gateCycleSBOMOutPath(runID types.RunID, cycleName string) string {
	return filepath.Join(gateCycleRootDir(runID, cycleName), "sbom", "out", preGateCanonicalSBOMFileName)
}

func gateCycleHookDir(runID types.RunID, cycleName string, hookIndex int) string {
	return filepath.Join(gateCycleRootDir(runID, cycleName), "hooks", fmt.Sprintf("%03d", hookIndex))
}

func gateCycleHookInPath(runID types.RunID, cycleName string, hookIndex int) string {
	return filepath.Join(gateCycleHookDir(runID, cycleName, hookIndex), "in", preGateCanonicalSBOMFileName)
}

func gateCycleHookOutPath(runID types.RunID, cycleName string, hookIndex int) string {
	return filepath.Join(gateCycleHookDir(runID, cycleName, hookIndex), "out", preGateCanonicalSBOMFileName)
}

func gateCycleHookInputSnapshotPath(runID types.RunID, cycleName string, hookIndex int) string {
	if hookIndex <= 0 {
		return gateCycleSBOMOutPath(runID, cycleName)
	}
	for idx := hookIndex - 1; idx >= 0; idx-- {
		candidate := gateCycleHookOutPath(runID, cycleName, idx)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return gateCycleSBOMOutPath(runID, cycleName)
}

func gateCycleFinalSnapshotPath(runID types.RunID, cycleName string, hooks []string) string {
	_ = hooks
	hooksRoot := filepath.Join(gateCycleRootDir(runID, cycleName), "hooks")
	entries, err := os.ReadDir(hooksRoot)
	if err != nil {
		return gateCycleSBOMOutPath(runID, cycleName)
	}
	maxIdx := -1
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		idx, convErr := strconv.Atoi(strings.TrimSpace(entry.Name()))
		if convErr != nil || idx < 0 {
			continue
		}
		candidate := gateCycleHookOutPath(runID, cycleName, idx)
		if _, statErr := os.Stat(candidate); statErr != nil {
			continue
		}
		if idx > maxIdx {
			maxIdx = idx
		}
	}
	if maxIdx >= 0 {
		return gateCycleHookOutPath(runID, cycleName, maxIdx)
	}
	return gateCycleSBOMOutPath(runID, cycleName)
}

func gateCycleNameFromSBOMContext(sbomCtx *contracts.SBOMJobMetadata) (string, error) {
	if sbomCtx == nil {
		return "", fmt.Errorf("sbom context is required")
	}
	if cycleName := strings.TrimSpace(sbomCtx.CycleName); cycleName != "" {
		return cycleName, nil
	}
	phase := strings.TrimSpace(sbomCtx.Phase)
	switch phase {
	case contracts.SBOMPhasePre:
		return preGateCycleName, nil
	case contracts.SBOMPhasePost:
		return postGateCycleName, nil
	default:
		return "", fmt.Errorf("sbom context phase invalid: %q", phase)
	}
}

func gateCycleHookIndexFromJobName(jobName string, hooksLen int) (string, int, error) {
	name := strings.TrimSpace(jobName)
	delimIdx := strings.LastIndex(name, hookJobNameDelimiter)
	if delimIdx <= 0 {
		return "", 0, fmt.Errorf("hook job_name must contain %q, got %q", hookJobNameDelimiter, name)
	}
	cycleName := strings.TrimSpace(name[:delimIdx])
	if cycleName == "" {
		return "", 0, fmt.Errorf("hook cycle name is empty in job_name %q", name)
	}
	raw := strings.TrimSpace(name[delimIdx+len(hookJobNameDelimiter):])
	idx, err := strconv.Atoi(raw)
	if err != nil {
		return "", 0, fmt.Errorf("parse hook index from job_name %q: %w", name, err)
	}
	if idx < 0 || idx >= hooksLen {
		return "", 0, fmt.Errorf("hook index out of range for job_name %q: idx=%d hooks_len=%d", name, idx, hooksLen)
	}
	return cycleName, idx, nil
}

func gateCycleNameFromGateJob(jobType types.JobType, jobName string) (string, error) {
	switch jobType {
	case types.JobTypePreGate:
		return preGateCycleName, nil
	case types.JobTypePostGate:
		return postGateCycleName, nil
	case types.JobTypeReGate:
		name := strings.TrimSpace(jobName)
		if name == "" {
			return "", fmt.Errorf("re-gate job_name is empty")
		}
		return name, nil
	default:
		return "", fmt.Errorf("unsupported gate job_type %q", jobType)
	}
}

func preGateSBOMOutPath(runID types.RunID) string {
	return gateCycleSBOMOutPath(runID, preGateCycleName)
}

func preGateHookDir(runID types.RunID, hookIndex int) string {
	return gateCycleHookDir(runID, preGateCycleName, hookIndex)
}

func preGateHookInPath(runID types.RunID, hookIndex int) string {
	return gateCycleHookInPath(runID, preGateCycleName, hookIndex)
}

func preGateHookOutPath(runID types.RunID, hookIndex int) string {
	return gateCycleHookOutPath(runID, preGateCycleName, hookIndex)
}

func preGateHookInputSnapshotPath(runID types.RunID, hookIndex int) string {
	return gateCycleHookInputSnapshotPath(runID, preGateCycleName, hookIndex)
}

func preGateFinalSnapshotPath(runID types.RunID, hooks []string) string {
	return gateCycleFinalSnapshotPath(runID, preGateCycleName, hooks)
}

func preGateHookIndexFromJobName(jobName string, hooksLen int) (int, error) {
	cycleName, idx, err := gateCycleHookIndexFromJobName(jobName, hooksLen)
	if err != nil {
		return 0, err
	}
	if cycleName != preGateCycleName {
		return 0, fmt.Errorf("hook job_name must start with %q, got %q", preGateHookJobNamePrefix, strings.TrimSpace(jobName))
	}
	return idx, nil
}

// executeSBOMJob runs SBOM collection in a container and materializes a
// validated canonical SPDX snapshot for the current gate cycle.
func (r *runController) executeSBOMJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	cycleName, err := gateCycleNameFromSBOMContext(req.SBOMContext)
	if err != nil {
		slog.Error("failed to derive sbom cycle", "run_id", req.RunID, "job_id", req.JobID, "job_name", req.JobName, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	initialStack := resolveSBOMStackForCycle(
		cycleName,
		resolveManifestStack(req, r.loadPersistedStack(req.RunID)),
		req.TypedOptions,
	)
	manifest, err := buildSBOMManifest(req, cycleName, initialStack)
	if err != nil {
		slog.Error("failed to build sbom manifest", "run_id", req.RunID, "job_id", req.JobID, "cycle_name", cycleName, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	stackForManifest := initialStack
	sbomSnapshotPath := gateCycleSBOMOutPath(req.RunID, cycleName)
	cfg := standardJobConfig{
		Manifest:      manifest,
		DiffType:      types.DiffJobTypeMig,
		OutDirPattern: "ploy-sbom-out-*",
		PrepareManifest: func(m *contracts.StepManifest, workspace string) error {
			detectedStack, detectErr := detectSBOMStackFromWorkspace(workspace)
			if detectErr != nil {
				return fmt.Errorf("resolve sbom stack from workspace: %w", detectErr)
			}
			if detectedStack == stackForManifest {
				return nil
			}
			stackForManifest = detectedStack
			return applySBOMRuntimeForStack(m, stackForManifest, sbomRuntimeReleaseForRequest(req, stackForManifest))
		},
		ValidateOutputs: func(outDir, _ string) error {
			return r.finalizeSBOMFlowOutputs(req.RunID, cycleName, outDir, sbomSnapshotPath)
		},
		WorkspacePolicy: workspaceChangePolicyIgnore,
		StartTime:       startTime,
	}
	r.executeStandardJob(ctx, req, cfg)
}

// executeHookJob executes the resolved hook step through the shared standard
// container runtime and stages canonical SBOM snapshots between hook jobs.
func (r *runController) executeHookJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()
	if len(req.TypedOptions.Hooks) == 0 {
		err := fmt.Errorf("hook job requires at least one declared hook source")
		slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	var (
		cycleName  string
		hookIndex  int
		hookSource string
		err        error
	)
	if req.HookContext != nil {
		req.HookContext.Normalize()
		cycleName = req.HookContext.CycleName
		hookIndex = req.HookContext.Index
		hookSource = req.HookContext.Source
	}
	if cycleName == "" || hookSource == "" || hookIndex < 0 {
		cycleName, hookIndex, err = gateCycleHookIndexFromJobName(req.JobName, len(req.TypedOptions.Hooks))
		if err != nil {
			slog.Error("failed to derive hook index", "run_id", req.RunID, "job_id", req.JobID, "error", err)
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
		hookSource = strings.TrimSpace(req.TypedOptions.Hooks[hookIndex])
	}
	if hookSource == "" {
		err = fmt.Errorf("hook source is empty for index %d", hookIndex)
		slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "hook_index", hookIndex, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	inputSnapshotPath := gateCycleHookInputSnapshotPath(req.RunID, cycleName, hookIndex)
	outPath := gateCycleHookOutPath(req.RunID, cycleName, hookIndex)
	conditionJSON := encodeHookConditionResult(req.HookRuntime)
	stack := resolveManifestStack(req, r.loadPersistedStack(req.RunID))
	if restoreErr := r.ensureHookSBOMInputSnapshot(ctx, req, cycleName, inputSnapshotPath); restoreErr != nil {
		err = fmt.Errorf("hook[%d] prepare sbom input snapshot: %w", hookIndex, restoreErr)
		slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "hook_index", hookIndex, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	if req.HookRuntime != nil && !req.HookRuntime.HookShouldRun {
		err = fmt.Errorf("hook[%d] runtime decision rejected execution: HookShouldRun=false (cycle=%s source=%q)", hookIndex, cycleName, hookSource)
		slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "hook_index", hookIndex, "error", err, "hook_condition_result", conditionJSON)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	specDoc, err := r.loadHookSpecForExecution(ctx, req, hookSource)
	if err != nil {
		err = fmt.Errorf("hook[%d] load hook spec %q: %w", hookIndex, hookSource, err)
		slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "hook_index", hookIndex, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}
	commandIdentityJSON := encodeHookCommandIdentityList(hookSource, specDoc.Steps, stack)
	phaseConfig := sbomPhaseConfigForCycle(cycleName, req.TypedOptions)
	phaseCA := []string(nil)
	if phaseConfig != nil {
		phaseCA = append(phaseCA, phaseConfig.CA...)
	}
	stepInputPath := inputSnapshotPath
	finalOutcome := standardJobOutcome{
		runErr: fmt.Errorf("hook[%d] has no executable steps", hookIndex),
	}
	var completedStepName string
	for stepIdx, execStep := range specDoc.Steps {
		runtimeStep := execStep
		runtimeStep.CA = mergeUniqueStringEntries(append([]string(nil), execStep.CA...), phaseCA)
		runtimeStep.Envs = mergeHookRuntimeDecisionEnv(runtimeStep.Envs, req.HookRuntime)

		manifest, manifestErr := buildManifestFromRequest(req, hookStepRunOptions(runtimeStep, req.TypedOptions.BundleMap), 0, stack)
		if manifestErr != nil {
			err = fmt.Errorf("hook[%d] step[%d] build runtime manifest: %w", hookIndex, stepIdx, manifestErr)
			slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "hook_index", hookIndex, "step_idx", stepIdx, "error", err)
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}

		stepOutPath := outPath
		if stepIdx+1 < len(specDoc.Steps) {
			stepOutPath = filepath.Join(gateCycleHookDir(req.RunID, cycleName, hookIndex), "steps", fmt.Sprintf("%03d", stepIdx), preGateCanonicalSBOMFileName)
		}
		cfg := standardJobConfig{
			Manifest:      manifest,
			DiffType:      types.DiffJobTypeMig,
			OutDirPattern: "ploy-hook-out-*",
			InDirPattern:  "ploy-hook-in-*",
			PopulateInDir: func(inDir string) error {
				if err := r.materializeJavaClasspathInDir(ctx, req, inDir); err != nil {
					return err
				}
				inPath := filepath.Join(inDir, preGateCanonicalSBOMFileName)
				if err := copyFileBytes(stepInputPath, inPath); err != nil {
					return fmt.Errorf("stage /in/%s: %w", preGateCanonicalSBOMFileName, err)
				}
				return nil
			},
			ValidateOutputs: func(_, _ string) error {
				return materializeHookSnapshot(stepInputPath, stepOutPath)
			},
			WorkspacePolicy: workspaceChangePolicyIgnore,
			UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baseDir, workspace string, result step.Result) {
				r.uploadDiffWithBaseline(ctx, runID, jobID, jobName, diffGen, baseDir, workspace, result, types.DiffJobTypeMig, false)
			},
			SuppressTerminalStatus: true,
			SuppressOutBundle:      stepIdx+1 < len(specDoc.Steps),
			StartTime:              startTime,
		}
		outcome, execErr := r.executeStandardJobWithOutcome(ctx, req, cfg)
		if execErr != nil {
			err = fmt.Errorf("hook[%d] step[%d] execute runtime step: %w", hookIndex, stepIdx, execErr)
			slog.Error("failed to execute hook job", "run_id", req.RunID, "job_id", req.JobID, "hook_index", hookIndex, "step_idx", stepIdx, "error", err)
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
		finalOutcome = outcome
		completedStepName = strings.TrimSpace(execStep.Name)
		if outcome.runErr != nil || outcome.result.ExitCode != 0 {
			break
		}
		stepInputPath = stepOutPath
	}

	if finalOutcome.duration == 0 {
		finalOutcome.duration = time.Since(startTime)
	}
	statsBuilder := types.NewRunStatsBuilder().
		ExitCode(finalOutcome.result.ExitCode).
		DurationMs(finalOutcome.duration.Milliseconds()).
		TimingsFromDurations(
			time.Duration(finalOutcome.result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(finalOutcome.result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(finalOutcome.result.Timings.DiffDuration).Milliseconds(),
			time.Duration(finalOutcome.result.Timings.TotalDuration).Milliseconds(),
		).
		MetadataEntry("cycle_name", cycleName).
		MetadataEntry("hook_index", strconv.Itoa(hookIndex)).
		MetadataEntry("hook_source", hookSource).
		MetadataEntry("hook_condition_result", conditionJSON).
		MetadataEntry("hook_command_identity", commandIdentityJSON).
		MetadataEntry("hook_command_executed", "true")
	if completedStepName != "" {
		statsBuilder.MetadataEntry("hook_step_name", completedStepName)
	}
	if req.HookRuntime != nil {
		if hash := strings.TrimSpace(req.HookRuntime.HookHash); hash != "" {
			statsBuilder.MetadataEntry("hook_hash", hash)
		}
		statsBuilder.MetadataEntry("hook_should_run", strconv.FormatBool(req.HookRuntime.HookShouldRun))
		if pkg := strings.TrimSpace(req.HookRuntime.MatchedPackage); pkg != "" {
			statsBuilder.MetadataEntry("hook_matched_package", pkg)
		}
		if prev := strings.TrimSpace(req.HookRuntime.PreviousVersion); prev != "" {
			statsBuilder.MetadataEntry("hook_previous_version", prev)
		}
		if cur := strings.TrimSpace(req.HookRuntime.CurrentVersion); cur != "" {
			statsBuilder.MetadataEntry("hook_current_version", cur)
		}
		if pred := strings.TrimSpace(req.HookRuntime.MatchedPredicate); pred != "" {
			statsBuilder.MetadataEntry("hook_matched_predicate", pred)
		}
	}
	if resources := runStatsJobResourcesFromStepUsage(finalOutcome.result.ContainerResources); resources != nil {
		statsBuilder.JobResources(resources)
	}
	r.reportTerminalStatus(ctx, req, finalOutcome.runErr, finalOutcome.result, statsBuilder.MustBuild(), finalOutcome.repoSHAOut, finalOutcome.duration)

	slog.Info("hook job scheduled for runtime execution",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"job_name", req.JobName,
		"cycle_name", cycleName,
		"hook_index", hookIndex,
		"hook_source", hookSource,
		"hook_steps", len(specDoc.Steps),
		"sbom_input", "/in/"+preGateCanonicalSBOMFileName,
	)
}

func addHookRuntimeMetadata(statsBuilder *types.RunStatsBuilder, decision *contracts.HookRuntimeDecision) {
	if statsBuilder == nil || decision == nil {
		return
	}
	if hash := strings.TrimSpace(decision.HookHash); hash != "" {
		statsBuilder.MetadataEntry("hook_hash", hash)
	}
	statsBuilder.MetadataEntry("hook_should_run", strconv.FormatBool(decision.HookShouldRun))
	if pkg := strings.TrimSpace(decision.MatchedPackage); pkg != "" {
		statsBuilder.MetadataEntry("hook_matched_package", pkg)
	}
	if prev := strings.TrimSpace(decision.PreviousVersion); prev != "" {
		statsBuilder.MetadataEntry("hook_previous_version", prev)
	}
	if cur := strings.TrimSpace(decision.CurrentVersion); cur != "" {
		statsBuilder.MetadataEntry("hook_current_version", cur)
	}
	if pred := strings.TrimSpace(decision.MatchedPredicate); pred != "" {
		statsBuilder.MetadataEntry("hook_matched_predicate", pred)
	}
}

func mergeHookRuntimeDecisionEnv(base map[string]string, decision *contracts.HookRuntimeDecision) map[string]string {
	if decision == nil {
		return copyStringMap(base)
	}
	merged := copyStringMap(base)
	if merged == nil {
		merged = map[string]string{}
	}
	if pred := strings.TrimSpace(decision.MatchedPredicate); pred != "" {
		merged["PLOY_HOOK_MATCHED_PREDICATE"] = pred
	}
	if name := strings.TrimSpace(decision.MatchedPackage); name != "" {
		merged["PLOY_HOOK_MATCHED_PACKAGE"] = name
	}
	if prev := strings.TrimSpace(decision.PreviousVersion); prev != "" {
		merged["PLOY_HOOK_PREVIOUS_VERSION"] = prev
	}
	if cur := strings.TrimSpace(decision.CurrentVersion); cur != "" {
		merged["PLOY_HOOK_CURRENT_VERSION"] = cur
	}
	return merged
}

func (r *runController) loadHookSpecForExecution(ctx context.Context, req StartRunRequest, source string) (hook.Spec, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return hook.Spec{}, fmt.Errorf("hook source is empty")
	}
	if hookBundleSourcePattern.MatchString(source) {
		return r.loadHookSpecFromBundleHash(ctx, req, source)
	}
	if isHTTPSHookSource(source) {
		specs, err := hook.NewLoader(nil).LoadFromMigSpec(contracts.MigSpec{
			Hooks: []string{source},
		}, ".")
		if err != nil {
			return hook.Spec{}, err
		}
		if len(specs) == 0 {
			return hook.Spec{}, fmt.Errorf("no resolved hook spec for source %q", source)
		}
		return specs[0], nil
	}
	return hook.Spec{}, fmt.Errorf("unsupported hook source %q", source)
}

func (r *runController) loadHookSpecFromBundleHash(ctx context.Context, req StartRunRequest, hash string) (hook.Spec, error) {
	if r.artifactUploader == nil {
		return hook.Spec{}, fmt.Errorf("artifact uploader is required")
	}
	bundleID := strings.TrimSpace(req.TypedOptions.BundleMap[hash])
	if bundleID == "" {
		return hook.Spec{}, fmt.Errorf("bundle_map[%q] is missing", hash)
	}
	data, err := r.artifactUploader.DownloadSpecBundle(ctx, bundleID)
	if err != nil {
		return hook.Spec{}, fmt.Errorf("download spec bundle %q: %w", bundleID, err)
	}
	if err := verifyDigestPrefix(data, hash); err != nil {
		return hook.Spec{}, fmt.Errorf("verify digest prefix for bundle %q: %w", bundleID, err)
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

func isHTTPSHookSource(source string) bool {
	parsed, err := url.Parse(strings.TrimSpace(source))
	if err != nil || parsed == nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
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

func hookStepRunOptions(stepSpec hook.Step, bundleMap map[string]string) RunOptions {
	return RunOptions{
		Steps: []StepMig{
			{
				MigContainerSpec: MigContainerSpec{
					Image:   stepSpec.ToJobImage(),
					Command: stepSpec.ToCommandSpec(),
					Env:     copyStringMap(stepSpec.Envs),
					CA:      append([]string(nil), stepSpec.CA...),
					In:      append([]string(nil), stepSpec.In...),
					Out:     append([]string(nil), stepSpec.Out...),
					Home:    append([]string(nil), stepSpec.Home...),
				},
			},
		},
		BundleMap: bundleMap,
	}
}

func materializeHookSnapshot(inputSnapshotPath, snapshotPath string) error {
	// Hooks mutate /workspace and do not produce SBOM output artifacts.
	// Preserve the input snapshot for downstream gate-cycle consumers.
	if err := copyFileBytes(inputSnapshotPath, snapshotPath); err != nil {
		return fmt.Errorf("stage hook snapshot %s: %w", snapshotPath, err)
	}
	return nil
}

func encodeHookConditionResult(decision *contracts.HookRuntimeDecision) string {
	payload := struct {
		Evaluated bool   `json:"evaluated"`
		ShouldRun bool   `json:"should_run"`
		Hash      string `json:"hash,omitempty"`
	}{
		Evaluated: decision != nil,
		ShouldRun: decision != nil && decision.HookShouldRun,
	}
	if decision != nil {
		payload.Hash = strings.TrimSpace(decision.HookHash)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"evaluated":%t,"should_run":%t}`, payload.Evaluated, payload.ShouldRun)
	}
	return string(raw)
}

func encodeHookCommandIdentityList(source string, steps []hook.Step, stack contracts.MigStack) string {
	type identityStep struct {
		Name    string   `json:"name,omitempty"`
		Image   string   `json:"image"`
		Command []string `json:"command,omitempty"`
	}
	stepList := make([]identityStep, 0, len(steps))
	for _, stepSpec := range steps {
		image, err := stepSpec.Image.ResolveImage(stack)
		if err != nil {
			image = stepSpec.Image.String()
		}
		stepList = append(stepList, identityStep{
			Name:    strings.TrimSpace(stepSpec.Name),
			Image:   strings.TrimSpace(image),
			Command: append([]string(nil), stepSpec.Command...),
		})
	}
	payload := struct {
		Source string         `json:"source,omitempty"`
		Steps  []identityStep `json:"steps"`
	}{
		Source: strings.TrimSpace(source),
		Steps:  stepList,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

// executeMigJob runs a mig container job.
// Executes the container, uploads diff, and reports status.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures mig steps
// use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeMigJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// MigStackUnknown which falls back to "default" in stack maps.
	stack := resolveManifestStack(req, r.loadPersistedStack(req.RunID))

	// Build manifest with stack-aware image resolution using typed options.
	typedOpts := req.TypedOptions
	stepIdx := 0
	if len(typedOpts.Steps) > 0 {
		if req.MigContext != nil {
			stepIdx = req.MigContext.StepIndex
		} else {
			idx, err := migStepIndexFromJobName(req.JobName, len(typedOpts.Steps))
			if err != nil {
				err = fmt.Errorf("derive mig step index from job_name: %w", err)
				slog.Error("failed to derive mig step index", "run_id", req.RunID, "job_id", req.JobID, "error", err)
				r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
				return
			}
			stepIdx = idx
		}
		if stepIdx < 0 || stepIdx >= len(typedOpts.Steps) {
			err := fmt.Errorf("derived mig step index out of range: derived=%d steps_len=%d", stepIdx, len(typedOpts.Steps))
			slog.Error("derived mig step index out of range", "run_id", req.RunID, "job_id", req.JobID, "derived_index", stepIdx, "steps_len", len(typedOpts.Steps))
			r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
			return
		}
	}
	manifest, err := buildManifestFromRequest(req, typedOpts, stepIdx, stack)
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("mig job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)

	cfg := standardJobConfig{
		Manifest:      manifest,
		DiffType:      types.DiffJobTypeMig,
		OutDirPattern: "ploy-mig-out-*",
		InDirPattern:  "ploy-mig-in-*",
		PopulateInDir: func(inDir string) error {
			if err := r.materializeJavaClasspathInDir(ctx, req, inDir); err != nil {
				return err
			}
			return r.materializeMigInFromInputs(ctx, req, inDir)
		},
		PrepareManifest: func(m *contracts.StepManifest, ws string) error {
			r.injectChildBuildRuntimeEnvVars(m, ws, req.JobID)
			r.mountChildBuildTLSCerts(m)
			return nil
		},
		RuntimeSync: func(outDir, _ string) error {
			return r.materializeParentChildBuildLineage(outDir, req.RecoveryContext)
		},
		FinalizeOutputs: func(outDir, _ string) error {
			return r.materializeParentChildBuildLineage(outDir, req.RecoveryContext)
		},
		WorkspacePolicy:           workspaceChangePolicyIgnore,
		UploadConfiguredArtifacts: true,
		UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baseDir, workspace string, result step.Result) {
			r.uploadDiffWithBaseline(ctx, runID, jobID, jobName, diffGen, baseDir, workspace, result, types.DiffJobTypeMig, true)
		},
		StartTime: startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// executeHealingJob runs a healing container job.
// Fetches gate logs from parent job, runs healing container, uploads diff.
//
// Stack-aware image selection: The job loads the persisted stack from the
// pre-gate phase and uses it for manifest building. This ensures healing
// migs use stack-specific images (e.g., java-maven, java-gradle) when configured.
func (r *runController) executeHealingJob(ctx context.Context, req StartRunRequest) {
	startTime := time.Now()

	// Load the persisted stack from the pre-gate phase for stack-aware image
	// selection. If no stack was persisted (e.g., gate skipped), defaults to
	// MigStackUnknown which falls back to "default" in stack maps.
	stack := resolveManifestStack(req, r.loadPersistedStack(req.RunID))
	if stack == contracts.MigStackUnknown && req.RecoveryContext != nil && req.RecoveryContext.DetectedStack != "" {
		stack = req.RecoveryContext.DetectedStack
	}

	// Build manifest with stack-aware image resolution using typed options.
	// stepIndex=0 is used for manifest building; job configuration comes from req.TypedOptions.
	typedOpts := req.TypedOptions

	var manifest contracts.StepManifest
	var err error

	if typedOpts.Healing == nil || typedOpts.Healing.Mig.Image.IsEmpty() {
		err = fmt.Errorf("healing job missing heal container image")
	} else {
		healMig := typedOpts.Healing.Mig
		manifest, err = buildHealingManifest(req, healMig, 0, "", stack)
	}
	if err != nil {
		slog.Error("failed to build manifest", "run_id", req.RunID, "error", err)
		r.uploadFailureStatus(ctx, req, err, time.Since(startTime))
		return
	}

	// Log the stack-aware image selection for observability.
	slog.Info("healing job using stack-aware image",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"detected_stack", stack,
		"resolved_image", manifest.Image,
	)
	workspacePolicy := resolveHealingWorkspacePolicy(req.RecoveryContext)
	schemaJSON := ""
	if req.Env != nil {
		schemaJSON = strings.TrimSpace(req.Env[contracts.GateProfileSchemaJSONEnv])
	}

	cfg := standardJobConfig{
		Manifest:      manifest,
		DiffType:      types.DiffJobTypeHealing,
		OutDirPattern: "ploy-heal-out-*",
		InDirPattern:  "ploy-heal-in-*",
		PopulateInDir: func(inDir string) error {
			if err := r.materializeJavaClasspathInDir(ctx, req, inDir); err != nil {
				return err
			}
			return r.populateHealingInDir(req.RunID, inDir, req.RecoveryContext, schemaJSON)
		},
		PrepareManifest: func(m *contracts.StepManifest, ws string) error {
			r.injectHealingEnvVars(m, ws, req.JobID)
			r.mountHealingTLSCerts(m)
			return nil
		},
		RuntimeSync: func(outDir, _ string) error {
			return r.materializeParentChildBuildLineage(outDir, req.RecoveryContext)
		},
		FinalizeOutputs: func(outDir, _ string) error {
			return r.materializeParentChildBuildLineage(outDir, req.RecoveryContext)
		},
		WorkspacePolicy: workspacePolicy,
		UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baseDir, workspace string, result step.Result) {
			r.uploadDiffWithBaseline(ctx, runID, jobID, jobName, diffGen, baseDir, workspace, result, types.DiffJobTypeHealing, false)
		},
		BuildJobMeta: func(outDir string) json.RawMessage {
			bugSummary := parseBugSummary(outDir)
			actionSummary := parseActionSummary(outDir)
			errorKind := parseErrorKind(outDir)
			if bugSummary == "" && actionSummary == "" && errorKind == "" {
				return nil
			}
			heal := &contracts.HealJobMetadata{
				BugSummary:    bugSummary,
				ActionSummary: actionSummary,
				ErrorKind:     errorKind,
			}
			meta := &contracts.JobMeta{
				Kind: contracts.JobKindMig,
				Heal: heal,
			}
			data, err := contracts.MarshalJobMeta(meta)
			if err != nil {
				slog.Warn("failed to marshal healing job meta", "run_id", req.RunID, "job_id", req.JobID, "error", err)
				return nil
			}
			return data
		},
		StartTime: startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// materializeGateSBOMForGate copies the final cycle SBOM snapshot to build-gate
// /out so gate jobs expose a stable output contract.
func materializeGateSBOMForGate(runID types.RunID, cycleName string, hooks []string, workspace string) error {
	snapshotPath := gateCycleFinalSnapshotPath(runID, cycleName, hooks)
	gateOutDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	sbomOutPath := filepath.Join(gateOutDir, preGateCanonicalSBOMFileName)
	if err := copyFileBytes(snapshotPath, sbomOutPath); err != nil {
		return fmt.Errorf("materialize %s sbom for gate /out: %w", cycleName, err)
	}
	return nil
}

// materializePreGateSBOMForGate preserves existing pre-gate helper callers.
func materializePreGateSBOMForGate(runID types.RunID, hooks []string, workspace string) error {
	return materializeGateSBOMForGate(runID, preGateCycleName, hooks, workspace)
}

type canonicalSBOMDocument struct {
	SPDXVersion       string                 `json:"spdxVersion"`
	DataLicense       string                 `json:"dataLicense"`
	SPDXID            string                 `json:"SPDXID"`
	Name              string                 `json:"name"`
	DocumentNamespace string                 `json:"documentNamespace"`
	CreationInfo      canonicalCreationInfo  `json:"creationInfo"`
	Packages          []canonicalSBOMPackage `json:"packages"`
}

type canonicalCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type canonicalSBOMPackage struct {
	SPDXID      string `json:"SPDXID,omitempty"`
	Name        string `json:"name"`
	VersionInfo string `json:"versionInfo"`
}

var (
	hookBundleSourcePattern = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
	gradleDependencyPattern = regexp.MustCompile(`([A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+):([A-Za-z0-9][A-Za-z0-9+_.-]*)`)
	gradleOverridePattern   = regexp.MustCompile(`->\s*([A-Za-z0-9][A-Za-z0-9+_.-]*)`)
	sbomNameTokenPattern    = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	sbomVersionTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]*$`)
)

func canonicalSBOMFromDependencyOutput(raw []byte) ([]byte, error) {
	packages := collectSBOMPackages(raw)
	doc := canonicalSBOMDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              "ploy-generated-sbom",
		DocumentNamespace: "https://ploy.dev/sbom/generated",
		CreationInfo: canonicalCreationInfo{
			Created:  "1970-01-01T00:00:00Z",
			Creators: []string{"Tool: ploy-nodeagent"},
		},
		Packages: make([]canonicalSBOMPackage, len(packages)),
	}
	copy(doc.Packages, packages)
	for i := range doc.Packages {
		doc.Packages[i].SPDXID = fmt.Sprintf("SPDXRef-Package-%06d", i+1)
	}
	return json.MarshalIndent(doc, "", "  ")
}

func collectSBOMPackages(raw []byte) []canonicalSBOMPackage {
	lines := strings.Split(string(raw), "\n")
	packages := make([]canonicalSBOMPackage, 0)
	seen := make(map[string]struct{})
	for _, line := range lines {
		if name, version, ok := parseMavenDependencyLine(line); ok {
			packages = appendUniqueSBOMPackage(packages, seen, name, version)
			continue
		}
		if name, version, ok := parseGradleDependencyLine(line); ok {
			packages = appendUniqueSBOMPackage(packages, seen, name, version)
		}
	}
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name == packages[j].Name {
			return packages[i].VersionInfo < packages[j].VersionInfo
		}
		return packages[i].Name < packages[j].Name
	})
	return packages
}

func appendUniqueSBOMPackage(
	packages []canonicalSBOMPackage,
	seen map[string]struct{},
	name string,
	version string,
) []canonicalSBOMPackage {
	key := name + "\x00" + version
	if _, exists := seen[key]; exists {
		return packages
	}
	seen[key] = struct{}{}
	return append(packages, canonicalSBOMPackage{
		Name:        name,
		VersionInfo: version,
	})
}

func parseMavenDependencyLine(line string) (string, string, bool) {
	fields := strings.Fields(line)
	for _, field := range fields {
		token := strings.Trim(strings.TrimSpace(field), ",;")
		if strings.Count(token, ":") < 4 {
			continue
		}
		parts := strings.Split(token, ":")
		if len(parts) < 5 {
			continue
		}
		nameGroup := strings.TrimSpace(parts[0])
		nameArtifact := strings.TrimSpace(parts[1])
		version := strings.TrimSpace(parts[len(parts)-2])
		scope := strings.TrimSpace(parts[len(parts)-1])
		if !sbomNameTokenPattern.MatchString(nameGroup) || !sbomNameTokenPattern.MatchString(nameArtifact) {
			continue
		}
		if !sbomVersionTokenPattern.MatchString(version) || scope == "" {
			continue
		}
		return nameGroup + ":" + nameArtifact, version, true
	}
	return "", "", false
}

func parseGradleDependencyLine(line string) (string, string, bool) {
	if !strings.Contains(line, "---") {
		return "", "", false
	}
	matches := gradleDependencyPattern.FindStringSubmatch(line)
	if len(matches) != 3 {
		return "", "", false
	}

	name := strings.TrimSpace(matches[1])
	version := strings.TrimSpace(matches[2])
	if override := gradleOverridePattern.FindStringSubmatch(line); len(override) == 2 {
		version = strings.TrimSpace(override[1])
	}
	parts := strings.Split(name, ":")
	if len(parts) != 2 {
		return "", "", false
	}
	if !sbomNameTokenPattern.MatchString(parts[0]) || !sbomNameTokenPattern.MatchString(parts[1]) {
		return "", "", false
	}
	if !sbomVersionTokenPattern.MatchString(version) {
		return "", "", false
	}
	return name, version, true
}

func validateCanonicalSBOMPath(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return validateCanonicalSBOMDocument(raw)
}

func validateJavaClasspathPath(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for idx, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		entry := strings.TrimSpace(line)
		if entry == "" {
			continue
		}
		if !filepath.IsAbs(entry) {
			return fmt.Errorf("line %d must be absolute path: %q", idx+1, entry)
		}
		if entry == "/home/gradle/.gradle" || strings.HasPrefix(entry, "/home/gradle/.gradle/") {
			return fmt.Errorf("line %d uses non-portable gradle cache path: %q (expected /root/.gradle/...)", idx+1, entry)
		}
	}
	return nil
}

func validateCanonicalSBOMDocument(raw []byte) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("sbom payload is empty")
	}
	var doc canonicalSBOMDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse json: %w", err)
	}
	if strings.TrimSpace(doc.SPDXVersion) != "SPDX-2.3" {
		return fmt.Errorf("spdxVersion must be SPDX-2.3")
	}
	if doc.Packages == nil {
		return fmt.Errorf("packages array is required")
	}
	for i, pkg := range doc.Packages {
		if strings.TrimSpace(pkg.Name) == "" {
			return fmt.Errorf("packages[%d].name is required", i)
		}
		if strings.TrimSpace(pkg.VersionInfo) == "" {
			return fmt.Errorf("packages[%d].versionInfo is required", i)
		}
	}
	return nil
}

// standardJobConfig configures the execution of a standard container job
// (mig/heal/sbom).
type standardJobConfig struct {
	Manifest      contracts.StepManifest
	DiffType      types.DiffJobType
	OutDirPattern string
	InDirPattern  string

	PopulateInDir   func(inDir string) error
	PrepareManifest func(manifest *contracts.StepManifest, workspace string) error
	RuntimeSync     func(outDir, workspace string) error
	ValidateOutputs func(outDir, workspace string) error
	FinalizeOutputs func(outDir, workspace string) error
	TrySkip         func(ctx context.Context, manifest contracts.StepManifest, workspace, outDir string) (bool, error)

	WorkspacePolicy           workspaceChangePolicy
	UploadConfiguredArtifacts bool

	UploadDiff    func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, baselineDir, workspace string, result step.Result)
	BuildJobMeta  func(outDir string) json.RawMessage
	BuildMetadata func(outDir string) map[string]string

	SuppressTerminalStatus bool
	SuppressOutBundle      bool

	StartTime time.Time
}

type standardJobOutcome struct {
	runErr     error
	result     step.Result
	repoSHAOut string
	duration   time.Duration
}

// executeStandardJob orchestrates the common lifecycle of a container job
// (mig/heal/sbom):
// runtime init, rehydration, snapshots, directory prep, execution, and uploading.
func (r *runController) executeStandardJob(ctx context.Context, req StartRunRequest, cfg standardJobConfig) {
	_, execErr := r.executeStandardJobWithOutcome(ctx, req, cfg)
	if execErr == nil {
		return
	}
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}
	slog.Error("standard job execution failed", "run_id", req.RunID, "job_id", req.JobID, "error", execErr)
	r.uploadFailureStatus(ctx, req, execErr, time.Since(startTime))
}

func (r *runController) executeStandardJobWithOutcome(ctx context.Context, req StartRunRequest, cfg standardJobConfig) (standardJobOutcome, error) {
	startTime := cfg.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}
	var outcome standardJobOutcome

	execCtx, cleanup, err := r.initJobExecutionContext(ctx, req.RunID, req.JobID)
	if err != nil {
		return outcome, fmt.Errorf("initialize runtime: %w", err)
	}
	defer cleanup()

	wsResult, err := r.rehydrateWorkspaceWithCleanup(ctx, req, cfg.Manifest)
	if err != nil {
		return outcome, fmt.Errorf("rehydrate workspace: %w", err)
	}
	defer wsResult.cleanup()
	workspace := wsResult.path

	var baselineDir string
	if execCtx.diffGenerator != nil {
		snapshot := snapshotWorkspaceForNoIndexDiff(req.RunID, req.JobID, cfg.DiffType, workspace)
		defer snapshot.cleanup()
		baselineDir = snapshot.path
	}

	outDirErr := withTempDir(cfg.OutDirPattern, func(outDir string) error {
		if cfg.InDirPattern != "" {
			return withTempDir(cfg.InDirPattern, func(inDir string) error {
				if cfg.PopulateInDir != nil {
					if err := cfg.PopulateInDir(inDir); err != nil {
						return fmt.Errorf("populate in dir: %w", err)
					}
				}
				stepOutcome, err := r.runContainerJob(ctx, req, cfg, execCtx, baselineDir, workspace, startTime, outDir, inDir)
				if err != nil {
					return err
				}
				outcome = stepOutcome
				return nil
			})
		}
		stepOutcome, err := r.runContainerJob(ctx, req, cfg, execCtx, baselineDir, workspace, startTime, outDir, "")
		if err != nil {
			return err
		}
		outcome = stepOutcome
		return nil
	})

	if outDirErr != nil {
		return outcome, outDirErr
	}
	return outcome, nil
}

// runContainerJob executes the container, uploads artifacts/diffs, and reports terminal status.
// Extracted from executeStandardJob to keep function sizes under ~100 lines.
func (r *runController) runContainerJob(
	ctx context.Context,
	req StartRunRequest,
	cfg standardJobConfig,
	execCtx jobExecutionContext,
	baselineDir, workspace string,
	startTime time.Time,
	outDir, inDir string,
) (standardJobOutcome, error) {
	outcome := standardJobOutcome{}
	manifest := cfg.Manifest
	disableManifestGate(&manifest)
	clearManifestHydration(&manifest)

	if cfg.PrepareManifest != nil {
		if err := cfg.PrepareManifest(&manifest, workspace); err != nil {
			return outcome, fmt.Errorf("prepare manifest: %w", err)
		}
	}

	imageName := strings.TrimSpace(manifest.Image)
	if imageName == "" {
		return outcome, fmt.Errorf("resolved job image is empty")
	}
	if err := r.SaveJobImageName(ctx, req.JobID, imageName); err != nil {
		return outcome, fmt.Errorf("save job image name: %w", err)
	}

	var result step.Result
	var runErr error
	var duration time.Duration
	if cfg.TrySkip != nil {
		skipped, err := cfg.TrySkip(ctx, manifest, workspace, outDir)
		if err != nil {
			return outcome, fmt.Errorf("evaluate skip: %w", err)
		}
		if skipped {
			duration := time.Since(startTime)
			if runErr == nil {
				if captureErr := r.captureJavaClasspathAfterStandardJob(req, inDir, outDir); captureErr != nil {
					runErr = fmt.Errorf("capture java classpath outputs: %w", captureErr)
				}
			}
			if runErr == nil && cfg.ValidateOutputs != nil {
				if validateErr := cfg.ValidateOutputs(outDir, workspace); validateErr != nil {
					runErr = fmt.Errorf("validate job outputs: %w", validateErr)
				}
			}
			runErr = r.finalizeStandardJobOutputs(req, cfg, outDir, workspace, runErr, step.Result{})
			repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace, "")
			statsBuilder := types.NewRunStatsBuilder().
				ExitCode(0).
				DurationMs(duration.Milliseconds())
			stats := statsBuilder.MustBuild()
			if !cfg.SuppressOutBundle {
				if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, outDir, "mig-out"); err != nil {
					slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
				}
			}
			outcome = standardJobOutcome{
				runErr:     runErr,
				result:     step.Result{},
				repoSHAOut: repoSHAOut,
				duration:   duration,
			}
			if !cfg.SuppressTerminalStatus {
				r.reportTerminalStatus(ctx, req, runErr, step.Result{}, stats, repoSHAOut, duration)
			}
			return outcome, nil
		}
	}

	var preStatus string
	var preStatusErr error
	if cfg.WorkspacePolicy != workspaceChangePolicyIgnore {
		preStatus, preStatusErr = gitpkg.WorkspaceStatus(ctx, workspace)
		if preStatusErr != nil {
			slog.Warn("failed to compute workspace status before execution", "run_id", req.RunID, "error", preStatusErr)
		}
	}
	preWorkspaceTree := ""
	if tree, treeErr := gitpkg.ComputeWorkspaceTreeSHA(ctx, workspace); treeErr != nil {
		slog.Warn("failed to compute pre-execution workspace tree", "run_id", req.RunID, "job_id", req.JobID, "error", treeErr)
	} else {
		preWorkspaceTree = tree
	}

	// Materialize Hydra resources into a staging directory for mount planning.
	stopRuntimeSync := r.startRuntimeOutputSyncLoop(ctx, req, cfg, outDir, workspace)
	if bundleErr := r.withMaterializedResources(ctx, manifest, req.TypedOptions.BundleMap, "ploy-staging-*", func(stagingDir string) error {
		result, runErr = execCtx.runner.Run(ctx, step.Request{
			RunID:      req.RunID,
			JobID:      req.JobID,
			Manifest:   manifest,
			Workspace:  workspace,
			OutDir:     outDir,
			InDir:      inDir,
			StagingDir: stagingDir,
		})
		duration = time.Since(startTime)
		return nil
	}); bundleErr != nil {
		stopRuntimeSync()
		return outcome, bundleErr
	}
	stopRuntimeSync()

	if runErr == nil && result.ExitCode == 0 && cfg.ValidateOutputs != nil {
		if validateErr := cfg.ValidateOutputs(outDir, workspace); validateErr != nil {
			runErr = fmt.Errorf("validate job outputs: %w", validateErr)
		}
	}
	if runErr == nil && result.ExitCode == 0 {
		if captureErr := r.captureJavaClasspathAfterStandardJob(req, inDir, outDir); captureErr != nil {
			runErr = fmt.Errorf("capture java classpath outputs: %w", captureErr)
		}
	}
	runErr = r.finalizeStandardJobOutputs(req, cfg, outDir, workspace, runErr, result)
	duration = time.Since(startTime)

	if runErr != nil || result.ExitCode != 0 {
		if preserveRoot, preserveErr := preserveFailureArtifacts(req.RunID, req.JobID, workspace, outDir, inDir); preserveErr != nil {
			slog.Warn("failed to preserve failure artifacts", "run_id", req.RunID, "job_id", req.JobID, "error", preserveErr)
		} else {
			slog.Info("preserved failure artifacts", "run_id", req.RunID, "job_id", req.JobID, "path", preserveRoot)
		}
	}

	if cfg.UploadDiff != nil {
		cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, baselineDir, workspace, result)
	}

	if !cfg.SuppressOutBundle {
		if err := r.uploadOutDirBundle(ctx, req.RunID, req.JobID, outDir, "mig-out"); err != nil {
			slog.Warn("/out artifact upload failed", "run_id", req.RunID, "job_id", req.JobID, "next_id", req.NextID, "error", err)
		}
	}

	if cfg.UploadConfiguredArtifacts {
		r.uploadConfiguredArtifacts(ctx, req, req.TypedOptions, manifest, workspace, outDir)
	}

	if cfg.WorkspacePolicy != workspaceChangePolicyIgnore && runErr == nil && result.ExitCode == 0 && preStatusErr == nil {
		postStatus, postErr := gitpkg.WorkspaceStatus(ctx, workspace)
		if postErr == nil {
			if warning, violated := validateWorkspacePolicy(cfg.WorkspacePolicy, preStatus, postStatus); violated {
				r.uploadHealingWorkspacePolicyFailure(ctx, req, warning, duration)
				return outcome, nil
			}
		}
	}

	repoSHAOut := r.computeRepoSHAOut(ctx, req, workspace, preWorkspaceTree)

	statsBuilder := types.NewRunStatsBuilder().
		ExitCode(result.ExitCode).
		DurationMs(duration.Milliseconds()).
		TimingsFromDurations(
			time.Duration(result.Timings.HydrationDuration).Milliseconds(),
			time.Duration(result.Timings.ExecutionDuration).Milliseconds(),
			time.Duration(result.Timings.DiffDuration).Milliseconds(),
			time.Duration(result.Timings.TotalDuration).Milliseconds(),
		)
	if resources := runStatsJobResourcesFromStepUsage(result.ContainerResources); resources != nil {
		statsBuilder.JobResources(resources)
	}

	if cfg.BuildJobMeta != nil {
		if meta := cfg.BuildJobMeta(outDir); len(meta) > 0 {
			statsBuilder.JobMeta(meta)
		}
	}
	if cfg.BuildMetadata != nil {
		for k, v := range cfg.BuildMetadata(outDir) {
			if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
				continue
			}
			statsBuilder.MetadataEntry(k, v)
		}
	}
	if orwMeta, orwErr := parseORWFailureMetadata(outDir); orwErr != nil {
		slog.Warn("failed to parse ORW report metadata", "run_id", req.RunID, "job_id", req.JobID, "error", orwErr)
	} else {
		for k, v := range orwMeta {
			statsBuilder.MetadataEntry(k, v)
		}
	}

	stats := statsBuilder.MustBuild()
	outcome = standardJobOutcome{
		runErr:     runErr,
		result:     result,
		repoSHAOut: repoSHAOut,
		duration:   duration,
	}
	if !cfg.SuppressTerminalStatus {
		r.reportTerminalStatus(ctx, req, runErr, result, stats, repoSHAOut, duration)
	}
	return outcome, nil
}

func (r *runController) finalizeStandardJobOutputs(
	req StartRunRequest,
	cfg standardJobConfig,
	outDir, workspace string,
	runErr error,
	result step.Result,
) error {
	if cfg.FinalizeOutputs == nil {
		return runErr
	}
	if finalizeErr := cfg.FinalizeOutputs(outDir, workspace); finalizeErr != nil {
		// Keep non-zero container exits mapped to their original fail/error
		// semantics while still attempting lineage finalization.
		if runErr != nil || result.ExitCode != 0 {
			slog.Warn("failed to finalize job outputs after non-zero execution",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", finalizeErr)
			return runErr
		}
		return fmt.Errorf("finalize job outputs: %w", finalizeErr)
	}
	return runErr
}

func (r *runController) startRuntimeOutputSyncLoop(
	ctx context.Context,
	req StartRunRequest,
	cfg standardJobConfig,
	outDir, workspace string,
) func() {
	if cfg.RuntimeSync == nil {
		return func() {}
	}

	done := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				if err := cfg.RuntimeSync(outDir, workspace); err != nil {
					slog.Warn("runtime output sync failed",
						"run_id", req.RunID,
						"job_id", req.JobID,
						"error", err)
				}
			}
		}
	}()

	return func() {
		close(stop)
		<-done
		if err := cfg.RuntimeSync(outDir, workspace); err != nil {
			slog.Warn("runtime output sync final pass failed",
				"run_id", req.RunID,
				"job_id", req.JobID,
				"error", err)
		}
	}
}

func restoreSBOMOutFilesFromBundle(bundle []byte, outDir string) (int, error) {
	if strings.TrimSpace(outDir) == "" {
		return 0, fmt.Errorf("out dir is required")
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(bundle))
	if err != nil {
		return 0, fmt.Errorf("open artifact gzip: %w", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	restored := 0
	for {
		header, err := tarReader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return restored, fmt.Errorf("read artifact tar header: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}

		entry := normalizeBundlePath(header.Name)
		if entry == "" || !(strings.HasPrefix(entry, "out/sbom.") || entry == "out/"+sbomJavaClasspathFileName) {
			continue
		}
		relative := strings.TrimPrefix(entry, "out/")
		targetPath := filepath.Join(outDir, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return restored, fmt.Errorf("mkdir sbom output dir: %w", err)
		}
		payload, readErr := io.ReadAll(tarReader)
		if readErr != nil {
			return restored, fmt.Errorf("read artifact entry %q: %w", entry, readErr)
		}
		if writeErr := os.WriteFile(targetPath, payload, 0o644); writeErr != nil {
			return restored, fmt.Errorf("write sbom output %q: %w", targetPath, writeErr)
		}
		restored++
	}

	if restored == 0 {
		return 0, fmt.Errorf("artifact bundle has no out/sbom.* entries")
	}
	canonicalPath := filepath.Join(outDir, preGateCanonicalSBOMFileName)
	if err := validateCanonicalSBOMPath(canonicalPath); err != nil {
		return restored, fmt.Errorf("validate restored canonical sbom output: %w", err)
	}
	classpathPath := filepath.Join(outDir, sbomJavaClasspathFileName)
	if err := validateJavaClasspathPath(classpathPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return restored, fmt.Errorf("validate restored java classpath output: %w", err)
	}
	return restored, nil
}

func (r *runController) ensureHookSBOMInputSnapshot(ctx context.Context, req StartRunRequest, cycleName, inputSnapshotPath string) error {
	if strings.TrimSpace(inputSnapshotPath) == "" {
		return fmt.Errorf("hook input snapshot path is empty")
	}
	if _, err := os.Stat(inputSnapshotPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat hook input snapshot: %w", err)
	}

	// Only restore from upstream artifacts when the hook expects the cycle SBOM snapshot.
	expectedSBOMInput := gateCycleSBOMOutPath(req.RunID, cycleName)
	if filepath.Clean(inputSnapshotPath) != filepath.Clean(expectedSBOMInput) {
		return fmt.Errorf("hook input snapshot missing: %s", inputSnapshotPath)
	}

	artifactID := ""
	if req.HookContext != nil {
		artifactID = strings.TrimSpace(req.HookContext.UpstreamSBOMArtifactID)
	}
	if artifactID == "" {
		return fmt.Errorf("hook input snapshot missing at %s and upstream sbom artifact id is empty", inputSnapshotPath)
	}
	if r.artifactUploader == nil {
		return fmt.Errorf("artifact uploader is required to restore upstream sbom snapshot")
	}

	bundle, err := r.artifactUploader.DownloadArtifactBundle(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("download upstream sbom artifact %q: %w", artifactID, err)
	}
	restored, err := restoreSBOMOutFilesFromBundle(bundle, filepath.Dir(inputSnapshotPath))
	if err != nil {
		return fmt.Errorf("restore sbom outputs from artifact %q: %w", artifactID, err)
	}
	if _, err := os.Stat(inputSnapshotPath); err != nil {
		return fmt.Errorf("verify restored hook input snapshot %q: %w", inputSnapshotPath, err)
	}
	slog.Info("restored hook sbom input snapshot from artifact",
		"run_id", req.RunID,
		"job_id", req.JobID,
		"cycle_name", cycleName,
		"artifact_id", artifactID,
		"restored_files", restored,
	)
	return nil
}

func normalizeBundlePath(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return ""
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(filepath.ToSlash(n), "/"))
	if cleaned == "/" || strings.HasPrefix(cleaned, "/../") {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}

// withTempDir creates a temporary directory, calls fn, then removes the directory.
func withTempDir(prefix string, fn func(path string) error) error {
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return fmt.Errorf("create temp dir %s: %w", prefix, err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("failed to remove temp dir", "path", dir, "error", err)
		}
	}()

	return fn(dir)
}

// withMaterializedResources materializes Hydra resources (CA/In/Out/Home) from the
// manifest into a staging directory and passes the staging path to fn. When the
// manifest has no Hydra entries, fn receives "".
func (r *runController) withMaterializedResources(ctx context.Context, manifest contracts.StepManifest, bundleMap map[string]string, prefix string, fn func(stagingDir string) error) error {
	hashes := collectUniqueHashes(manifest)
	if len(hashes) == 0 {
		return fn("")
	}
	return withTempDir(prefix, func(dir string) error {
		if err := r.materializeHydraResources(ctx, manifest, bundleMap, dir); err != nil {
			return fmt.Errorf("materialize hydra resources: %w", err)
		}
		return fn(dir)
	})
}

// tempResource holds a temporary path and its cleanup function.
// Used for workspace snapshots, rehydrated workspaces, and similar lifecycle-scoped directories.
type tempResource struct {
	path    string
	cleanup func()
}

// noopTempResource is a zero-value tempResource with a no-op cleanup.
var noopTempResource = tempResource{path: "", cleanup: func() {}}

// snapshotWorkspaceForNoIndexDiff creates a snapshot of the workspace for baseline comparison.
func snapshotWorkspaceForNoIndexDiff(runID types.RunID, jobID types.JobID, diffType types.DiffJobType, workspace string) tempResource {
	jobTypeStr := diffType.String()
	prefix := fmt.Sprintf("ploy-%s-base-*", jobTypeStr)
	snapshotDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to create baseline snapshot directory", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		return noopTempResource
	}

	if err := copyGitClone(workspace, snapshotDir); err != nil {
		slog.Warn(fmt.Sprintf("%s: failed to snapshot baseline workspace", jobTypeStr),
			"run_id", runID, "job_id", jobID, "error", err)
		if rmErr := os.RemoveAll(snapshotDir); rmErr != nil {
			slog.Warn("failed to remove snapshot dir after copy failure", "path", snapshotDir, "error", rmErr)
		}
		return noopTempResource
	}

	return tempResource{
		path: snapshotDir,
		cleanup: func() {
			if err := os.RemoveAll(snapshotDir); err != nil {
				slog.Warn("failed to remove snapshot dir", "path", snapshotDir, "error", err)
			}
		},
	}
}

// rehydrateWorkspaceWithCleanup wraps rehydrateWorkspaceForStep and returns a no-op
// cleanup for sticky run/repo workspaces.
func (r *runController) rehydrateWorkspaceWithCleanup(
	ctx context.Context,
	req StartRunRequest,
	manifest contracts.StepManifest,
) (tempResource, error) {
	workspace, err := r.rehydrateWorkspaceForStep(ctx, req, manifest)
	if err != nil {
		return tempResource{}, err
	}

	return tempResource{
		path:    workspace,
		cleanup: func() {},
	}, nil
}

// clearManifestHydration removes hydration config from manifest inputs to prevent double-hydration.
func clearManifestHydration(manifest *contracts.StepManifest) {
	if len(manifest.Inputs) == 0 {
		return
	}
	inputs := make([]contracts.StepInput, len(manifest.Inputs))
	copy(inputs, manifest.Inputs)
	for i := range inputs {
		inputs[i].Hydration = nil
	}
	manifest.Inputs = inputs
}

// disableManifestGate sets Gate.Enabled=false on the manifest.
func disableManifestGate(manifest *contracts.StepManifest) {
	manifest.Gate = &contracts.StepGateSpec{Enabled: false}
}
