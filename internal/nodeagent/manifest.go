package nodeagent

import (
	"errors"
	"fmt"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildManifestFromRequest converts a StartRunRequest into a StepManifest.
// The function accepts typed RunOptions for type-safe access to spec configuration.
//
// For multi-step runs (when typedOpts.Steps is non-empty), stepIndex selects which
// mod step to build a manifest for. For single-step runs, stepIndex is ignored and
// Execution options are used. This enables step-by-step execution where each step
// runs gate+mod with its own image/command/env configuration.
//
// ## Stack Parameter
//
// The stack parameter is used to resolve stack-specific images when the image field
// is a map (e.g., different images for java-maven vs java-gradle). Stack values
// typically come from Build Gate detection:
//   - contracts.ModStackJavaMaven: Maven project detected (pom.xml present)
//   - contracts.ModStackJavaGradle: Gradle project detected (build.gradle present)
//   - contracts.ModStackJava: Generic Java (no specific build tool)
//   - contracts.ModStackUnknown: No recognized stack markers or stack not yet detected
//
// Callers should pass the explicit stack when available. For gate jobs (where stack
// is not yet detected), pass contracts.ModStackUnknown explicitly.
func buildManifestFromRequest(req StartRunRequest, typedOpts RunOptions, stepIndex int, stack contracts.ModStack) (contracts.StepManifest, error) {
	if req.RunID.IsZero() {
		return contracts.StepManifest{}, errors.New("run_id required")
	}
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}
	if strings.TrimSpace(req.RepoURL.String()) == "" {
		return contracts.StepManifest{}, errors.New("repo_url required")
	}

	// Default image; command is only injected for the default image to
	// preserve image-provided CMD/ENTRYPOINT for custom mods containers.
	const defaultImage = "ubuntu:latest"
	image := defaultImage
	command := []string(nil)
	env := make(map[string]string, len(req.Env))
	retain := false

	// Select step-specific configuration: use Steps[stepIndex] for multi-step runs,
	// or Execution for single-step runs. Multi-step runs take precedence when Steps
	// is non-empty.
	if len(typedOpts.Steps) > 0 {
		// Multi-step run: extract image, command, env, and retention from Steps[stepIndex].
		if stepIndex < 0 || stepIndex >= len(typedOpts.Steps) {
			return contracts.StepManifest{}, fmt.Errorf("step index %d out of range (0-%d)", stepIndex, len(typedOpts.Steps)-1)
		}
		stepMod := typedOpts.Steps[stepIndex]

		// Resolve image using stack-aware selection. If the image spec is empty,
		// fall back to the default image. Resolution errors fail the manifest build.
		if !stepMod.Image.IsEmpty() {
			resolved, err := stepMod.Image.ResolveImage(stack)
			if err != nil {
				return contracts.StepManifest{}, fmt.Errorf("step[%d] image resolution: %w", stepIndex, err)
			}
			image = strings.TrimSpace(resolved)
		}
		command = stepMod.Command.ToSlice()

		// Merge base env (from spec top-level) with step-specific env (step wins on conflict).
		for k, v := range req.Env {
			env[k] = v
		}
		for k, v := range stepMod.Env {
			env[k] = v
		}

		retain = stepMod.RetainContainer
	} else {
		// Single-step run: use Execution options.
		// Resolve image using stack-aware selection. If the image spec is empty,
		// fall back to the default image. Resolution errors fail the manifest build.
		if !typedOpts.Execution.Image.IsEmpty() {
			resolved, err := typedOpts.Execution.Image.ResolveImage(stack)
			if err != nil {
				return contracts.StepManifest{}, fmt.Errorf("image resolution: %w", err)
			}
			image = strings.TrimSpace(resolved)
		}
		command = typedOpts.Execution.Command.ToSlice()

		// Copy base env.
		for k, v := range req.Env {
			env[k] = v
		}

		retain = typedOpts.Execution.RetainContainer
	}

	// If no explicit command was provided, inject a harmless placeholder only
	// when running the default ubuntu image. For custom images (e.g., mods
	// containers with ENTRYPOINT+CMD), leaving command empty allows Docker to
	// use the image's own defaults.
	if len(command) == 0 && image == defaultImage {
		command = []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
	}

	// Determine the ref to clone.
	targetRef := strings.TrimSpace(req.TargetRef.String())
	if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
		targetRef = strings.TrimSpace(req.BaseRef.String())
	}

	// Build the repo materialization.
	repo := contracts.RepoMaterialization{
		URL:       req.RepoURL,
		BaseRef:   req.BaseRef,
		TargetRef: types.GitRef(targetRef),
		Commit:    req.CommitSHA,
	}

	// Build manifest options from typed accessors. Only select keys are propagated
	// to manifest.Options to keep scope tight and avoid accidentally logging/transmitting
	// unrelated values.
	//
	// Allowed keys:
	//   - gitlab_pat, gitlab_domain, mr_on_success, mr_on_fail (MR wiring)
	//   - job_id (server-provided job identifier for uploads)
	//   - artifact_name (optional bundle name override)
	mergedOpts := make(map[string]any)
	if pat := strings.TrimSpace(typedOpts.MRWiring.GitLabPAT); pat != "" {
		mergedOpts["gitlab_pat"] = pat
	}
	if domain := strings.TrimSpace(typedOpts.MRWiring.GitLabDomain); domain != "" {
		mergedOpts["gitlab_domain"] = domain
	}
	// Include MR flags in options only when explicitly set (not just default false).
	// Use typed MRFlagsPresent to distinguish between "not set" and "set to false".
	if typedOpts.MRFlagsPresent.MROnSuccessSet {
		mergedOpts["mr_on_success"] = typedOpts.MRWiring.MROnSuccess
	}
	if typedOpts.MRFlagsPresent.MROnFailSet {
		mergedOpts["mr_on_fail"] = typedOpts.MRWiring.MROnFail
	}
	if !typedOpts.ServerMetadata.JobID.IsZero() {
		mergedOpts["job_id"] = typedOpts.ServerMetadata.JobID.String()
	}
	if aname := strings.TrimSpace(typedOpts.Artifacts.Name); aname != "" {
		mergedOpts["artifact_name"] = aname
	}

	// Derive the gate ref from repo metadata. Precedence:
	//   1. CommitSHA — pinned commit ensures deterministic gate validation.
	//   2. TargetRef — branch/tag for feature branches and PR flows.
	//   3. BaseRef — fallback for baseline validations.
	// This mirrors the ref precedence documented in StepGateSpec.
	gateRef := ""
	if sha := strings.TrimSpace(req.CommitSHA.String()); sha != "" {
		gateRef = sha
	} else if tr := strings.TrimSpace(req.TargetRef.String()); tr != "" {
		gateRef = tr
	} else if br := strings.TrimSpace(req.BaseRef.String()); br != "" {
		gateRef = br
	}

	// Step manifest IDs must be unique per job. Use JobID to avoid collisions
	// across jobs within the same run (pre_gate/mod/post_gate/heal/re_gate).
	stepID := types.StepID(req.JobID)

	manifest := contracts.StepManifest{
		ID:         stepID,
		Name:       fmt.Sprintf("Run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		// Gate spec includes repo metadata for HTTP-based gate execution (C1).
		// RepoURL and Ref enable remote gate workers to clone and validate
		// without direct workspace access.
		Gate: &contracts.StepGateSpec{
			Enabled:        true,
			Env:            map[string]string{},
			ImageOverrides: nil,
			RepoURL:        types.RepoURL(strings.TrimSpace(req.RepoURL.String())),
			Ref:            types.GitRef(strings.TrimSpace(gateRef)),
		},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
				Hydration: &contracts.StepInputHydration{
					Repo: &repo,
				},
			},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: retain,
			TTL:             types.Duration(time.Hour),
		},
		Options: mergedOpts,
	}

	// Override Gate from typed build gate options.
	manifest.Gate.Enabled = typedOpts.BuildGate.Enabled
	manifest.Gate.ImageOverrides = typedOpts.BuildGate.Images

	// Note: Stack Gate expectations are threaded into gate manifests via the
	// gate-specific path in executeGateJob, which sets typedOpts.StackGate
	// based on gate type (pre_gate uses inbound, post_gate/re_gate use outbound).
	// This allows correct differentiation between inbound and outbound phases.

	return manifest, nil
}

// buildGateManifestFromRequest builds a StepManifest for gate jobs (pre_gate,
// post_gate, re_gate) using only the gate configuration and repo metadata.
//
// Gate jobs must not depend on stack-aware mod images from Execution.Image or
// mods[].image. Stack detection happens inside the Build Gate itself, and the
// resulting stack is persisted for later Mods/healing jobs. To avoid resolving
// stack-specific image maps with an "unknown" stack (which would fail when no
// default key is present), this helper:
//
//   - Clears Steps so multi-step mods[] configuration is ignored.
//   - Clears Execution.Image and Execution.Command so the default ubuntu image
//     and placeholder command are used.
//
// This keeps gate manifest image resolution independent of mods[] image maps
// while preserving repo metadata and MR wiring options.
//
// Gate jobs explicitly use contracts.ModStackUnknown since stack detection has
// not yet occurred. The Build Gate will determine the actual stack (e.g.,
// java-maven, java-gradle) which is then persisted for subsequent mod/healing jobs.
func buildGateManifestFromRequest(req StartRunRequest, typedOpts RunOptions) (contracts.StepManifest, error) {
	// Shallow copy to avoid mutating caller's RunOptions.
	sanitized := typedOpts

	// Ignore per-step mods configuration for gate jobs; gate execution does not
	// run the Mods containers and should not depend on mods[].image.
	sanitized.Steps = nil

	// Ignore stack-aware Execution.Image and custom commands for gate jobs.
	// Gate containers are selected by the Gate executor based on workspace
	// stack detection, not by the Mods execution image.
	sanitized.Execution.Image = contracts.ModImage{}
	sanitized.Execution.Command = ExecutionCommand{}

	// Delegate to the standard manifest builder with sanitized options.
	// Pass ModStackUnknown explicitly since gate jobs run before stack detection.
	manifest, err := buildManifestFromRequest(req, sanitized, 0, contracts.ModStackUnknown)
	if err != nil {
		return manifest, err
	}

	// Thread Stack Gate expectation passed via typedOpts.StackGate.
	// This is set by executeGateJob based on gate type (pre_gate uses inbound,
	// post_gate/re_gate use outbound expectations).
	if typedOpts.StackGate != nil {
		manifest.Gate.StackGate = typedOpts.StackGate
	}

	return manifest, nil
}

// isCodexHealingImage returns true if the image name indicates a Codex-based
// healing mod container. This enables automatic session resume for Codex healers.
//
// The function checks for common Codex image patterns:
//   - "mods-codex" (exact or as prefix/suffix)
//   - Image names containing "codex" substring
//
// This heuristic allows session propagation without requiring explicit configuration.
func isCodexHealingImage(image string) bool {
	// Normalize image name by extracting the repository/image portion
	// without registry prefix or tag suffix for matching.
	// Examples:
	//   - "mods-codex" → match
	//   - "registry.io/mods-codex:v1" → match
	//   - "my-codex-healer" → match
	//   - "standard-healer" → no match
	lower := strings.ToLower(image)
	return strings.Contains(lower, "codex")
}

// buildHealingManifest constructs a StepManifest from a typed HealingMod.
// The healing mod runs with /workspace (RW), /out (RW), and /in (RO) mounts.
// Using typed HealingMod clarifies which fields are understood by the agent.
//
// Repo metadata is injected into the manifest environment to allow healing containers
// to derive the same Git baseline used by the Mods run:
//   - PLOY_REPO_URL: repository URL for cloning/verification
//   - PLOY_BASE_REF: base Git reference (branch or tag)
//   - PLOY_TARGET_REF: target Git reference for the run
//   - PLOY_COMMIT_SHA: pinned commit SHA when available
//
// When codexSession is non-empty and the healing mod image is a Codex-based healer,
// the function injects CODEX_RESUME=1 to signal that the healer should resume from
// an existing session. The codex-session.txt file must be placed in /in by the
// caller (executeWithHealing) for the healer to read.
//
// These env vars enable healing mods to call the Build Gate HTTP API with the same
// repo+ref baseline used by the initial Build Gate check.
//
// ## Stack Parameter
//
// HealingMod.Image supports both canonical forms (universal string and stack map).
// The stack parameter is used to resolve stack-specific images when the image field
// is a map. Stack values typically come from Build Gate detection:
//   - contracts.ModStackJavaMaven: Maven project detected (pom.xml present)
//   - contracts.ModStackJavaGradle: Gradle project detected (build.gradle present)
//   - contracts.ModStackJava: Generic Java (no specific build tool)
//   - contracts.ModStackUnknown: No recognized stack markers or stack not yet detected
//
// Callers should pass the explicit stack when available. For healing during inline
// gate-heal-regate loops (where stack may not be persisted yet), pass
// contracts.ModStackUnknown explicitly.
func buildHealingManifest(req StartRunRequest, mod HealingMod, index int, codexSession string, stack contracts.ModStack) (contracts.StepManifest, error) {
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}
	// Validate and resolve the image field. ModImage can be:
	//   - Universal string: returned directly
	//   - Stack map: resolved using the provided stack with default fallback
	if mod.Image.IsEmpty() {
		return contracts.StepManifest{}, fmt.Errorf("healing mod[%d]: image required", index)
	}
	image, err := mod.Image.ResolveImage(stack)
	if err != nil {
		return contracts.StepManifest{}, fmt.Errorf("healing mod[%d] image resolution: %w", index, err)
	}
	image = strings.TrimSpace(image)
	// Handle whitespace-only universal images (resolved but empty after trim).
	if image == "" {
		return contracts.StepManifest{}, fmt.Errorf("healing mod[%d]: image required", index)
	}

	// Use typed command accessor to avoid polymorphic handling.
	command := mod.Command.ToSlice()

	// Use typed env map (already map[string]string). Clone to avoid mutating caller's map.
	env := make(map[string]string, len(mod.Env)+4) // +4 for repo metadata vars
	for k, v := range mod.Env {
		env[k] = v
	}

	// Inject repo metadata from StartRunRequest so healing containers can derive
	// the same Git baseline used by the Mods run. Only set when values are present
	// to avoid injecting empty strings into the container environment.
	if repoURL := strings.TrimSpace(req.RepoURL.String()); repoURL != "" {
		env["PLOY_REPO_URL"] = repoURL
	}
	if baseRef := strings.TrimSpace(req.BaseRef.String()); baseRef != "" {
		env["PLOY_BASE_REF"] = baseRef
	}
	if targetRef := strings.TrimSpace(req.TargetRef.String()); targetRef != "" {
		env["PLOY_TARGET_REF"] = targetRef
	}
	if commitSHA := strings.TrimSpace(req.CommitSHA.String()); commitSHA != "" {
		env["PLOY_COMMIT_SHA"] = commitSHA
	}

	// Inject CODEX_RESUME=1 when a Codex session is available and the healing mod
	// image is a Codex-based healer. This signals the healer to resume from an
	// existing conversation instead of starting fresh. The session file itself
	// (codex-session.txt) is placed in /in by the caller (executeWithHealing).
	// Non-Codex healing mods remain unaffected by this injection.
	if codexSession != "" && isCodexHealingImage(image) {
		env["CODEX_RESUME"] = "1"
	}

	// Use typed retain flag.
	retain := mod.RetainContainer

	// For healing, reuse the existing workspace; do not re-hydrate the repo.

	// Create the healing manifest.
	// Healing mods do not run the build gate themselves; they only modify the workspace.
	//
	// Step manifest IDs must be unique per job. Healing jobs may execute multiple
	// healing mods; include the mod index to keep IDs distinct within the job.
	healingStepID := types.StepID(fmt.Sprintf("%s-heal-%d", req.JobID, index))

	manifest := contracts.StepManifest{
		ID:         healingStepID,
		Name:       fmt.Sprintf("Healing mod %d for run %s", index, req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		Gate:       &contracts.StepGateSpec{Enabled: false}, // Healing mods do not run gates.
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
				// Do not re-hydrate the repository for healing mods; they
				// operate on the existing workspace produced by the initial
				// preparation to avoid clone collisions.
			},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: retain,
			TTL:             types.Duration(time.Hour),
		},
		Options: map[string]any{
			// Allow in-container verification tools that need Docker by mounting the host Docker socket.
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}

// buildRouterManifest constructs a StepManifest from a typed RouterConfig.
// The router container runs with /in (RO) containing build-gate.log and /out (RW)
// for writing codex-last.txt with the bug_summary JSON one-liner.
func buildRouterManifest(req StartRunRequest, router RouterConfig, stack contracts.ModStack) (contracts.StepManifest, error) {
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}
	if router.Image.IsEmpty() {
		return contracts.StepManifest{}, fmt.Errorf("router: image required")
	}
	image, err := router.Image.ResolveImage(stack)
	if err != nil {
		return contracts.StepManifest{}, fmt.Errorf("router image resolution: %w", err)
	}
	image = strings.TrimSpace(image)
	if image == "" {
		return contracts.StepManifest{}, fmt.Errorf("router: image required")
	}

	command := router.Command.ToSlice()

	env := make(map[string]string, len(router.Env)+4)
	for k, v := range router.Env {
		env[k] = v
	}

	// Inject repo metadata.
	if repoURL := strings.TrimSpace(req.RepoURL.String()); repoURL != "" {
		env["PLOY_REPO_URL"] = repoURL
	}
	if baseRef := strings.TrimSpace(req.BaseRef.String()); baseRef != "" {
		env["PLOY_BASE_REF"] = baseRef
	}
	if targetRef := strings.TrimSpace(req.TargetRef.String()); targetRef != "" {
		env["PLOY_TARGET_REF"] = targetRef
	}
	if commitSHA := strings.TrimSpace(req.CommitSHA.String()); commitSHA != "" {
		env["PLOY_COMMIT_SHA"] = commitSHA
	}

	routerStepID := types.StepID(fmt.Sprintf("%s-router", req.JobID))

	manifest := contracts.StepManifest{
		ID:         routerStepID,
		Name:       fmt.Sprintf("Router for run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		Gate:       &contracts.StepGateSpec{Enabled: false},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadOnly,
			},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: router.RetainContainer,
			TTL:             types.Duration(time.Hour),
		},
		Options: map[string]any{
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}

// validateAndDeriveStackGateChaining validates and derives Stack Gate chaining for multi-step runs.
// For steps after the first, it:
//   - Derives inbound expectations from the previous step's outbound when omitted.
//   - Rejects mismatched explicit inbound that differs from previous outbound.
//
// This function modifies the input steps slice in place when deriving inbound expectations.
// Returns an error if explicit inbound mismatches previous outbound.
func validateAndDeriveStackGateChaining(steps []StepMod) error {
	if len(steps) <= 1 {
		// Single-step runs have no chaining to validate.
		return nil
	}

	for i := 1; i < len(steps); i++ {
		prev := steps[i-1]
		curr := &steps[i]

		// Skip if previous step has no outbound expectations.
		if prev.Stack == nil || prev.Stack.Outbound == nil || !prev.Stack.Outbound.Enabled {
			continue
		}
		prevOutbound := prev.Stack.Outbound

		// If current step has no Stack spec, create one with derived inbound.
		if curr.Stack == nil {
			curr.Stack = &contracts.StackGateSpec{
				Inbound: &contracts.StackGatePhaseSpec{
					Enabled: prevOutbound.Enabled,
					Expect:  prevOutbound.Expect,
				},
			}
			continue
		}

		// If current step has no inbound, derive from previous outbound.
		if curr.Stack.Inbound == nil {
			curr.Stack.Inbound = &contracts.StackGatePhaseSpec{
				Enabled: prevOutbound.Enabled,
				Expect:  prevOutbound.Expect,
			}
			continue
		}

		// If current step has explicit inbound, validate it matches previous outbound.
		currInbound := curr.Stack.Inbound
		if currInbound.Enabled && prevOutbound.Enabled {
			// Both are enabled; validate expectations match.
			if currInbound.Expect != nil && prevOutbound.Expect != nil {
				if !currInbound.Expect.Equal(*prevOutbound.Expect) {
					return fmt.Errorf(
						"steps[%d].stack.inbound: mismatch with steps[%d].stack.outbound "+
							"(inbound: language=%q tool=%q release=%q, outbound: language=%q tool=%q release=%q)",
						i, i-1,
						currInbound.Expect.Language, currInbound.Expect.Tool, currInbound.Expect.Release,
						prevOutbound.Expect.Language, prevOutbound.Expect.Tool, prevOutbound.Expect.Release,
					)
				}
			}
		}
	}

	return nil
}

// stackGatePhaseSpecToStepGate converts a StackGatePhaseSpec to StepGateStackSpec.
// Returns nil if the input is nil or disabled.
func stackGatePhaseSpecToStepGate(phase *contracts.StackGatePhaseSpec, _ []contracts.BuildGateImageRule) *contracts.StepGateStackSpec {
	if phase == nil || !phase.Enabled {
		return nil
	}
	return &contracts.StepGateStackSpec{
		Enabled: phase.Enabled,
		Expect:  phase.Expect,
	}
}
