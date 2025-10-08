package runner

import (
	"context"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/manifests"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
)

func handleHealing(ctx context.Context, res stageResult, attempts *int, stageNodes map[string]*stageNode, stageOrder map[string]int, orderedNodes *[]*stageNode, scheduled map[string]bool, ticket contracts.WorkflowTicket, compiled manifests.Compilation, opts Options, composer CacheComposer, start func([]*stageNode)) (bool, int, int, error) {
	if !shouldScheduleHealing(res, *attempts, opts.Mods) {
		return false, 0, 0, nil
	}
	attempt := *attempts + 1
	skippedNames := collectDependentStages(res.node, stageNodes)
	for _, name := range skippedNames {
		scheduled[name] = true
	}
	ready, added, err := appendHealingPlan(ctx, ticket, compiled, opts, composer, stageNodes, stageOrder, orderedNodes, attempt, res.outcome)
	if err != nil {
		return true, 0, 0, err
	}
	*attempts = attempt
	if len(ready) > 0 {
		start(ready)
	}
	return true, added, len(skippedNames), nil
}

func shouldScheduleHealing(res stageResult, attempts int, modsOpts ModsOptions) bool {
	if res.outcome.Stage.Kind != StageKindBuildGate {
		return false
	}
	if !res.outcome.Retryable {
		return false
	}
	if attempts >= 1 {
		return false
	}
	if strings.TrimSpace(modsOpts.PlanLane) == "" {
		return false
	}
	return true
}

func appendHealingPlan(ctx context.Context, ticket contracts.WorkflowTicket, compiled manifests.Compilation, opts Options, composer CacheComposer, stageNodes map[string]*stageNode, stageOrder map[string]int, orderedNodes *[]*stageNode, attempt int, outcome StageOutcome) ([]*stageNode, int, error) {
	plannerOpts := mods.Options{
		PlanLane:        opts.Mods.PlanLane,
		OpenRewriteLane: opts.Mods.OpenRewriteLane,
		LLMPlanLane:     opts.Mods.LLMPlanLane,
		LLMExecLane:     opts.Mods.LLMExecLane,
		HumanLane:       opts.Mods.HumanLane,
		PlanTimeout:     opts.Mods.PlanTimeout,
		MaxParallel:     opts.Mods.MaxParallel,
		Advisor:         opts.Mods.Advisor,
	}
	modPlanner := mods.NewPlanner(plannerOpts)
	signals := buildHealingSignals(outcome)
	modStages, err := modPlanner.Plan(ctx, mods.PlanInput{Ticket: ticket, Signals: signals})
	if err != nil {
		return nil, 0, err
	}
	suffix := fmt.Sprintf("#heal%d", attempt)
	ready := make([]*stageNode, 0, len(modStages))
	added := 0
	for _, modStage := range modStages {
		stage := Stage{
			Name:         modStage.Name + suffix,
			Kind:         StageKind(modStage.Kind),
			Lane:         strings.TrimSpace(modStage.Lane),
			Dependencies: renameDependencies(modStage.Dependencies, suffix),
			Metadata:     convertStageMetadata(modStage.Metadata),
		}
		if stage.Lane == "" {
			stage.Lane = modStage.Lane
		}
		stage.Constraints.Manifest = compiled
		asterMeta, err := resolveStageAster(ctx, stage, compiled, opts.Aster)
		if err != nil {
			return nil, 0, err
		}
		stage.Aster = asterMeta
		cacheKey, err := composer.Compose(ctx, CacheComposeRequest{Stage: stage, Ticket: ticket})
		if err != nil {
			return nil, 0, err
		}
		stage.CacheKey = strings.TrimSpace(cacheKey)
		node := &stageNode{Stage: stage, Remaining: len(stage.Dependencies)}
		stageNodes[stage.Name] = node
		stageOrder[stage.Name] = len(*orderedNodes)
		*orderedNodes = append(*orderedNodes, node)
		if node.Remaining == 0 {
			ready = append(ready, node)
		}
		for _, dep := range stage.Dependencies {
			if depNode, ok := stageNodes[dep]; ok {
				depNode.Dependents = append(depNode.Dependents, stage.Name)
			}
		}
		added++
	}

	appendLinearStage := func(name string, kind StageKind, lane string, deps []string) error {
		stage := Stage{
			Name:         name,
			Kind:         kind,
			Lane:         lane,
			Dependencies: deps,
		}
		stage.Constraints.Manifest = compiled
		asterMeta, err := resolveStageAster(ctx, stage, compiled, opts.Aster)
		if err != nil {
			return err
		}
		stage.Aster = asterMeta
		cacheKey, err := composer.Compose(ctx, CacheComposeRequest{Stage: stage, Ticket: ticket})
		if err != nil {
			return err
		}
		stage.CacheKey = strings.TrimSpace(cacheKey)
		node := &stageNode{Stage: stage, Remaining: len(deps)}
		stageNodes[stage.Name] = node
		stageOrder[stage.Name] = len(*orderedNodes)
		*orderedNodes = append(*orderedNodes, node)
		if node.Remaining == 0 {
			ready = append(ready, node)
		}
		for _, dep := range deps {
			if depNode, ok := stageNodes[dep]; ok {
				depNode.Dependents = append(depNode.Dependents, stage.Name)
			}
		}
		added++
		return nil
	}

	if err := appendLinearStage(buildGateStageName+suffix, StageKindBuildGate, "go-native", []string{mods.StageNameHuman + suffix}); err != nil {
		return nil, 0, err
	}
	if err := appendLinearStage(staticChecksStageName+suffix, StageKindStaticChecks, "go-native", []string{buildGateStageName + suffix}); err != nil {
		return nil, 0, err
	}
	if err := appendLinearStage("test"+suffix, StageKindTest, "go-native", []string{staticChecksStageName + suffix}); err != nil {
		return nil, 0, err
	}

	return ready, added, nil
}

func renameDependencies(deps []string, suffix string) []string {
	if len(deps) == 0 {
		return nil
	}
	renamed := make([]string, len(deps))
	for i, dep := range deps {
		renamed[i] = strings.TrimSpace(dep) + suffix
	}
	return renamed
}

func buildHealingSignals(outcome StageOutcome) mods.AdviceSignals {
	signals := mods.AdviceSignals{}
	if msg := strings.TrimSpace(outcome.Message); msg != "" {
		signals.Errors = append(signals.Errors, msg)
	}
	if outcome.Stage.Metadata.BuildGate != nil {
		for _, finding := range outcome.Stage.Metadata.BuildGate.LogFindings {
			if trimmed := strings.TrimSpace(finding.Message); trimmed != "" {
				signals.Errors = append(signals.Errors, trimmed)
			}
		}
		for _, report := range outcome.Stage.Metadata.BuildGate.StaticChecks {
			for _, failure := range report.Failures {
				if trimmed := strings.TrimSpace(failure.Message); trimmed != "" {
					signals.Errors = append(signals.Errors, trimmed)
				}
			}
		}
	}
	return signals
}

func collectDependentStages(node *stageNode, stageNodes map[string]*stageNode) []string {
	if node == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var visit func(name string)
	visit = func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		n := stageNodes[name]
		if n == nil {
			return
		}
		for _, dep := range n.Dependents {
			visit(dep)
		}
	}
	visit(node.Stage.Name)
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	return result
}
