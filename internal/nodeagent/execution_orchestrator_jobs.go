// execution_orchestrator_jobs.go contains mig and sbom job implementations,
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
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	gitpkg "github.com/iw2rmb/ploy/internal/nodeagent/git"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

const preGateCanonicalSBOMFileName = "sbom.spdx.json"

const (
	preGateCycleName  = "pre-gate"
	postGateCycleName = "post-gate"
)

func gateCycleRootDir(runID types.RunID, cycleName string) string {
	return filepath.Join(runCacheDir(runID), "gate-cycles", strings.TrimSpace(cycleName))
}

func gateCycleSBOMOutPath(runID types.RunID, cycleName string) string {
	return filepath.Join(gateCycleRootDir(runID, cycleName), "sbom", "out", preGateCanonicalSBOMFileName)
}

func gateCycleFinalSnapshotPath(runID types.RunID, cycleName string) string {
	return gateCycleSBOMOutPath(runID, cycleName)
}

func gateCycleNameFromGateJob(jobType types.JobType, jobName string) (string, error) {
	switch jobType {
	case types.JobTypePreGate:
		return preGateCycleName, nil
	case types.JobTypePostGate:
		return postGateCycleName, nil
	default:
		return "", fmt.Errorf("unsupported gate job_type %q", jobType)
	}
}

func preGateSBOMOutPath(runID types.RunID) string {
	return gateCycleSBOMOutPath(runID, preGateCycleName)
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
			return r.materializeMigInFromInputs(ctx, req, inDir)
		},
		WorkspacePolicy:           workspaceChangePolicyIgnore,
		UploadConfiguredArtifacts: true,
		UploadDiff: func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, workspace string, result step.Result) {
			r.uploadJobDiff(ctx, runID, jobID, diffGen, workspace, result, types.DiffJobTypeMig)
		},
		StartTime: startTime,
	}

	r.executeStandardJob(ctx, req, cfg)
}

// materializeGateSBOMForGate copies the final cycle SBOM snapshot to build-gate
// /out so gate jobs expose a stable output contract.
func materializeGateSBOMForGate(runID types.RunID, cycleName string, workspace string) error {
	snapshotPath := gateCycleFinalSnapshotPath(runID, cycleName)
	gateOutDir := filepath.Join(workspace, step.BuildGateWorkspaceOutDir)
	sbomOutPath := filepath.Join(gateOutDir, preGateCanonicalSBOMFileName)
	if err := copyFileBytes(snapshotPath, sbomOutPath); err != nil {
		return fmt.Errorf("materialize %s sbom for gate /out: %w", cycleName, err)
	}
	return nil
}

// materializePreGateSBOMForGate preserves existing pre-gate helper callers.
func materializePreGateSBOMForGate(runID types.RunID, workspace string) error {
	return materializeGateSBOMForGate(runID, preGateCycleName, workspace)
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

	UploadDiff    func(ctx context.Context, runID types.RunID, jobID types.JobID, jobName string, diffGen step.DiffGenerator, workspace string, result step.Result)
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
// runtime init, rehydration, directory prep, execution, and uploading.
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

	outDirErr := withTempDir(cfg.OutDirPattern, func(outDir string) error {
		if cfg.InDirPattern != "" {
			return withTempDir(cfg.InDirPattern, func(inDir string) error {
				if cfg.PopulateInDir != nil {
					if err := cfg.PopulateInDir(inDir); err != nil {
						return fmt.Errorf("populate in dir: %w", err)
					}
				}
				stepOutcome, err := r.runContainerJob(ctx, req, cfg, execCtx, workspace, startTime, outDir, inDir)
				if err != nil {
					return err
				}
				outcome = stepOutcome
				return nil
			})
		}
		stepOutcome, err := r.runContainerJob(ctx, req, cfg, execCtx, workspace, startTime, outDir, "")
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
	workspace string,
	startTime time.Time,
	outDir, inDir string,
) (standardJobOutcome, error) {
	outcome := standardJobOutcome{}
	shareDir, err := ensureRunRepoShareDir(req.RunID, req.RepoID)
	if err != nil {
		return outcome, err
	}
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
			if runErr == nil && cfg.ValidateOutputs != nil {
				if validateErr := cfg.ValidateOutputs(outDir, workspace); validateErr != nil {
					runErr = fmt.Errorf("validate job outputs: %w", validateErr)
				}
			}
			runErr = r.finalizeStandardJobOutputs(req, cfg, outDir, workspace, runErr, step.Result{})
			repoSHAOut := ""
			if runErr == nil {
				var repoSHAErr error
				repoSHAOut, repoSHAErr = r.computeRepoSHAOut(ctx, req, workspace, "")
				if repoSHAErr != nil {
					runErr = repoSHAErr
					slog.Error("failed to compute repo_sha_out", "run_id", req.RunID, "job_id", req.JobID, "error", repoSHAErr)
				}
			}
			statsBuilder := types.NewRunStatsBuilder().
				ExitCode(0).
				DurationMs(duration.Milliseconds())
			if runErr != nil {
				statsBuilder.Error(runErr.Error())
			}
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
		return outcome, fmt.Errorf("compute pre-execution workspace tree: %w", treeErr)
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
			ShareDir:   shareDir,
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
		cfg.UploadDiff(ctx, req.RunID, req.JobID, req.JobName, execCtx.diffGenerator, workspace, result)
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

	repoSHAOut := ""
	if runErr == nil && result.ExitCode == 0 {
		var repoSHAErr error
		repoSHAOut, repoSHAErr = r.computeRepoSHAOut(ctx, req, workspace, preWorkspaceTree)
		if repoSHAErr != nil {
			runErr = repoSHAErr
			slog.Error("failed to compute repo_sha_out", "run_id", req.RunID, "job_id", req.JobID, "error", repoSHAErr)
		}
	}

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
	if runErr != nil {
		statsBuilder.Error(runErr.Error())
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
