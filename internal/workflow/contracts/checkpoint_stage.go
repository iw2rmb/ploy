package contracts

import (
	"fmt"
	"strings"
)

// CheckpointStage captures DAG metadata for a workflow stage recorded in a
// checkpoint. It mirrors the planner output so consumers can reconstruct
// dependencies and lane assignments without inspecting the CLI runtime state.
type CheckpointStage struct {
	Name         string                  `json:"name"`
	Kind         string                  `json:"kind"`
	Lane         string                  `json:"lane"`
	Dependencies []string                `json:"dependencies,omitempty"`
	Manifest     ManifestReference       `json:"manifest"`
	Aster        CheckpointStageAster    `json:"aster"`
	BuildGate    *BuildGateStageMetadata `json:"build_gate,omitempty"`
	Mods         *ModsStageMetadata      `json:"mods,omitempty"`
}

// Validate ensures the stage metadata includes the required identifiers.
func (s CheckpointStage) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("stage name is required")
	}
	if strings.TrimSpace(s.Kind) == "" {
		return fmt.Errorf("stage kind is required")
	}
	if strings.TrimSpace(s.Lane) == "" {
		return fmt.Errorf("stage lane is required")
	}
	for i, dep := range s.Dependencies {
		if strings.TrimSpace(dep) == "" {
			return fmt.Errorf("dependency %d is empty", i)
		}
	}
	if err := s.Manifest.Validate(); err != nil {
		return fmt.Errorf("manifest invalid: %w", err)
	}
	if err := s.Aster.Validate(); err != nil {
		return fmt.Errorf("aster metadata invalid: %w", err)
	}
	if s.BuildGate != nil {
		if err := s.BuildGate.Validate(); err != nil {
			return fmt.Errorf("build gate metadata invalid: %w", err)
		}
	}
	if s.Mods != nil {
		if err := s.Mods.Validate(); err != nil {
			return fmt.Errorf("mods metadata invalid: %w", err)
		}
	}
	return nil
}

// CheckpointStageAster describes the active Aster toggles and bundle metadata
// for a checkpointed stage.
type CheckpointStageAster struct {
	Enabled bool                    `json:"enabled"`
	Toggles []string                `json:"toggles,omitempty"`
	Bundles []CheckpointAsterBundle `json:"bundles,omitempty"`
}

// Validate ensures bundle metadata is well-formed.
func (a CheckpointStageAster) Validate() error {
	for i, bundle := range a.Bundles {
		if err := bundle.Validate(); err != nil {
			return fmt.Errorf("bundle %d invalid: %w", i, err)
		}
	}
	return nil
}

// CheckpointAsterBundle carries bundle provenance for a single
// stage/toggle pair.
type CheckpointAsterBundle struct {
	Stage       string `json:"stage"`
	Toggle      string `json:"toggle"`
	BundleID    string `json:"bundle_id"`
	Digest      string `json:"digest,omitempty"`
	ArtifactCID string `json:"artifact_cid,omitempty"`
	Source      string `json:"source,omitempty"`
}

// Validate ensures the bundle metadata lists the identifying fields.
func (b CheckpointAsterBundle) Validate() error {
	if strings.TrimSpace(b.Stage) == "" {
		return fmt.Errorf("bundle stage is required")
	}
	if strings.TrimSpace(b.Toggle) == "" {
		return fmt.Errorf("bundle toggle is required")
	}
	if strings.TrimSpace(b.BundleID) == "" {
		return fmt.Errorf("bundle_id is required")
	}
	return nil
}
