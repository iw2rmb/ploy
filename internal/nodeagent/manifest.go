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
func buildManifestFromRequest(req StartRunRequest) (contracts.StepManifest, error) {
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
	var command []string
	if imgOpt, ok := req.Options["image"].(string); ok && strings.TrimSpace(imgOpt) != "" {
		image = strings.TrimSpace(imgOpt)
	}
	// Accept command as []string or single shell string.
	switch v := req.Options["command"].(type) {
	case []string:
		if len(v) > 0 {
			command = v
		}
	case string:
		if s := strings.TrimSpace(v); s != "" {
			command = []string{"/bin/sh", "-c", s}
		}
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

	// Create a single read-write input that will be hydrated from the repository.
	// Defensive copy of env to avoid aliasing caller map.
	env := make(map[string]string, len(req.Env))
	for k, v := range req.Env {
		env[k] = v
	}

	// Optional: allow spec to request container retention for post-run inspection.
	retain := false
	if b, ok := req.Options["retain_container"].(bool); ok {
		retain = b
	}

	// Extract GitLab-related options that will be consumed by later phases.
	// These options are: gitlab_pat, gitlab_domain, mr_on_success, mr_on_fail.
	// We store them in the manifest Options field without logging them.
	gitlabOpts := make(map[string]any)
	if pat, ok := req.Options["gitlab_pat"].(string); ok && strings.TrimSpace(pat) != "" {
		gitlabOpts["gitlab_pat"] = strings.TrimSpace(pat)
	}
	if domain, ok := req.Options["gitlab_domain"].(string); ok && strings.TrimSpace(domain) != "" {
		gitlabOpts["gitlab_domain"] = strings.TrimSpace(domain)
	}
	if mrSuccess, ok := req.Options["mr_on_success"].(bool); ok {
		gitlabOpts["mr_on_success"] = mrSuccess
	}
	if mrFail, ok := req.Options["mr_on_fail"].(bool); ok {
		gitlabOpts["mr_on_fail"] = mrFail
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
		Options: gitlabOpts,
	}

	return manifest, nil
}

// buildHealingManifest constructs a StepManifest from a healing mod entry.
// The healing mod runs with /workspace (RW), /out (RW), and /in (RO) mounts.
func buildHealingManifest(req StartRunRequest, modEntry any, index int) (contracts.StepManifest, error) {
	entry, ok := modEntry.(map[string]any)
	if !ok {
		return contracts.StepManifest{}, fmt.Errorf("healing mod[%d]: expected map, got %T", index, modEntry)
	}

	// Extract image (required).
	image, ok := entry["image"].(string)
	if !ok || strings.TrimSpace(image) == "" {
		return contracts.StepManifest{}, fmt.Errorf("healing mod[%d]: image required", index)
	}
	image = strings.TrimSpace(image)

	// Extract command (optional).
	var command []string
	switch v := entry["command"].(type) {
	case []any:
		for _, c := range v {
			if s, ok := c.(string); ok {
				command = append(command, s)
			}
		}
	case string:
		if s := strings.TrimSpace(v); s != "" {
			command = []string{"/bin/sh", "-c", s}
		}
	}

	// Extract env (optional).
	env := make(map[string]string)
	if envMap, ok := entry["env"].(map[string]any); ok {
		for k, v := range envMap {
			if s, ok := v.(string); ok {
				env[k] = s
			}
		}
	}

	// Extract retain_container (optional).
	retain := false
	if b, ok := entry["retain_container"].(bool); ok {
		retain = b
	}

	// Build the repo materialization (same as main mod).
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
				Hydration: &contracts.StepInputHydration{
					Repo: &repo,
				},
			},
		},
		Retention: contracts.StepRetentionSpec{
			RetainContainer: retain,
			TTL:             types.Duration(time.Hour),
		},
		Options: map[string]any{
			// Allow in-container verification via ploy-buildgate by mounting the host Docker socket.
			"mount_docker_socket": true,
		},
	}

	return manifest, nil
}
