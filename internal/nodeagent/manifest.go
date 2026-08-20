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

func injectNodeOwnedRepoEnv(env map[string]string, req StartRunRequest) {
	env["PLOY_REPO_URL"] = strings.TrimSpace(req.RepoURL.String())
	env["PLOY_REPO_REF"] = strings.TrimSpace(req.BaseRef.String())
}

func injectNodeOwnedMigEnv(env map[string]string, req StartRunRequest) {
	if v := strings.TrimSpace(req.ServerURL); v != "" {
		env["PLOY_SERVER_URL"] = v
	}
}

// --- Main manifest builders ---

// buildMigManifest selects one canonical migration step and assembles its manifest.
func buildMigManifest(req StartRunRequest, stepIndex int, stack contracts.MigStack) (contracts.StepManifest, error) {
	var migStep *contracts.MigStep
	if req.MigSpec != nil {
		if stepIndex < 0 || stepIndex >= len(req.MigSpec.Steps) {
			return contracts.StepManifest{}, fmt.Errorf("step index %d out of range (0-%d)", stepIndex, len(req.MigSpec.Steps)-1)
		}
		migStep = &req.MigSpec.Steps[stepIndex]
	}
	return buildManifest(req, migStep, stack, nil, true)
}

// buildGateManifest assembles a gate manifest without using a migration
// image or command.
func buildGateManifest(req StartRunRequest, stackGate *contracts.StackGatePhaseSpec) (contracts.StepManifest, error) {
	var migStep *contracts.MigStep
	if req.MigSpec != nil && len(req.MigSpec.Steps) == 1 {
		migStep = &req.MigSpec.Steps[0]
	}
	return buildManifest(req, migStep, contracts.MigStackUnknown, stackGate, false)
}

func buildManifest(
	req StartRunRequest,
	migStep *contracts.MigStep,
	stack contracts.MigStack,
	stackGate *contracts.StackGatePhaseSpec,
	useMigrationCommand bool,
) (contracts.StepManifest, error) {
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
	env := contracts.CopyEnv(req.Env)
	if req.MigSpec != nil {
		env = contracts.MergeEnv(env, req.MigSpec.Envs)
	}
	stackExp := stackExpectationForRequest(req, stack)

	var hydraIn, hydraOut, hydraHome, hydraTmp []string
	var stepOptions contracts.MigStepOptions
	if migStep != nil {
		if useMigrationCommand && !migStep.Image.IsEmpty() {
			resolved, err := resolveImage(migStep.Image, stack, stackExp, "step")
			if err != nil {
				return contracts.StepManifest{}, err
			}
			image = resolved
		}
		if useMigrationCommand {
			command = migStep.Command.ToSlice()
		}
		hydraIn = migStep.In
		hydraOut = migStep.Out
		hydraHome = migStep.Home
		hydraTmp = migStep.Tmp
		stepOptions = migStep.Options
		env = contracts.MergeEnv(env, migStep.Envs)
	}
	if env == nil {
		env = make(map[string]string)
	}

	injectStackTupleEnv(env, stackExp)
	injectNodeOwnedRepoEnv(env, req)
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

	mergedOpts := make(map[string]any)
	if stepOptions.MountDockerSocket {
		mergedOpts["mount_docker_socket"] = true
	}
	if req.MigSpec != nil && !req.MigSpec.JobID.IsZero() {
		mergedOpts["job_id"] = req.MigSpec.JobID.String()
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
		Tmp:        hydraTmp,
		BundleMap:  nil,
		Gate: &contracts.StepGateSpec{
			Enabled:   true,
			Env:       gateEnv,
			StackGate: stackGate,
			RepoID:    req.RepoID,
			RepoURL:   types.RepoURL(strings.TrimSpace(req.RepoURL.String())),
			Ref:       types.GitRef(strings.TrimSpace(gateRef)),
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

	if req.MigSpec != nil {
		manifest.BundleMap = req.MigSpec.BundleMap
		if req.MigSpec.BuildGate != nil {
			manifest.Gate.Enabled = !req.MigSpec.BuildGate.Disabled
			manifest.Gate.ImageOverrides = req.MigSpec.BuildGate.Images
		}
	}

	return manifest, nil
}

// --- Stack Gate chaining ---

// validateAndDeriveStackGateChaining validates and derives Stack Gate chaining for multi-step runs.
// For steps after the first, it derives inbound expectations from the previous step's outbound
// when omitted, and rejects mismatched explicit inbound. Updates steps in place.
func validateAndDeriveStackGateChaining(steps []contracts.MigStep) error {
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
