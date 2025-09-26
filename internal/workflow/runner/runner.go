package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
)

var (
	ErrTicketRequired             = errors.New("ticket is required")
	ErrEventsClientRequired       = errors.New("events client is required")
	ErrGridClientRequired         = errors.New("grid client is required")
	ErrPlannerRequired            = errors.New("planner is required")
	ErrManifestCompilerRequired   = errors.New("manifest compiler is required")
	ErrTicketValidationFailed     = errors.New("ticket payload failed validation")
	ErrCheckpointValidationFailed = errors.New("checkpoint payload failed validation")
	ErrArtifactValidationFailed   = errors.New("artifact envelope failed validation")
	ErrStageFailed                = errors.New("workflow stage failed")
	ErrLaneRequired               = errors.New("lane is required")
	ErrAsterLocatorRequired       = errors.New("aster locator is required")
)

type StageKind string

const (
	StageKindMods  StageKind = "mods"
	StageKindBuild StageKind = "build"
	StageKindTest  StageKind = "test"
)

type Stage struct {
	Name         string
	Kind         StageKind
	Lane         string
	Dependencies []string
	Constraints  StageConstraints
	Aster        StageAster
	CacheKey     string
}

// Artifact represents a manifest describing an output produced by a workflow
// stage and referenced in checkpoints for downstream consumers.
type Artifact struct {
	Name        string
	ArtifactCID string
	Digest      string
	MediaType   string
}

type StageConstraints struct {
	Manifest manifests.Compilation
}

type StageAster struct {
	Enabled bool
	Toggles []string
	Bundles []aster.Metadata
}

type StageStatus = contracts.CheckpointStatus

const (
	StageStatusPending   StageStatus = contracts.CheckpointStatusPending
	StageStatusClaimed   StageStatus = contracts.CheckpointStatusClaimed
	StageStatusRunning   StageStatus = contracts.CheckpointStatusRunning
	StageStatusRetrying  StageStatus = contracts.CheckpointStatusRetrying
	StageStatusCompleted StageStatus = contracts.CheckpointStatusCompleted
	StageStatusFailed    StageStatus = contracts.CheckpointStatusFailed
)

type StageOutcome struct {
	Stage     Stage
	Status    StageStatus
	Retryable bool
	Message   string
	Artifacts []Artifact
}

type CacheComposeRequest struct {
	Stage  Stage
	Ticket contracts.WorkflowTicket
}

type CacheComposer interface {
	Compose(ctx context.Context, req CacheComposeRequest) (string, error)
}

type ExecutionPlan struct {
	TicketID string
	Stages   []Stage
}

type Planner interface {
	Build(ctx context.Context, ticket contracts.WorkflowTicket) (ExecutionPlan, error)
}

type DefaultPlanner struct{}

func NewDefaultPlanner() Planner {
	return DefaultPlanner{}
}

func (DefaultPlanner) Build(ctx context.Context, ticket contracts.WorkflowTicket) (ExecutionPlan, error) {
	_ = ctx
	plan := ExecutionPlan{
		TicketID: ticket.TicketID,
		Stages: []Stage{
			{Name: "mods", Kind: StageKindMods, Lane: "node-wasm"},
			{Name: "build", Kind: StageKindBuild, Lane: "go-native", Dependencies: []string{"mods"}},
			{Name: "test", Kind: StageKindTest, Lane: "go-native", Dependencies: []string{"build"}},
		},
	}
	return plan, nil
}

type GridClient interface {
	ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage Stage, workspace string) (StageOutcome, error)
}

type EventsClient interface {
	ClaimTicket(ctx context.Context, ticketID string) (contracts.WorkflowTicket, error)
	PublishCheckpoint(ctx context.Context, checkpoint contracts.WorkflowCheckpoint) error
	PublishArtifact(ctx context.Context, artifact contracts.WorkflowArtifact) error
}

type ManifestCompiler interface {
	Compile(ctx context.Context, ref contracts.ManifestReference) (manifests.Compilation, error)
}

type Options struct {
	Ticket           string
	Tenant           string
	Events           EventsClient
	Grid             GridClient
	Planner          Planner
	WorkspaceRoot    string
	MaxStageRetries  int
	ManifestCompiler ManifestCompiler
	Aster            AsterOptions
	CacheComposer    CacheComposer
}

type AsterOptions struct {
	Locator           aster.Locator
	AdditionalToggles []string
	StageOverrides    map[string]AsterStageOverride
}

type AsterStageOverride struct {
	Disable      bool
	ExtraToggles []string
}

func Run(ctx context.Context, opts Options) (err error) {
	if opts.Events == nil {
		return ErrEventsClientRequired
	}
	if opts.Grid == nil {
		return ErrGridClientRequired
	}
	if opts.ManifestCompiler == nil {
		return ErrManifestCompilerRequired
	}

	planner := opts.Planner
	if planner == nil {
		planner = NewDefaultPlanner()
	}

	trimmedTicket := strings.TrimSpace(opts.Ticket)

	ticket, err := opts.Events.ClaimTicket(ctx, trimmedTicket)
	if err != nil {
		return err
	}
	if err := ticket.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrTicketValidationFailed, err)
	}

	compiledManifest, err := opts.ManifestCompiler.Compile(ctx, ticket.Manifest)
	if err != nil {
		return err
	}

	plan, err := planner.Build(ctx, ticket)
	if err != nil {
		return err
	}

	composer := opts.CacheComposer
	if composer == nil {
		composer = defaultCacheComposer{}
	}

	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = os.TempDir()
	}

	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}

	workspace, err := os.MkdirTemp(workspaceRoot, "ploy-workflow-")
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	workspace = filepath.Clean(workspace)
	defer func() {
		if removeErr := os.RemoveAll(workspace); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("workspace cleanup: %w", removeErr))
		}
	}()

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "ticket-claimed", StageStatusCompleted, "", nil, nil); err != nil {
		return err
	}

	maxRetries := opts.MaxStageRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	for _, stage := range plan.Stages {
		if strings.TrimSpace(stage.Lane) == "" {
			return fmt.Errorf("%w: %s", ErrLaneRequired, stage.Name)
		}
		stage.Constraints.Manifest = compiledManifest
		asterMeta, err := resolveStageAster(ctx, stage, compiledManifest, opts.Aster)
		if err != nil {
			return err
		}
		stage.Aster = asterMeta
		cacheKey, err := composer.Compose(ctx, CacheComposeRequest{Stage: stage, Ticket: ticket})
		if err != nil {
			return fmt.Errorf("compose cache key for stage %s: %w", stage.Name, err)
		}
		stage.CacheKey = strings.TrimSpace(cacheKey)
		for attempt := 0; ; attempt++ {
			runningStage := stage
			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRunning, stage.CacheKey, &runningStage, nil); err != nil {
				return err
			}

			outcome, execErr := opts.Grid.ExecuteStage(ctx, ticket, stage, workspace)
			if execErr != nil {
				return execErr
			}

			executedStage := resolvedStage(stage, outcome.Stage)

			status := outcome.Status
			if status == "" {
				status = StageStatusCompleted
			}

			if status == StageStatusFailed {
				if outcome.Retryable && attempt < maxRetries {
					stageCopy := executedStage
					if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRetrying, stage.CacheKey, &stageCopy, nil); err != nil {
						return err
					}
					continue
				}

				stageCopy := executedStage
				if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusFailed, stage.CacheKey, &stageCopy, nil); err != nil {
					return err
				}
				_ = publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusFailed, "", nil, nil)

				message := outcome.Message
				if strings.TrimSpace(message) == "" {
					message = "stage failed"
				}
				return fmt.Errorf("%w: stage %s: %s", ErrStageFailed, stage.Name, message)
			}

			stageCopy := executedStage
			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusCompleted, stage.CacheKey, &stageCopy, outcome.Artifacts); err != nil {
				return err
			}
			break
		}
	}

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusCompleted, "", nil, nil); err != nil {
		return err
	}

	return nil
}

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

type defaultCacheComposer struct{}

func (defaultCacheComposer) Compose(ctx context.Context, req CacheComposeRequest) (string, error) {
	_ = ctx
	lane := strings.TrimSpace(req.Stage.Lane)
	if lane == "" {
		return "", fmt.Errorf("lane missing")
	}
	manifest := strings.TrimSpace(req.Stage.Constraints.Manifest.Manifest.Version)
	if manifest == "" {
		manifest = "unknown"
	}
	toggles := "none"
	if len(req.Stage.Aster.Toggles) > 0 {
		toggles = strings.Join(req.Stage.Aster.Toggles, "+")
	}
	return fmt.Sprintf("%s/%s@manifest=%s@aster=%s", lane, lane, manifest, toggles), nil
}

func resolveStageAster(ctx context.Context, stage Stage, manifest manifests.Compilation, opts AsterOptions) (StageAster, error) {
	stageKey := strings.ToLower(strings.TrimSpace(stage.Name))
	override := opts.StageOverrides
	if override == nil {
		override = map[string]AsterStageOverride{}
	}
	config := override[stageKey]
	if config.Disable {
		return StageAster{}, nil
	}

	active := make([]string, 0, len(manifest.Aster.Required)+len(opts.AdditionalToggles)+len(config.ExtraToggles))
	active = append(active, manifest.Aster.Required...)
	active = append(active, opts.AdditionalToggles...)
	active = append(active, config.ExtraToggles...)
	toggles := normalizeAsterToggles(active)
	if len(toggles) == 0 {
		return StageAster{}, nil
	}
	if opts.Locator == nil {
		return StageAster{}, ErrAsterLocatorRequired
	}
	bundles := make([]aster.Metadata, 0, len(toggles))
	stageName := strings.TrimSpace(stage.Name)
	for _, toggle := range toggles {
		if err := ctx.Err(); err != nil {
			return StageAster{}, err
		}
		meta, err := opts.Locator.Locate(ctx, aster.Request{Stage: stageName, Toggle: toggle})
		if err != nil {
			return StageAster{}, fmt.Errorf("locate Aster bundle for stage %s toggle %s: %w", stageName, toggle, err)
		}
		if strings.TrimSpace(meta.Stage) == "" {
			meta.Stage = stageName
		}
		if strings.TrimSpace(meta.Toggle) == "" {
			meta.Toggle = toggle
		}
		bundles = append(bundles, meta)
	}
	return StageAster{Enabled: true, Toggles: toggles, Bundles: bundles}, nil
}

func normalizeAsterToggles(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(value)); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

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
	return meta
}

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

func resolvedStage(base Stage, outcome Stage) Stage {
	resolved := outcome
	if strings.TrimSpace(resolved.Name) == "" {
		return base
	}
	if strings.TrimSpace(resolved.Lane) == "" {
		resolved.Lane = base.Lane
	}
	if len(resolved.Dependencies) == 0 {
		resolved.Dependencies = copyStringSlice(base.Dependencies)
	}
	if strings.TrimSpace(resolved.CacheKey) == "" {
		resolved.CacheKey = base.CacheKey
	}
	if resolved.Constraints.Manifest.Manifest.Name == "" && resolved.Constraints.Manifest.Manifest.Version == "" {
		resolved.Constraints.Manifest = base.Constraints.Manifest
	}
	if !resolved.Aster.Enabled && base.Aster.Enabled {
		resolved.Aster = base.Aster
	} else {
		if resolved.Aster.Enabled {
			if len(resolved.Aster.Toggles) == 0 {
				resolved.Aster.Toggles = copyStringSlice(base.Aster.Toggles)
			}
			if len(resolved.Aster.Bundles) == 0 {
				resolved.Aster.Bundles = append([]aster.Metadata(nil), base.Aster.Bundles...)
			}
		}
	}
	return resolved
}

func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
