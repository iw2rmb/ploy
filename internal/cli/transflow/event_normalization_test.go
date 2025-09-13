package transflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/git/provider"
)

// minimalRunner implements ProductionBranchRunner for event tests
type minimalRunner struct{ rep EventReporter }

func (m *minimalRunner) RenderLLMExecAssets(optionID string) (string, error)  { return "", nil }
func (m *minimalRunner) RenderORWApplyAssets(optionID string) (string, error) { return "", nil }
func (m *minimalRunner) GetGitProvider() provider.GitProvider                 { return nil }
func (m *minimalRunner) GetBuildChecker() BuildCheckerInterface               { return nil }
func (m *minimalRunner) GetWorkspaceDir() string                              { return "" }
func (m *minimalRunner) GetTargetRepo() string                                { return "" }
func (m *minimalRunner) GetEventReporter() EventReporter                      { return m.rep }

// quickSubmitter returns instant success
type quickSubmitter struct{}

func (q *quickSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	return JobResult{JobID: "job-1", Status: "completed", Duration: 10 * time.Millisecond}, nil
}

func TestFanoutEventStepNormalization(t *testing.T) {
	t.Run("human-only", func(t *testing.T) {
		cap := &captureReporter{}
		runner := &minimalRunner{rep: cap}
		submitter := &quickSubmitter{}
		fo := NewFanoutOrchestratorWithRunner(submitter, runner)

		_, _, _ = fo.RunHealingFanout(context.Background(), nil, []BranchSpec{{ID: "b1", Type: "human"}}, 1)

		gotStart := false
		gotFinish := false
		for _, ev := range cap.events {
			if ev.Phase == "fanout" && ev.Step == string(StepTypeHumanStep) {
				if strings.Contains(ev.Message, "finished:") || strings.Contains(ev.Message, "completed") {
					gotFinish = true
				} else {
					gotStart = true
				}
			}
		}
		if !gotStart || !gotFinish {
			t.Fatalf("expected start and finish events with normalized step for human branch; got=%+v", cap.events)
		}
	})

	t.Run("llm-only", func(t *testing.T) {
		cap := &captureReporter{}
		runner := &minimalRunner{rep: cap}
		submitter := &quickSubmitter{}
		fo := NewFanoutOrchestratorWithRunner(submitter, runner)

		_, _, _ = fo.RunHealingFanout(context.Background(), nil, []BranchSpec{{ID: "b2", Type: "llm-exec"}}, 1)

		gotStart := false
		gotFinish := false
		for _, ev := range cap.events {
			if ev.Phase == "fanout" && ev.Step == string(StepTypeLLMExec) {
				if strings.Contains(ev.Message, "finished:") || strings.Contains(ev.Message, "completed") {
					gotFinish = true
				} else {
					gotStart = true
				}
			}
		}
		if !gotStart || !gotFinish {
			t.Fatalf("expected start and finish events with normalized step for llm-exec branch; got=%+v", cap.events)
		}
	})
}

// failSubmitter returns terminal failed status to exercise error-level event path
type failSubmitter struct{}

func (f *failSubmitter) SubmitAndWaitTerminal(ctx context.Context, spec JobSpec) (JobResult, error) {
	return JobResult{JobID: "job-f", Status: "failed", Duration: 10 * time.Millisecond}, nil
}

func TestFanoutEventStepNormalization_Failure(t *testing.T) {
	cap := &captureReporter{}
	runner := &minimalRunner{rep: cap}
	submitter := &failSubmitter{}
	fo := NewFanoutOrchestratorWithRunner(submitter, runner)

	branches := []BranchSpec{
		{ID: "bh", Type: "human", Inputs: map[string]any{}},
	}

	_, _, _ = fo.RunHealingFanout(context.Background(), nil, branches, 1)

	// Expect an error-level finish event with normalized step
	found := false
	for _, ev := range cap.events {
		if ev.Phase == "fanout" && ev.Step == string(StepTypeHumanStep) && ev.Level == "error" && strings.Contains(ev.Message, "finished:") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error finish event with normalized step for human branch")
	}
}
