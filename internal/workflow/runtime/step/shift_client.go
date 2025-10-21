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

// BuildGateRunner executes SHIFT build gate validation.
type BuildGateRunner interface {
	Run(ctx context.Context, spec buildgate.RunSpec) (buildgate.RunResult, error)
}

// BuildGateShiftOptions configures the build gate backed SHIFT client.
type BuildGateShiftOptions struct {
	Runner BuildGateRunner
}

// BuildGateShiftClient adapts the build gate runner to the step.ShiftClient interface.
type BuildGateShiftClient struct {
	runner BuildGateRunner
}

// NewBuildGateShiftClient constructs a SHIFT client backed by the build gate runner.
func NewBuildGateShiftClient(opts BuildGateShiftOptions) (*BuildGateShiftClient, error) {
	if opts.Runner == nil {
		return nil, errors.New("shift: build gate runner required")
	}
	return &BuildGateShiftClient{runner: opts.Runner}, nil
}

// Validate executes the build gate runner for the provided manifest workspace context.
func (c *BuildGateShiftClient) Validate(ctx context.Context, req ShiftRequest) (ShiftResult, error) {
	if c == nil || c.runner == nil {
		return ShiftResult{}, fmt.Errorf("shift: build gate runner not configured")
	}
	spec := buildgate.RunSpec{
		Sandbox: buildgate.SandboxSpec{
			CacheKey: buildSandboxCacheKey(req.Manifest),
			Env:      cloneShiftEnv(req.Manifest.Shift),
		},
	}
	if req.LogArtifact != nil && strings.TrimSpace(req.LogArtifact.CID) != "" {
		spec.LogArtifact = &buildgate.ArtifactReference{
			CID:         strings.TrimSpace(req.LogArtifact.CID),
			Description: fmt.Sprintf("logs for step %s", req.Manifest.ID),
		}
	}

	result, err := c.runner.Run(ctx, spec)
	if err != nil {
		return ShiftResult{}, fmt.Errorf("shift: build gate run: %w", err)
	}

	metadata := result.Metadata
	if metadata.StaticChecks == nil && metadata.LogFindings == nil && metadata.LogDigest == "" {
		metadata = buildgate.Sanitize(result.Metadata)
	}

	report, _ := json.Marshal(struct {
		Metadata buildgate.Metadata `json:"metadata"`
	}{
		Metadata: metadata,
	})

	failures := collectBuildGateFailures(result, metadata)
	if len(failures) == 0 {
		return ShiftResult{
			Passed: true,
			Report: report,
		}, nil
	}

	message := strings.Join(failures, "; ")
	return ShiftResult{
		Passed:  false,
		Message: message,
		Report:  report,
	}, nil
}

func buildSandboxCacheKey(manifest contracts.StepManifest) string {
	id := strings.TrimSpace(manifest.ID)
	profile := ""
	if manifest.Shift != nil {
		profile = strings.TrimSpace(manifest.Shift.Profile)
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

func cloneShiftEnv(spec *contracts.StepShiftSpec) map[string]string {
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
