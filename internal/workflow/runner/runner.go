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

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "ticket-claimed", StageStatusCompleted, ""); err != nil {
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
			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRunning, stage.CacheKey); err != nil {
				return err
			}

			outcome, execErr := opts.Grid.ExecuteStage(ctx, ticket, stage, workspace)
			if execErr != nil {
				return execErr
			}

			if outcome.Stage.Name == "" {
				outcome.Stage = stage
			}

			status := outcome.Status
			if status == "" {
				status = StageStatusCompleted
			}

			if status == StageStatusFailed {
				if outcome.Retryable && attempt < maxRetries {
					if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusRetrying, stage.CacheKey); err != nil {
						return err
					}
					continue
				}

				if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusFailed, stage.CacheKey); err != nil {
					return err
				}
				_ = publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusFailed, "")

				message := outcome.Message
				if strings.TrimSpace(message) == "" {
					message = "stage failed"
				}
				return fmt.Errorf("%w: stage %s: %s", ErrStageFailed, stage.Name, message)
			}

			if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, stage.Name, StageStatusCompleted, stage.CacheKey); err != nil {
				return err
			}
			break
		}
	}

	if err := publishCheckpoint(ctx, opts.Events, ticket.TicketID, "workflow", StageStatusCompleted, ""); err != nil {
		return err
	}

	return nil
}

func publishCheckpoint(ctx context.Context, events EventsClient, ticketID, stage string, status StageStatus, cacheKey string) error {
	checkpoint := contracts.WorkflowCheckpoint{
		SchemaVersion: contracts.SchemaVersion,
		TicketID:      ticketID,
		Stage:         stage,
		Status:        contracts.CheckpointStatus(status),
		CacheKey:      cacheKey,
	}
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrCheckpointValidationFailed, err)
	}
	return events.PublishCheckpoint(ctx, checkpoint)
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
