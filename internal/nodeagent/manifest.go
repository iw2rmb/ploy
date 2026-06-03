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
func resolveImage(
	img contracts.JobImage,
	stack contracts.MigStack,
	stackExp *contracts.StackExpectation,
	label string,
) (string, error) {
	if img.IsEmpty() {
		return "", fmt.Errorf("%s: image required", label)
	}
	resolved, err := img.ResolveImage(stack)
	if err != nil {
		return "", fmt.Errorf("%s image resolution: %w", label, err)
	}
	expanded, err := contracts.ExpandImageTemplate(resolved, stackExp)
	if err != nil {
		return "", fmt.Errorf("%s image template expansion: %w", label, err)
	}
	resolved = strings.TrimSpace(expanded)
	if resolved == "" {
		return "", fmt.Errorf("%s: image required", label)
	}
	return resolved, nil
}

// injectRepoMetadataEnv adds PLOY_REPO_URL, PLOY_BASE_REF, and PLOY_COMMIT_SHA
// to env from the request. Only non-empty values are set.
func injectRepoMetadataEnv(env map[string]string, req StartRunRequest) {
	if v := strings.TrimSpace(req.RepoURL.String()); v != "" {
		env["PLOY_REPO_URL"] = v
	}
	if v := strings.TrimSpace(req.BaseRef.String()); v != "" {
		env["PLOY_BASE_REF"] = v
	}
	if v := strings.TrimSpace(req.CommitSHA.String()); v != "" {
		env["PLOY_COMMIT_SHA"] = v
	}
}

func injectNodeOwnedMigEnv(env map[string]string, req StartRunRequest) {
	if v := strings.TrimSpace(req.ServerURL); v != "" {
		env["PLOY_SERVER_URL"] = v
	}
}

// --- Main manifest builders ---

// buildMigManifest converts a StartRunRequest into a StepManifest.
// For multi-step runs, stepIndex selects the step; for single-step runs it is ignored.
// The stack parameter resolves stack-specific images (pass MigStackUnknown if unknown).
func buildMigManifest(req StartRunRequest, typedOpts RunOptions, stepIndex int, stack contracts.MigStack) (contracts.StepManifest, error) {
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
	stackExp := stackExpectationForRequest(req, stack)

	var hydraIn, hydraOut, hydraHome []string
	if len(typedOpts.Steps) > 0 {
		// Multi-step run.
		if stepIndex < 0 || stepIndex >= len(typedOpts.Steps) {
			return contracts.StepManifest{}, fmt.Errorf("step index %d out of range (0-%d)", stepIndex, len(typedOpts.Steps)-1)
		}
		stepMig := typedOpts.Steps[stepIndex]

		if !stepMig.Image.IsEmpty() {
			resolved, err := resolveImage(stepMig.Image, stack, stackExp, fmt.Sprintf("step[%d]", stepIndex))
			if err != nil {
				return contracts.StepManifest{}, err
			}
			image = resolved
		}
		command = stepMig.Command.ToSlice()
		hydraIn = stepMig.In
		hydraOut = stepMig.Out
		hydraHome = stepMig.Home

		for k, v := range req.Env {
			env[k] = v
		}
		for k, v := range stepMig.Env {
			env[k] = v
		}
	} else {
		// Single-step run.
		if !typedOpts.Execution.Image.IsEmpty() {
			resolved, err := resolveImage(typedOpts.Execution.Image, stack, stackExp, "execution")
			if err != nil {
				return contracts.StepManifest{}, err
			}
			image = resolved
		}
		command = typedOpts.Execution.Command.ToSlice()
		hydraIn = typedOpts.Execution.In
		hydraOut = typedOpts.Execution.Out
		hydraHome = typedOpts.Execution.Home

		for k, v := range req.Env {
			env[k] = v
		}
	}

	injectStackTupleEnv(env, stackExp)
	injectNodeOwnedMigEnv(env, req)

	// Inject placeholder command only for default ubuntu image.
	if len(command) == 0 && image == defaultImage {
		command = []string{"/bin/sh", "-c", "echo 'Build gate placeholder'"}
	}

	repo := contracts.RepoMaterialization{
		URL:     req.RepoURL,
		BaseRef: req.BaseRef,
		Commit:  req.CommitSHA,
	}

	// Build manifest options from typed accessors.
	mergedOpts := make(map[string]any)
	if !typedOpts.ServerMetadata.JobID.IsZero() {
		mergedOpts["job_id"] = typedOpts.ServerMetadata.JobID.String()
	}

	// Derive gate ref: CommitSHA > BaseRef.
	gateRef := ""
	if sha := strings.TrimSpace(req.CommitSHA.String()); sha != "" {
		gateRef = sha
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
		Envs:       env,
		In:         hydraIn,
		Out:        hydraOut,
		Home:       hydraHome,
		BundleMap:  typedOpts.BundleMap,
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

	manifest.Gate.Enabled = !typedOpts.BuildGate.Disabled
	manifest.Gate.ImageOverrides = typedOpts.BuildGate.Images

	return manifest, nil
}

// buildGateManifest builds a StepManifest for gate jobs (pre_gate,
// post_gate). Gate jobs use the default image since stack detection
// happens inside the Build Gate itself.
func buildGateManifest(req StartRunRequest, typedOpts RunOptions) (contracts.StepManifest, error) {
	sanitized := typedOpts
	sanitized.Steps = nil
	sanitized.Execution.Image = contracts.JobImage{}
	sanitized.Execution.Command = contracts.CommandSpec{}

	manifest, err := buildMigManifest(req, sanitized, 0, contracts.MigStackUnknown)
	if err != nil {
		return manifest, err
	}

	if typedOpts.StackGate != nil {
		manifest.Gate.StackGate = typedOpts.StackGate
	}

	return manifest, nil
}

// --- Stack Gate chaining ---

// validateAndDeriveStackGateChaining validates and derives Stack Gate chaining for multi-step runs.
// For steps after the first, it derives inbound expectations from the previous step's outbound
// when omitted, and rejects mismatched explicit inbound. Updates steps in place.
func validateAndDeriveStackGateChaining(steps []StepOptions) error {
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
