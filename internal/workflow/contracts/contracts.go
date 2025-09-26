package contracts

import (
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = "2025-09-26.1"

type SubjectSet struct {
	TicketInbox      string
	CheckpointStream string
	ArtifactStream   string
	StatusStream     string
}

func SubjectsForTenant(tenant, ticketID string) SubjectSet {
	return SubjectSet{
		TicketInbox:      fmt.Sprintf("grid.webhook.%s", tenant),
		CheckpointStream: fmt.Sprintf("ploy.workflow.%s.checkpoints", ticketID),
		ArtifactStream:   fmt.Sprintf("ploy.artifact.%s", ticketID),
		StatusStream:     fmt.Sprintf("grid.status.%s", ticketID),
	}
}

type WorkflowTicket struct {
	SchemaVersion string            `json:"schema_version"`
	TicketID      string            `json:"ticket_id"`
	Tenant        string            `json:"tenant"`
	Manifest      ManifestReference `json:"manifest"`
}

func (t WorkflowTicket) Validate() error {
	if t.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if t.TicketID == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if t.Tenant == "" {
		return fmt.Errorf("tenant is required")
	}
	if err := t.Manifest.Validate(); err != nil {
		return fmt.Errorf("manifest invalid: %w", err)
	}
	return nil
}

type ManifestReference struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (m ManifestReference) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("version is required")
	}
	return nil
}

// CheckpointStage captures DAG metadata for a workflow stage recorded in a
// checkpoint. It mirrors the planner output so consumers can reconstruct
// dependencies and lane assignments without inspecting the CLI runtime state.
type CheckpointStage struct {
	Name         string               `json:"name"`
	Kind         string               `json:"kind"`
	Lane         string               `json:"lane"`
	Dependencies []string             `json:"dependencies,omitempty"`
	Manifest     ManifestReference    `json:"manifest"`
	Aster        CheckpointStageAster `json:"aster"`
	Mods         *ModsStageMetadata   `json:"mods,omitempty"`
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

// ModsStageMetadata captures Mods-specific checkpoint metadata.
type ModsStageMetadata struct {
	Plan            *ModsPlanMetadata    `json:"plan,omitempty"`
	Human           *ModsHumanMetadata   `json:"human,omitempty"`
	Recommendations []ModsRecommendation `json:"recommendations,omitempty"`
}

// Validate ensures Mods metadata entries are well formed.
func (m ModsStageMetadata) Validate() error {
	if m.Plan != nil {
		if err := m.Plan.Validate(); err != nil {
			return fmt.Errorf("plan metadata invalid: %w", err)
		}
	}
	if m.Human != nil {
		if err := m.Human.Validate(); err != nil {
			return fmt.Errorf("human metadata invalid: %w", err)
		}
	}
	for i, rec := range m.Recommendations {
		if err := rec.Validate(); err != nil {
			return fmt.Errorf("recommendation %d invalid: %w", i, err)
		}
	}
	return nil
}

// ModsPlanMetadata documents planner decisions included in checkpoints.
type ModsPlanMetadata struct {
	SelectedRecipes []string `json:"selected_recipes,omitempty"`
	ParallelStages  []string `json:"parallel_stages,omitempty"`
	HumanGate       bool     `json:"human_gate"`
	Summary         string   `json:"summary,omitempty"`
	PlanTimeout     string   `json:"plan_timeout,omitempty"`
	MaxParallel     int      `json:"max_parallel,omitempty"`
}

// Validate ensures Mods plan metadata entries contain non-empty values.
func (m ModsPlanMetadata) Validate() error {
	for i, recipe := range m.SelectedRecipes {
		if strings.TrimSpace(recipe) == "" {
			return fmt.Errorf("selected recipe %d is empty", i)
		}
	}
	for i, stage := range m.ParallelStages {
		if strings.TrimSpace(stage) == "" {
			return fmt.Errorf("parallel stage %d is empty", i)
		}
	}
	if trimmed := strings.TrimSpace(m.PlanTimeout); trimmed != "" {
		if _, err := time.ParseDuration(trimmed); err != nil {
			return fmt.Errorf("plan timeout invalid: %w", err)
		}
	}
	if m.MaxParallel < 0 {
		return fmt.Errorf("max parallel cannot be negative")
	}
	return nil
}

// ModsHumanMetadata captures human checkpoint expectations.
type ModsHumanMetadata struct {
	Required  bool     `json:"required"`
	Playbooks []string `json:"playbooks,omitempty"`
}

// Validate ensures Mods human metadata contains valid playbooks.
func (m ModsHumanMetadata) Validate() error {
	for i, playbook := range m.Playbooks {
		if strings.TrimSpace(playbook) == "" {
			return fmt.Errorf("playbook %d is empty", i)
		}
	}
	return nil
}

// ModsRecommendation records a single recommendation entry.
type ModsRecommendation struct {
	Source     string  `json:"source,omitempty"`
	Message    string  `json:"message"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Validate ensures the recommendation message exists and confidence is bounded.
func (m ModsRecommendation) Validate() error {
	if strings.TrimSpace(m.Message) == "" {
		return fmt.Errorf("recommendation message is required")
	}
	if m.Confidence < 0 || m.Confidence > 1 {
		return fmt.Errorf("recommendation confidence must be within [0,1]")
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

// CheckpointArtifact records an artifact manifest emitted by a workflow stage.
type CheckpointArtifact struct {
	Name        string `json:"name"`
	ArtifactCID string `json:"artifact_cid,omitempty"`
	Digest      string `json:"digest,omitempty"`
	MediaType   string `json:"media_type,omitempty"`
}

// Validate ensures the artifact manifest has at least a display name.
func (a CheckpointArtifact) Validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("artifact name is required")
	}
	return nil
}

type CheckpointStatus string

const (
	CheckpointStatusPending   CheckpointStatus = "pending"
	CheckpointStatusClaimed   CheckpointStatus = "claimed"
	CheckpointStatusRunning   CheckpointStatus = "running"
	CheckpointStatusRetrying  CheckpointStatus = "retrying"
	CheckpointStatusCompleted CheckpointStatus = "completed"
	CheckpointStatusFailed    CheckpointStatus = "failed"
)

type WorkflowCheckpoint struct {
	SchemaVersion string               `json:"schema_version"`
	TicketID      string               `json:"ticket_id"`
	Stage         string               `json:"stage"`
	Status        CheckpointStatus     `json:"status"`
	CacheKey      string               `json:"cache_key,omitempty"`
	StageMetadata *CheckpointStage     `json:"stage_metadata,omitempty"`
	Artifacts     []CheckpointArtifact `json:"artifacts,omitempty"`
}

func (c WorkflowCheckpoint) Validate() error {
	if c.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if c.TicketID == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if c.Stage == "" {
		return fmt.Errorf("stage is required")
	}
	if c.Status == "" {
		return fmt.Errorf("status is required")
	}
	if c.StageMetadata != nil {
		if err := c.StageMetadata.Validate(); err != nil {
			return fmt.Errorf("stage metadata invalid: %w", err)
		}
		if c.Stage != "" && strings.TrimSpace(c.StageMetadata.Name) != strings.TrimSpace(c.Stage) {
			return fmt.Errorf("stage metadata name mismatch")
		}
	}
	if len(c.Artifacts) > 0 {
		if c.StageMetadata == nil {
			return fmt.Errorf("artifacts require stage metadata")
		}
		for i, artifact := range c.Artifacts {
			if err := artifact.Validate(); err != nil {
				return fmt.Errorf("artifact %d invalid: %w", i, err)
			}
		}
	}
	return nil
}

func (c WorkflowCheckpoint) Subject() string {
	return fmt.Sprintf("ploy.workflow.%s.checkpoints", c.TicketID)
}

// WorkflowArtifact represents an artifact envelope published to the artifact
// stream for consumers that hydrate caches without reading checkpoints.
type WorkflowArtifact struct {
	SchemaVersion string             `json:"schema_version"`
	TicketID      string             `json:"ticket_id"`
	Stage         string             `json:"stage"`
	CacheKey      string             `json:"cache_key,omitempty"`
	StageMetadata *CheckpointStage   `json:"stage_metadata,omitempty"`
	Artifact      CheckpointArtifact `json:"artifact"`
}

// Validate ensures the artifact envelope is well formed.
func (a WorkflowArtifact) Validate() error {
	if strings.TrimSpace(a.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(a.TicketID) == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if strings.TrimSpace(a.Stage) == "" {
		return fmt.Errorf("stage is required")
	}
	if a.StageMetadata != nil {
		if err := a.StageMetadata.Validate(); err != nil {
			return fmt.Errorf("stage metadata invalid: %w", err)
		}
		if strings.TrimSpace(a.StageMetadata.Name) != strings.TrimSpace(a.Stage) {
			return fmt.Errorf("stage metadata name mismatch")
		}
	}
	if err := a.Artifact.Validate(); err != nil {
		return fmt.Errorf("artifact invalid: %w", err)
	}
	return nil
}

// Subject returns the JetStream subject for the artifact envelope.
func (a WorkflowArtifact) Subject() string {
	return fmt.Sprintf("ploy.artifact.%s", a.TicketID)
}
