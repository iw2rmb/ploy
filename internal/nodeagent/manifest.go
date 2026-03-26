package nodeagent

import (
	"errors"
	"fmt"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// --- Shared manifest helpers ---

// resolveImage validates and resolves a JobImage to a concrete image string using the
// given stack. Returns an error if the image is empty after resolution.
func resolveImage(img contracts.JobImage, stack contracts.ModStack, label string) (string, error) {
	if img.IsEmpty() {
		return "", fmt.Errorf("%s: image required", label)
	}
	resolved, err := img.ResolveImage(stack)
	if err != nil {
		return "", fmt.Errorf("%s image resolution: %w", label, err)
	}
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return "", fmt.Errorf("%s: image required", label)
	}
	return resolved, nil
}

// injectRepoMetadataEnv adds PLOY_REPO_URL, PLOY_BASE_REF, PLOY_TARGET_REF, and
// PLOY_COMMIT_SHA to env from the request. Only non-empty values are set.
func injectRepoMetadataEnv(env map[string]string, req StartRunRequest) {
	if v := strings.TrimSpace(req.RepoURL.String()); v != "" {
		env["PLOY_REPO_URL"] = v
	}
	if v := strings.TrimSpace(req.BaseRef.String()); v != "" {
		env["PLOY_BASE_REF"] = v
	}
	if v := strings.TrimSpace(req.TargetRef.String()); v != "" {
		env["PLOY_TARGET_REF"] = v
	}
	if v := strings.TrimSpace(req.CommitSHA.String()); v != "" {
		env["PLOY_COMMIT_SHA"] = v
	}
}

// --- Main manifest builders ---

// buildManifestFromRequest converts a StartRunRequest into a StepManifest.
// For multi-step runs, stepIndex selects the step; for single-step runs it is ignored.
// The stack parameter resolves stack-specific images (pass ModStackUnknown if unknown).
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

	const defaultImage = "ubuntu:latest"
	image := defaultImage
	command := []string(nil)
	env := make(map[string]string, len(req.Env))

	var tmpBundle *contracts.TmpBundleRef
	if len(typedOpts.Steps) > 0 {
		// Multi-step run.
		if stepIndex < 0 || stepIndex >= len(typedOpts.Steps) {
			return contracts.StepManifest{}, fmt.Errorf("step index %d out of range (0-%d)", stepIndex, len(typedOpts.Steps)-1)
		}
		stepMod := typedOpts.Steps[stepIndex]

		if !stepMod.Image.IsEmpty() {
			resolved, err := stepMod.Image.ResolveImage(stack)
			if err != nil {
				return contracts.StepManifest{}, fmt.Errorf("step[%d] image resolution: %w", stepIndex, err)
			}
			image = strings.TrimSpace(resolved)
		}
		if stepMod.Amata != nil && strings.TrimSpace(stepMod.Amata.Spec) != "" {
			command = resolveAmataCommand(stepMod.Amata)
		} else {
			command = stepMod.Command.ToSlice()
		}
		tmpBundle = stepMod.TmpBundle

		for k, v := range req.Env {
			env[k] = v
		}
		for k, v := range stepMod.Env {
			env[k] = v
		}
	} else {
		// Single-step run.
		if !typedOpts.Execution.Image.IsEmpty() {
			resolved, err := typedOpts.Execution.Image.ResolveImage(stack)
			if err != nil {
				return contracts.StepManifest{}, fmt.Errorf("image resolution: %w", err)
			}
			image = strings.TrimSpace(resolved)
		}
		if typedOpts.Execution.Amata != nil && strings.TrimSpace(typedOpts.Execution.Amata.Spec) != "" {
			command = resolveAmataCommand(typedOpts.Execution.Amata)
		} else {
			command = typedOpts.Execution.Command.ToSlice()
		}
		tmpBundle = typedOpts.Execution.TmpBundle

		for k, v := range req.Env {
			env[k] = v
		}
	}

	// Inject placeholder command only for default ubuntu image.
	if len(command) == 0 && image == defaultImage {
		command = []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
	}

	targetRef := strings.TrimSpace(req.TargetRef.String())
	if targetRef == "" && strings.TrimSpace(req.BaseRef.String()) != "" {
		targetRef = strings.TrimSpace(req.BaseRef.String())
	}

	repo := contracts.RepoMaterialization{
		URL:       req.RepoURL,
		BaseRef:   req.BaseRef,
		TargetRef: types.GitRef(targetRef),
		Commit:    req.CommitSHA,
	}

	// Build manifest options from typed accessors.
	mergedOpts := make(map[string]any)
	if pat := strings.TrimSpace(typedOpts.MRWiring.GitLabPAT); pat != "" {
		mergedOpts["gitlab_pat"] = pat
	}
	if domain := strings.TrimSpace(typedOpts.MRWiring.GitLabDomain); domain != "" {
		mergedOpts["gitlab_domain"] = domain
	}
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

	// Derive gate ref: CommitSHA > TargetRef > BaseRef.
	gateRef := ""
	if sha := strings.TrimSpace(req.CommitSHA.String()); sha != "" {
		gateRef = sha
	} else if tr := strings.TrimSpace(req.TargetRef.String()); tr != "" {
		gateRef = tr
	} else if br := strings.TrimSpace(req.BaseRef.String()); br != "" {
		gateRef = br
	}

	stepID := types.StepID(req.JobID)

	// Gate env mirrors the job env.
	gateEnv := make(map[string]string, len(env))
	for k, v := range env {
		gateEnv[k] = v
	}

	manifest := contracts.StepManifest{
		ID:         stepID,
		Name:       fmt.Sprintf("Run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		TmpBundle:  tmpBundle,
		Gate: &contracts.StepGateSpec{
			Enabled:        true,
			Env:            gateEnv,
			ImageOverrides: nil,
			RepoID:         req.RepoID,
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
		Options: mergedOpts,
	}

	manifest.Gate.Enabled = typedOpts.BuildGate.Enabled
	manifest.Gate.ImageOverrides = typedOpts.BuildGate.Images

	return manifest, nil
}

// buildGateManifestFromRequest builds a StepManifest for gate jobs (pre_gate,
// post_gate, re_gate). Gate jobs use the default image since stack detection
// happens inside the Build Gate itself.
func buildGateManifestFromRequest(req StartRunRequest, typedOpts RunOptions) (contracts.StepManifest, error) {
	sanitized := typedOpts
	sanitized.Steps = nil
	sanitized.Execution.Image = contracts.JobImage{}
	sanitized.Execution.Command = contracts.CommandSpec{}

	manifest, err := buildManifestFromRequest(req, sanitized, 0, contracts.ModStackUnknown)
	if err != nil {
		return manifest, err
	}

	if typedOpts.StackGate != nil {
		manifest.Gate.StackGate = typedOpts.StackGate
	}

	return manifest, nil
}

// isCodexHealingImage returns true if the image name indicates a Codex-based healing container.
func isCodexHealingImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "codex")
}

// resolveAmataCommand builds the command slice for amata-mode execution.
// Produces ["amata", "run", "/in/amata.yaml"] followed by ordered "--set" "param=value" pairs.
// Order follows amata.Set slice order for deterministic CLI materialization.
func resolveAmataCommand(amata *contracts.AmataRunSpec) []string {
	cmd := []string{"amata", "run", "/in/amata.yaml"}
	for _, p := range amata.Set {
		cmd = append(cmd, "--set", p.Param+"="+p.Value)
	}
	return cmd
}

// buildHealingManifest constructs a StepManifest from a typed ModContainerSpec.
// The healing mig runs with /workspace (RW), /out (RW), and /in (RO) mounts.
// When codexSession is non-empty and the image is Codex-based, CODEX_RESUME=1 is injected.
func buildHealingManifest(req StartRunRequest, mig ModContainerSpec, index int, codexSession string, stack contracts.ModStack) (contracts.StepManifest, error) {
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}

	image, err := resolveImage(mig.Image, stack, fmt.Sprintf("healing mig[%d]", index))
	if err != nil {
		return contracts.StepManifest{}, err
	}

	var command []string
	if mig.Amata != nil && strings.TrimSpace(mig.Amata.Spec) != "" {
		command = resolveAmataCommand(mig.Amata)
	} else {
		command = mig.Command.ToSlice()
	}

	env := make(map[string]string, len(mig.Env)+4)
	for k, v := range mig.Env {
		env[k] = v
	}
	injectRepoMetadataEnv(env, req)

	if codexSession != "" && isCodexHealingImage(image) {
		env["CODEX_RESUME"] = "1"
	}

	healingStepID := types.StepID(fmt.Sprintf("%s-heal-%d", req.JobID, index))

	manifest := contracts.StepManifest{
		ID:         healingStepID,
		Name:       fmt.Sprintf("Healing mig %d for run %s", index, req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		TmpBundle:  mig.TmpBundle,
		Gate:       &contracts.StepGateSpec{Enabled: false},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadWrite,
			},
		},
		Options: map[string]any{
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}

// buildRouterManifest constructs a StepManifest for the router container that
// produces bug_summary before healing begins.
func buildRouterManifest(req StartRunRequest, router ModContainerSpec, stack contracts.ModStack, gatePhase types.JobType, loopKind string) (contracts.StepManifest, error) {
	if req.JobID.IsZero() {
		return contracts.StepManifest{}, errors.New("job_id required")
	}

	image, err := resolveImage(router.Image, stack, "router")
	if err != nil {
		return contracts.StepManifest{}, err
	}

	var command []string
	if router.Amata != nil && strings.TrimSpace(router.Amata.Spec) != "" {
		command = resolveAmataCommand(router.Amata)
	} else {
		command = router.Command.ToSlice()
	}

	env := make(map[string]string, len(router.Env)+4)
	for k, v := range router.Env {
		env[k] = v
	}
	injectRepoMetadataEnv(env, req)
	env["PLOY_GATE_PHASE"] = gatePhase.String()
	env["PLOY_LOOP_KIND"] = loopKind

	routerStepID := types.StepID(fmt.Sprintf("%s-router", req.JobID))

	manifest := contracts.StepManifest{
		ID:         routerStepID,
		Name:       fmt.Sprintf("Router for run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		TmpBundle:  router.TmpBundle,
		Gate:       &contracts.StepGateSpec{Enabled: false},
		Inputs: []contracts.StepInput{
			{
				Name:      "workspace",
				MountPath: "/workspace",
				Mode:      contracts.StepInputModeReadOnly,
			},
		},
		Options: map[string]any{
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}

// --- Stack Gate chaining ---

// validateAndDeriveStackGateChaining validates and derives Stack Gate chaining for multi-step runs.
// For steps after the first, it derives inbound expectations from the previous step's outbound
// when omitted, and rejects mismatched explicit inbound. Modifies steps in place.
func validateAndDeriveStackGateChaining(steps []StepMod) error {
	if len(steps) <= 1 {
		return nil
	}

	for i := 1; i < len(steps); i++ {
		prev := steps[i-1]
		curr := &steps[i]

		if prev.Stack == nil || prev.Stack.Outbound == nil || !prev.Stack.Outbound.Enabled {
			continue
		}
		prevOutbound := prev.Stack.Outbound

		if curr.Stack == nil {
			curr.Stack = &contracts.StackGateSpec{
				Inbound: &contracts.StackGatePhaseSpec{
					Enabled: prevOutbound.Enabled,
					Expect:  prevOutbound.Expect,
				},
			}
			continue
		}

		if curr.Stack.Inbound == nil {
			curr.Stack.Inbound = &contracts.StackGatePhaseSpec{
				Enabled: prevOutbound.Enabled,
				Expect:  prevOutbound.Expect,
			}
			continue
		}

		currInbound := curr.Stack.Inbound
		if currInbound.Enabled && prevOutbound.Enabled {
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
