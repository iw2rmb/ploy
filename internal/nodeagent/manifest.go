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
// The function accepts typed RunOptions to reduce map[string]any casts while
// preserving backward compatibility with the raw Options map for wire-level access.
func buildManifestFromRequest(req StartRunRequest, typedOpts RunOptions) (contracts.StepManifest, error) {
	if req.RunID.IsZero() {
		return contracts.StepManifest{}, errors.New("run_id required")
	}
	if strings.TrimSpace(req.RepoURL.String()) == "" {
		return contracts.StepManifest{}, errors.New("repo_url required")
	}

	// Default image; command is only injected for the default image to
	// preserve image-provided CMD/ENTRYPOINT for custom mods containers.
	const defaultImage = "ubuntu:latest"
	image := defaultImage
	if typedOpts.Execution.Image != "" {
		image = strings.TrimSpace(typedOpts.Execution.Image)
	}

	// Use typed command accessor to avoid map[string]any cast.
	command := typedOpts.Execution.Command.ToSlice()

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

	// Create a single read-write input that will be hydrated from the repository.
	// Defensive copy of env to avoid aliasing caller map.
	env := make(map[string]string, len(req.Env))
	for k, v := range req.Env {
		env[k] = v
	}

	// Use typed accessor for container retention.
	retain := typedOpts.Execution.RetainContainer

	// Build manifest options from typed accessors. Only select keys are propagated
	// to manifest.Options to keep scope tight and avoid accidentally logging/transmitting
	// unrelated values.
	//
	// Allowed keys:
	//   - gitlab_pat, gitlab_domain, mr_on_success, mr_on_fail (MR wiring)
	//   - stage_id (server-provided stage identifier for uploads)
	//   - artifact_name (optional bundle name override)
	mergedOpts := make(map[string]any)
	if pat := strings.TrimSpace(typedOpts.MRWiring.GitLabPAT); pat != "" {
		mergedOpts["gitlab_pat"] = pat
	}
	if domain := strings.TrimSpace(typedOpts.MRWiring.GitLabDomain); domain != "" {
		mergedOpts["gitlab_domain"] = domain
	}
	// Include MR flags in options only when explicitly set (not just default false).
	// We check the raw options map to distinguish between "not set" and "set to false".
	if _, hasMRSuccess := req.Options["mr_on_success"]; hasMRSuccess {
		mergedOpts["mr_on_success"] = typedOpts.MRWiring.MROnSuccess
	}
	if _, hasMRFail := req.Options["mr_on_fail"]; hasMRFail {
		mergedOpts["mr_on_fail"] = typedOpts.MRWiring.MROnFail
	}
	if sid := strings.TrimSpace(typedOpts.ServerMetadata.StageID); sid != "" {
		mergedOpts["stage_id"] = sid
	}
	if aname := strings.TrimSpace(typedOpts.Artifacts.Name); aname != "" {
		mergedOpts["artifact_name"] = aname
	}

	manifest := contracts.StepManifest{
		ID:         types.StepID(req.RunID),
		Name:       fmt.Sprintf("Run %s", req.RunID),
		Image:      image,
		Command:    command,
		WorkingDir: "/workspace",
		Env:        env,
		Gate:       &contracts.StepGateSpec{Enabled: true, Profile: "java-auto", Env: map[string]string{}},
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
	if profile := strings.TrimSpace(typedOpts.BuildGate.Profile); profile != "" {
		manifest.Gate.Profile = profile
	}

	return manifest, nil
}

// buildHealingManifest constructs a StepManifest from a typed HealingMod.
// The healing mod runs with /workspace (RW), /out (RW), and /in (RO) mounts.
// Using typed HealingMod clarifies which fields are understood by the agent.
func buildHealingManifest(req StartRunRequest, mod HealingMod, index int) (contracts.StepManifest, error) {
	// Validate required image field.
	image := strings.TrimSpace(mod.Image)
	if image == "" {
		return contracts.StepManifest{}, fmt.Errorf("healing mod[%d]: image required", index)
	}

	// Use typed command accessor to avoid polymorphic handling.
	command := mod.Command.ToSlice()

	// Use typed env map (already map[string]string).
	env := mod.Env
	if env == nil {
		env = make(map[string]string)
	}

	// Use typed retain flag.
	retain := mod.RetainContainer

	// For healing, reuse the existing workspace; do not re-hydrate the repo.

	// Create the healing manifest.
	// Healing mods do not run the build gate themselves; they only modify the workspace.
	manifest := contracts.StepManifest{
		ID:         types.StepID(fmt.Sprintf("%s-heal-%d", req.RunID, index)),
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
			// Allow in-container verification via buildgate API by mounting the host Docker socket.
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}
