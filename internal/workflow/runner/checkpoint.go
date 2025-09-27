package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// publishCheckpoint records stage progress and publishes artifacts when available.
func publishCheckpoint(ctx context.Context, events EventsClient, ticketID, stage string, status StageStatus, cacheKey string, stageMeta *Stage, artifacts []Artifact) error {
	checkpoint := contracts.WorkflowCheckpoint{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Stage:         stage,
		Status:        contracts.CheckpointStatus(status),
		CacheKey:      cacheKey,
	}
	if stageMeta != nil {
		if meta := buildCheckpointStage(*stageMeta); meta != nil {
			checkpoint.StageMetadata = meta
		}
	}
	if len(artifacts) > 0 {
		checkpoint.Artifacts = buildCheckpointArtifacts(artifacts)
	}
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrCheckpointValidationFailed, err)
	}
	if err := events.PublishCheckpoint(ctx, checkpoint); err != nil {
		return err
	}
	if status != StageStatusCompleted {
		return nil
	}
	if checkpoint.StageMetadata == nil {
		return nil
	}
	if len(checkpoint.Artifacts) == 0 {
		return nil
	}
	envelopes := buildWorkflowArtifacts(ticketID, stage, cacheKey, checkpoint.StageMetadata, checkpoint.Artifacts)
	for _, envelope := range envelopes {
		if err := envelope.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrArtifactValidationFailed, err)
		}
		if err := events.PublishArtifact(ctx, envelope); err != nil {
			return fmt.Errorf("publish artifact envelope: %w", err)
		}
	}
	return nil
}

// buildCheckpointStage converts stage metadata into checkpoint form.
func buildCheckpointStage(stage Stage) *contracts.CheckpointStage {
	name := strings.TrimSpace(stage.Name)
	if name == "" {
		return nil
	}
	meta := &contracts.CheckpointStage{
		Name:         name,
		Kind:         string(stage.Kind),
		Lane:         strings.TrimSpace(stage.Lane),
		Dependencies: copyStringSlice(stage.Dependencies),
		Manifest: contracts.ManifestReference{
			Name:    strings.TrimSpace(stage.Constraints.Manifest.Manifest.Name),
			Version: strings.TrimSpace(stage.Constraints.Manifest.Manifest.Version),
		},
		Aster: buildCheckpointStageAster(stage.Aster),
	}
	if modsMeta := buildCheckpointModsMetadata(stage.Metadata.Mods); modsMeta != nil {
		meta.Mods = modsMeta
	}
	return meta
}

// buildCheckpointStageAster converts stage Aster metadata into checkpoint metadata.
func buildCheckpointStageAster(stage StageAster) contracts.CheckpointStageAster {
	result := contracts.CheckpointStageAster{
		Enabled: stage.Enabled,
		Toggles: copyStringSlice(stage.Toggles),
	}
	if len(stage.Bundles) > 0 {
		result.Bundles = make([]contracts.CheckpointAsterBundle, 0, len(stage.Bundles))
		for _, bundle := range stage.Bundles {
			result.Bundles = append(result.Bundles, contracts.CheckpointAsterBundle{
				Stage:       strings.TrimSpace(bundle.Stage),
				Toggle:      strings.TrimSpace(bundle.Toggle),
				BundleID:    strings.TrimSpace(bundle.BundleID),
				Digest:      strings.TrimSpace(bundle.Digest),
				ArtifactCID: strings.TrimSpace(bundle.ArtifactCID),
				Source:      strings.TrimSpace(bundle.Source),
			})
		}
	}
	if !result.Enabled && (len(result.Toggles) > 0 || len(result.Bundles) > 0) {
		result.Enabled = true
	}
	return result
}

// buildCheckpointModsMetadata converts stage Mods metadata into contract metadata.
func buildCheckpointModsMetadata(meta *StageModsMetadata) *contracts.ModsStageMetadata {
	if meta == nil {
		return nil
	}
	result := &contracts.ModsStageMetadata{}
	if meta.Plan != nil {
		result.Plan = &contracts.ModsPlanMetadata{
			SelectedRecipes: copyStringSlice(meta.Plan.SelectedRecipes),
			ParallelStages:  copyStringSlice(meta.Plan.ParallelStages),
			HumanGate:       meta.Plan.HumanGate,
			Summary:         strings.TrimSpace(meta.Plan.Summary),
			PlanTimeout:     strings.TrimSpace(meta.Plan.PlanTimeout),
			MaxParallel:     meta.Plan.MaxParallel,
		}
	}
	if meta.Human != nil {
		result.Human = &contracts.ModsHumanMetadata{
			Required:  meta.Human.Required,
			Playbooks: copyStringSlice(meta.Human.Playbooks),
		}
	}
	if len(meta.Recommendations) > 0 {
		result.Recommendations = make([]contracts.ModsRecommendation, 0, len(meta.Recommendations))
		for _, rec := range meta.Recommendations {
			message := strings.TrimSpace(rec.Message)
			if message == "" {
				continue
			}
			source := strings.TrimSpace(rec.Source)
			result.Recommendations = append(result.Recommendations, contracts.ModsRecommendation{
				Source:     source,
				Message:    message,
				Confidence: clampConfidence(rec.Confidence),
			})
		}
	}
	if result.Plan == nil && result.Human == nil && len(result.Recommendations) == 0 {
		return nil
	}
	return result
}

// buildCheckpointArtifacts copies artifact references into checkpoint format.
func buildCheckpointArtifacts(artifacts []Artifact) []contracts.CheckpointArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	result := make([]contracts.CheckpointArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		name := strings.TrimSpace(artifact.Name)
		cid := strings.TrimSpace(artifact.ArtifactCID)
		digest := strings.TrimSpace(artifact.Digest)
		mediaType := strings.TrimSpace(artifact.MediaType)
		if name == "" && cid == "" {
			continue
		}
		result = append(result, contracts.CheckpointArtifact{
			Name:        name,
			ArtifactCID: cid,
			Digest:      digest,
			MediaType:   mediaType,
		})
	}
	return result
}

// buildWorkflowArtifacts produces workflow artifact envelopes from checkpoint state.
func buildWorkflowArtifacts(ticketID, stage, cacheKey string, stageMeta *contracts.CheckpointStage, artifacts []contracts.CheckpointArtifact) []contracts.WorkflowArtifact {
	if stageMeta == nil || len(artifacts) == 0 {
		return nil
	}
	result := make([]contracts.WorkflowArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		metaCopy := *stageMeta
		envelope := contracts.WorkflowArtifact{
			SchemaVersion: contracts.SchemaVersion,
			TicketID:      ticketID,
			Stage:         stage,
			CacheKey:      cacheKey,
			StageMetadata: &metaCopy,
			Artifact:      artifact,
		}
		result = append(result, envelope)
	}
	return result
}
