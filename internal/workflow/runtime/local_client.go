package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

// StepExecutor executes step manifests and returns execution results.
type StepExecutor interface {
	Run(ctx context.Context, req step.Request) (step.Result, error)
}

// LocalStepClientOptions configures the node-local runtime client.
type LocalStepClientOptions struct {
	Runner StepExecutor
}

// LocalStepClient executes workflow stages by invoking the node-local step runner.
type LocalStepClient struct {
	runner      StepExecutor
	mu          sync.Mutex
	invocations []runner.StageInvocation
}

// NewLocalStepClient constructs a node-local workflow runtime client.
func NewLocalStepClient(opts LocalStepClientOptions) (*LocalStepClient, error) {
	if opts.Runner == nil {
		return nil, errors.New("runtime: step runner required")
	}
	return &LocalStepClient{runner: opts.Runner}, nil
}

// ExecuteStage executes the provided stage using the node-local step runner.
func (c *LocalStepClient) ExecuteStage(ctx context.Context, ticket contracts.WorkflowTicket, stage runner.Stage, workspace string) (runner.StageOutcome, error) {
	if c == nil || c.runner == nil {
		return runner.StageOutcome{}, fmt.Errorf("runtime: step runner not configured")
	}
	if stage.StepManifest == nil {
		return runner.StageOutcome{}, fmt.Errorf("runtime: step manifest missing for stage %s", stage.Name)
	}

	request := step.Request{
		Manifest:  *stage.StepManifest,
		Workspace: strings.TrimSpace(workspace),
	}
	result, execErr := c.runner.Run(ctx, request)

	outcome := runner.StageOutcome{
		Stage:  stage,
		RunID:  strings.TrimSpace(result.ContainerID),
		Status: runner.StageStatusCompleted,
	}
	outcome.Artifacts = convertPublishedArtifacts(result)
	outcome.Evidence = buildStageEvidence(result, runner.StageStatusCompleted)

	if meta := parseGateMetadata(result.GateReport.Report); meta != nil {
		outcome.Stage.Metadata.BuildGate = meta
	}

	if execErr != nil {
		if errors.Is(execErr, step.ErrBuildGateFailed) {
			outcome.Status = runner.StageStatusFailed
			outcome.Retryable = false
			outcome.Message = strings.TrimSpace(result.GateReport.Message)
			if outcome.Message == "" {
				outcome.Message = strings.TrimSpace(execErr.Error())
			}
			outcome.Evidence = buildStageEvidence(result, outcome.Status)
			c.recordInvocation(ticket, outcome.Stage, workspace, outcome)
			return outcome, nil
		}
		return runner.StageOutcome{}, execErr
	}

	if !result.GateReport.Passed {
		outcome.Status = runner.StageStatusFailed
		outcome.Retryable = false
		outcome.Message = strings.TrimSpace(result.GateReport.Message)
		outcome.Evidence = buildStageEvidence(result, outcome.Status)
		c.recordInvocation(ticket, outcome.Stage, workspace, outcome)
		return outcome, nil
	}

	if result.ExitCode != 0 {
		outcome.Status = runner.StageStatusFailed
		outcome.Retryable = false
		outcome.Message = fmt.Sprintf("container exited with status %d", result.ExitCode)
		outcome.Evidence = buildStageEvidence(result, outcome.Status)
		c.recordInvocation(ticket, outcome.Stage, workspace, outcome)
		return outcome, nil
	}

	outcome.Evidence = buildStageEvidence(result, outcome.Status)
	c.recordInvocation(ticket, outcome.Stage, workspace, outcome)
	return outcome, nil
}

// CancelWorkflow currently delegates to the legacy cancellation behaviour.
func (c *LocalStepClient) CancelWorkflow(ctx context.Context, req runner.CancelRequest) (runner.CancelResult, error) {
	_ = ctx
	_ = req
	return runner.CancelResult{}, runner.ErrCancellationUnsupported
}

// Invocations returns a snapshot of recorded stage invocations executed via the local client.
func (c *LocalStepClient) Invocations() []runner.StageInvocation {
	c.mu.Lock()
	defer c.mu.Unlock()
	dup := make([]runner.StageInvocation, len(c.invocations))
	copy(dup, c.invocations)
	return dup
}

func (c *LocalStepClient) recordInvocation(ticket contracts.WorkflowTicket, stage runner.Stage, workspace string, outcome runner.StageOutcome) {
	c.mu.Lock()
	defer c.mu.Unlock()
	invocation := runner.StageInvocation{
		TicketID:  strings.TrimSpace(ticket.TicketID),
		Stage:     outcome.Stage,
		Workspace: strings.TrimSpace(workspace),
		RunID:     strings.TrimSpace(outcome.RunID),
		Archive:   outcome.Archive,
		Evidence:  outcome.Evidence,
	}
	if len(outcome.Artifacts) > 0 {
		invocation.Artifacts = cloneArtifacts(outcome.Artifacts)
	}
	c.invocations = append(c.invocations, invocation)
}

func cloneArtifacts(src []runner.Artifact) []runner.Artifact {
	if len(src) == 0 {
		return nil
	}
	dst := make([]runner.Artifact, len(src))
	copy(dst, src)
	return dst
}

func convertPublishedArtifacts(result step.Result) []runner.Artifact {
	var artifacts []runner.Artifact
	if strings.TrimSpace(result.DiffArtifact.CID) != "" {
		artifacts = append(artifacts, runner.Artifact{
			Name:        string(step.ArtifactKindDiff),
			ArtifactCID: strings.TrimSpace(result.DiffArtifact.CID),
			Digest:      strings.TrimSpace(result.DiffArtifact.Digest),
			MediaType:   "application/vnd.ploy.diff+tar",
		})
	}
	if strings.TrimSpace(result.LogArtifact.CID) != "" {
		artifacts = append(artifacts, runner.Artifact{
			Name:        string(step.ArtifactKindLogs),
			ArtifactCID: strings.TrimSpace(result.LogArtifact.CID),
			Digest:      strings.TrimSpace(result.LogArtifact.Digest),
			MediaType:   "text/plain",
		})
	}
	if strings.TrimSpace(result.GateArtifact.CID) != "" {
		artifacts = append(artifacts, runner.Artifact{
			Name:        string(step.ArtifactKindGateReport),
			ArtifactCID: strings.TrimSpace(result.GateArtifact.CID),
			Digest:      strings.TrimSpace(result.GateArtifact.Digest),
			MediaType:   "application/json",
		})
	}
	if len(artifacts) == 0 {
		return nil
	}
	return artifacts
}

func buildStageEvidence(result step.Result, status runner.StageStatus) *runner.StageEvidence {
	exitCode := result.ExitCode
	evidence := &runner.StageEvidence{
		Source:   "local-step-runner",
		Metadata: make(map[string]string),
		Result: map[string]any{
			"gate": map[string]any{
				"passed":  result.GateReport.Passed,
				"message": strings.TrimSpace(result.GateReport.Message),
			},
		},
	}
	evidence.ExitCode = &exitCode
	switch status {
	case runner.StageStatusCompleted:
		evidence.JobState = "completed"
	case runner.StageStatusFailed:
		evidence.JobState = "failed"
	default:
		evidence.JobState = strings.ToLower(string(status))
	}
	if strings.TrimSpace(result.ContainerID) != "" {
		evidence.Metadata["container_id"] = strings.TrimSpace(result.ContainerID)
	}
	evidence.Metadata["retained"] = strconv.FormatBool(result.Retained)
	if strings.TrimSpace(result.RetentionTTL) != "" {
		evidence.Metadata["retention_ttl"] = strings.TrimSpace(result.RetentionTTL)
	}
	if result.GateReport.Duration > 0 {
		evidence.Result["gate"].(map[string]any)["duration_seconds"] = result.GateReport.Duration.Seconds()
	}
	if len(result.GateReport.Report) > 0 {
		evidence.Result["gate"].(map[string]any)["report"] = string(result.GateReport.Report)
	}
	return evidence
}

func parseGateMetadata(report []byte) *runner.StageBuildGateMetadata {
	if len(report) == 0 {
		return nil
	}
	var payload struct {
		Metadata buildgate.Metadata `json:"metadata"`
	}
	if err := json.Unmarshal(report, &payload); err != nil {
		return nil
	}
	sanitized := buildgate.Sanitize(payload.Metadata)
	if sanitized.LogDigest == "" && len(sanitized.StaticChecks) == 0 && len(sanitized.LogFindings) == 0 {
		return nil
	}
	return &runner.StageBuildGateMetadata{
		LogDigest:    sanitized.LogDigest,
		StaticChecks: convertStaticChecks(sanitized.StaticChecks),
		LogFindings:  convertLogFindings(sanitized.LogFindings),
	}
}

func convertStaticChecks(reports []buildgate.StaticCheckReport) []runner.StageStaticCheck {
	if len(reports) == 0 {
		return nil
	}
	result := make([]runner.StageStaticCheck, 0, len(reports))
	for _, report := range reports {
		stageReport := runner.StageStaticCheck{
			Language: strings.TrimSpace(report.Language),
			Tool:     strings.TrimSpace(report.Tool),
			Passed:   report.Passed,
		}
		if len(report.Failures) > 0 {
			stageReport.Failures = make([]runner.StageStaticCheckFailure, 0, len(report.Failures))
			for _, failure := range report.Failures {
				stageReport.Failures = append(stageReport.Failures, runner.StageStaticCheckFailure{
					RuleID:   strings.TrimSpace(failure.RuleID),
					File:     strings.TrimSpace(failure.File),
					Line:     failure.Line,
					Column:   failure.Column,
					Severity: strings.TrimSpace(failure.Severity),
					Message:  strings.TrimSpace(failure.Message),
				})
			}
		}
		result = append(result, stageReport)
	}
	return result
}

func convertLogFindings(findings []buildgate.LogFinding) []runner.StageLogFinding {
	if len(findings) == 0 {
		return nil
	}
	result := make([]runner.StageLogFinding, 0, len(findings))
	for _, finding := range findings {
		result = append(result, runner.StageLogFinding{
			Code:     strings.TrimSpace(finding.Code),
			Severity: strings.TrimSpace(finding.Severity),
			Message:  strings.TrimSpace(finding.Message),
			Evidence: strings.TrimSpace(finding.Evidence),
		})
	}
	return result
}
