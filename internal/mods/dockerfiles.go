package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	build "github.com/iw2rmb/ploy/internal/build"
	project "github.com/iw2rmb/ploy/internal/detect/project"
	lanedetect "github.com/iw2rmb/ploy/internal/lane"
)

// ensureDockerfilePair ensures build/deploy Dockerfiles exist for lane D builds.
func ensureDockerfilePair(repoPath string) error {
	if strings.TrimSpace(repoPath) == "" {
		return fmt.Errorf("repo path is empty")
	}
	buildPath := filepath.Join(repoPath, "build.Dockerfile")
	deployPath := filepath.Join(repoPath, "deploy.Dockerfile")
	buildExists := fileExists(buildPath)
	deployExists := fileExists(deployPath)
	if buildExists && deployExists {
		return nil
	}

	detect := lanedetect.Detect(repoPath)
	facts := project.ComputeFacts(repoPath, detect.Language)
	lang := strings.ToLower(strings.TrimSpace(facts.Language))
	switch lang {
	case "java", "kotlin", "scala":
		// supported
	default:
		return nil
	}

	pair, err := build.RenderDockerfilePair(facts)
	if err != nil {
		return fmt.Errorf("render dockerfile pair: %w", err)
	}

	if !buildExists {
		if err := os.WriteFile(buildPath, []byte(strings.TrimSpace(pair.Build)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write build.Dockerfile: %w", err)
		}
	}
	if !deployExists {
		if err := os.WriteFile(deployPath, []byte(strings.TrimSpace(pair.Deploy)+"\n"), 0o644); err != nil {
			return fmt.Errorf("write deploy.Dockerfile: %w", err)
		}
	}
	return nil
}

func fileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
