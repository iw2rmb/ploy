package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/buildgate"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// BuildGateRunner executes Build Gate validation.
type BuildGateRunner interface {
	Run(ctx context.Context, spec buildgate.RunSpec) (buildgate.RunResult, error)
}

// BuildGateClientOptions configures the build gate client.
type BuildGateClientOptions struct {
	Runner BuildGateRunner
}

// BuildGateClient adapts the build gate runner to the step.GateClient interface.
type BuildGateClient struct {
	runner BuildGateRunner
}

// NewBuildGateClient constructs a Build Gate client backed by the build gate runner.
func NewBuildGateClient(opts BuildGateClientOptions) (*BuildGateClient, error) {
	if opts.Runner == nil {
		return nil, errors.New("buildgate: runner required")
	}
	return &BuildGateClient{runner: opts.Runner}, nil
}

// Validate executes the build gate runner for the provided manifest workspace context.
func (c *BuildGateClient) Validate(ctx context.Context, req GateRequest) (GateResult, error) {
	if c == nil || c.runner == nil {
		return GateResult{}, fmt.Errorf("buildgate: runner not configured")
	}
	workspacePath := resolveWorkspacePath(req.Manifest, req.Workspace)
	gs := selectGateSpec(req.Manifest)
	env := cloneGateEnv(gs)
	if env == nil {
		env = make(map[string]string)
	}
	if gs != nil {
		if profile := strings.TrimSpace(gs.Profile); profile != "" {
			env["PLOY_SHIFT_PROFILE"] = profile
		}
	}

	runSpec := buildgate.RunSpec{
		Sandbox: buildgate.SandboxSpec{
			CacheKey:  buildSandboxCacheKey(req.Manifest),
			Env:       env,
			Workspace: workspacePath,
		},
	}
	if req.LogArtifact != nil && strings.TrimSpace(req.LogArtifact.CID) != "" {
		runSpec.LogArtifact = &buildgate.ArtifactReference{
			CID:         strings.TrimSpace(req.LogArtifact.CID),
			Description: fmt.Sprintf("logs for step %s", req.Manifest.ID),
		}
	}
	result, err := c.runner.Run(ctx, runSpec)
	if err != nil {
		return GateResult{}, fmt.Errorf("buildgate: run: %w", err)
	}

	metadata := buildgate.Sanitize(result.Metadata)

	payload := struct {
		Metadata buildgate.Metadata `json:"metadata"`
		Summary  json.RawMessage    `json:"summary,omitempty"`
	}{
		Metadata: metadata,
	}
	if len(result.Report) > 0 {
		payload.Summary = append(payload.Summary, result.Report...)
	}

	report, _ := json.Marshal(payload)

	failures := collectBuildGateFailures(result, metadata)
	if len(failures) == 0 {
		return GateResult{
			Passed: true,
			Report: report,
		}, nil
	}

	message := strings.Join(failures, "; ")
	return GateResult{
		Passed:  false,
		Message: message,
		Report:  report,
	}, nil
}

func buildSandboxCacheKey(manifest contracts.StepManifest) string {
	id := strings.TrimSpace(manifest.ID)
	profile := ""
	if s := selectGateSpec(manifest); s != nil {
		profile = strings.TrimSpace(s.Profile)
	}
	switch {
	case id != "" && profile != "":
		return fmt.Sprintf("%s:%s", id, profile)
	case id != "":
		return id
	case profile != "":
		return profile
	default:
		return ""
	}
}

func cloneGateEnv(spec *contracts.StepGateSpec) map[string]string {
	if spec == nil || len(spec.Env) == 0 {
		return nil
	}
	env := make(map[string]string, len(spec.Env))
	keys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env[key] = spec.Env[key]
	}
	return env
}

func selectGateSpec(manifest contracts.StepManifest) *contracts.StepGateSpec {
	if manifest.Gate != nil {
		return manifest.Gate
	}
	if manifest.Shift != nil {
		// Backward compatibility: map Shift to Gate.
		return &contracts.StepGateSpec{Enabled: manifest.Shift.Enabled, Profile: manifest.Shift.Profile, Env: manifest.Shift.Env}
	}
	return nil
}

func resolveWorkspacePath(manifest contracts.StepManifest, workspace Workspace) string {
	if len(workspace.Inputs) == 0 {
		return ""
	}
	for _, input := range manifest.Inputs {
		if input.Mode == contracts.StepInputModeReadWrite {
			if path := strings.TrimSpace(workspace.Inputs[input.Name]); path != "" {
				return path
			}
		}
	}
	for _, path := range workspace.Inputs {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func collectBuildGateFailures(result buildgate.RunResult, metadata buildgate.Metadata) []string {
	var failures []string
	switch result.Sandbox.Status {
	case buildgate.SandboxStatusFailed, buildgate.SandboxStatusTimedOut:
		reason := strings.TrimSpace(result.Sandbox.FailureReason)
		if reason == "" {
			if result.Sandbox.Status == buildgate.SandboxStatusTimedOut {
				reason = "timeout"
			} else {
				reason = "failed"
			}
		}
		detail := strings.TrimSpace(result.Sandbox.FailureDetail)
		if detail == "" {
			detail = "sandbox execution failed"
		}
		failures = append(failures, fmt.Sprintf("sandbox %s: %s", reason, detail))
	}
	for _, check := range metadata.StaticChecks {
		if check.Passed {
			continue
		}
		desc := strings.TrimSpace(check.Tool)
		lang := strings.TrimSpace(check.Language)
		if desc == "" {
			desc = "static check"
		}
		if lang != "" {
			desc = fmt.Sprintf("%s (%s)", desc, lang)
		}
		if len(check.Failures) == 0 {
			failures = append(failures, fmt.Sprintf("%s failed", desc))
			continue
		}
		top := check.Failures[0]
		message := strings.TrimSpace(top.Message)
		if message == "" {
			message = "reported diagnostics"
		}
		location := strings.TrimSpace(top.File)
		if location != "" && top.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, top.Line)
		}
		if location != "" {
			message = fmt.Sprintf("%s – %s", location, message)
		}
		failures = append(failures, fmt.Sprintf("%s: %s", desc, message))
	}
	return failures
}
