package nodeagent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildManifestFromRequest converts a StartRunRequest into a StepManifest.
func buildManifestFromRequest(req StartRunRequest) (contracts.StepManifest, error) {
	if strings.TrimSpace(req.RunID) == "" {
		return contracts.StepManifest{}, errors.New("run_id required")
	}
	if strings.TrimSpace(req.RepoURL) == "" {
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
	targetRef := strings.TrimSpace(req.TargetRef)
	if targetRef == "" && strings.TrimSpace(req.BaseRef) != "" {
		targetRef = strings.TrimSpace(req.BaseRef)
	}

	// Build the repo materialization.
	repo := contracts.RepoMaterialization{
		URL:       req.RepoURL,
		BaseRef:   req.BaseRef,
		TargetRef: targetRef,
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

	manifest := contracts.StepManifest{
		ID:         req.RunID,
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
			TTL:             "1h",
		},
	}

	return manifest, nil
}
