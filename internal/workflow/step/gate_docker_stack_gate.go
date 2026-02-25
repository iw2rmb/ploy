package step

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

type gateExecutionPlan struct {
	image     string
	cmd       []string
	language  string
	tool      string
	stackGate *contracts.StackGateResult
}

type gateExecutionTerminal struct {
	meta               *contracts.BuildGateStageMetadata
	err                error
	reportRuntimeImage bool
	runtimeImage       string
}

type gateTerminalOptions struct {
	language           string
	tool               string
	code               string
	message            string
	evidence           string
	stackGate          *contracts.StackGateResult
	runtimeImage       string
	reportRuntimeImage bool
	err                error
}

func newGateTerminal(opts gateTerminalOptions) *gateExecutionTerminal {
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: opts.language,
			Tool:     opts.tool,
			Passed:   false,
		}},
		LogFindings: []contracts.BuildGateLogFinding{{
			Severity: "error",
			Code:     opts.code,
			Message:  opts.message,
			Evidence: opts.evidence,
		}},
	}
	if opts.stackGate != nil {
		meta.StackGate = opts.stackGate
	}
	if opts.reportRuntimeImage || opts.runtimeImage != "" {
		meta.RuntimeImage = opts.runtimeImage
	}
	return &gateExecutionTerminal{
		meta:               meta,
		err:                opts.err,
		reportRuntimeImage: opts.reportRuntimeImage,
		runtimeImage:       opts.runtimeImage,
	}
}

func resolveGateExecutionPlan(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	envImage string,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil
	if stackGateMode {
		return resolveStackGateExecutionPlan(ctx, spec, obs, detectErr, envImage, mappingPath)
	}
	return resolveDetectedStackExecutionPlan(ctx, workspace, spec, obs, detectErr, envImage, mappingPath)
}

func resolveStackGateExecutionPlan(
	_ context.Context,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	envImage string,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	sgResult := &contracts.StackGateResult{
		Enabled:  true,
		Expected: spec.StackGate.Expect,
	}

	resolveExpectedRuntimeImage := func() (string, error) {
		return resolveExpectedRuntimeImageForStackGate(envImage, mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
	}

	if detectErr != nil {
		var detErr *stackdetect.DetectionError
		var evidenceStr string
		if errors.As(detectErr, &detErr) {
			sgResult.Result = "unknown"
			sgResult.Reason = detErr.Message
			evidenceStr = formatEvidenceForLog(detErr.Evidence)
		} else {
			sgResult.Result = "unknown"
			sgResult.Reason = detectErr.Error()
		}
		var runtimeImage string
		if img, err := resolveExpectedRuntimeImage(); err == nil {
			runtimeImage = strings.TrimSpace(img)
			sgResult.RuntimeImage = runtimeImage
		}
		return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
			language:           spec.StackGate.Expect.Language,
			tool:               "stack-gate",
			code:               "STACK_GATE_UNKNOWN",
			message:            sgResult.Reason,
			evidence:           evidenceStr,
			stackGate:          sgResult,
			runtimeImage:       runtimeImage,
			reportRuntimeImage: true,
		})
	}

	sgResult.Detected = observationToStackExpectation(obs)

	if !stackMatchesExpectation(obs, spec.StackGate.Expect) {
		sgResult.Result = "mismatch"
		sgResult.Reason = formatMismatchReason(obs, spec.StackGate.Expect)
		evidenceStr := formatEvidenceForLog(obs.Evidence)
		var runtimeImage string
		if img, err := resolveExpectedRuntimeImage(); err == nil {
			runtimeImage = strings.TrimSpace(img)
			sgResult.RuntimeImage = runtimeImage
		}
		return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
			language:           spec.StackGate.Expect.Language,
			tool:               "stack-gate",
			code:               "STACK_GATE_MISMATCH",
			message:            sgResult.Reason,
			evidence:           evidenceStr,
			stackGate:          sgResult,
			runtimeImage:       runtimeImage,
			reportRuntimeImage: true,
		})
	}

	sgResult.Result = "pass"

	language := strings.TrimSpace(spec.StackGate.Expect.Language)
	if language == "" {
		language = strings.TrimSpace(obs.Language)
	}
	tool := strings.TrimSpace(obs.Tool)

	image := envImage
	if image == "" {
		if strings.TrimSpace(spec.StackGate.Expect.Release) == "" {
			sgResult.Result = "unknown"
			sgResult.Reason = "stack gate expectation missing release; cannot resolve runtime image"
			return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
				language:  language,
				tool:      "stack-gate",
				code:      "STACK_GATE_INVALID_EXPECTATION",
				message:   sgResult.Reason,
				stackGate: sgResult,
			})
		}

		resolvedImage, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *spec.StackGate.Expect, true)
		if err != nil {
			code := "STACK_GATE_IMAGE_MAPPING_ERROR"
			prefix := "image mapping error"
			if errors.Is(err, errBuildGateImageRuleMatch) {
				code = "STACK_GATE_NO_IMAGE_RULE"
				prefix = "no matching image rule"
			}
			sgResult.Result = "unknown"
			sgResult.Reason = fmt.Sprintf("%s: %s", prefix, err.Error())
			return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
				language:  language,
				tool:      "stack-gate",
				code:      code,
				message:   sgResult.Reason,
				stackGate: sgResult,
			})
		}
		image = resolvedImage
	}

	sgResult.RuntimeImage = image
	cmd, err := buildCommandForTool(tool)
	if err != nil {
		sgResult.Result = "unknown"
		sgResult.Reason = err.Error()
		return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
			language:           language,
			tool:               "stack-gate",
			code:               "STACK_GATE_UNKNOWN",
			message:            sgResult.Reason,
			stackGate:          sgResult,
			runtimeImage:       image,
			reportRuntimeImage: true,
		})
	}

	return gateExecutionPlan{
		image:     image,
		cmd:       cmd,
		language:  language,
		tool:      tool,
		stackGate: sgResult,
	}, nil
}

func resolveDetectedStackExecutionPlan(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	envImage string,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	exp := observationToStackExpectation(obs)
	expIncomplete := exp == nil || strings.TrimSpace(exp.Language) == "" || strings.TrimSpace(exp.Release) == ""
	stackDetectCfg := spec.StackDetect
	language := ""
	tool := ""

	// If StackDetect is enabled for this gate phase, treat it as the expected stack
	// for runtime selection and mismatch validation. Tool is optional and can be
	// inferred from detection when omitted.
	if stackDetectCfg != nil && stackDetectCfg.Enabled {
		expectedLanguage := strings.TrimSpace(stackDetectCfg.Language)
		expectedRelease := strings.TrimSpace(stackDetectCfg.Release)
		expectedTool := strings.TrimSpace(stackDetectCfg.Tool)

		// Validate mismatch only when we have a complete strict detection.
		if detectErr == nil && !expIncomplete && obs != nil {
			var mismatches []string
			if expectedLanguage != "" && strings.TrimSpace(obs.Language) != expectedLanguage {
				mismatches = append(mismatches, fmt.Sprintf("language: expected %q, detected %q", expectedLanguage, obs.Language))
			}
			if expectedRelease != "" && obs.Release != nil && strings.TrimSpace(*obs.Release) != expectedRelease {
				mismatches = append(mismatches, fmt.Sprintf("release: expected %q, detected %q", expectedRelease, strings.TrimSpace(*obs.Release)))
			}
			if expectedTool != "" && strings.TrimSpace(obs.Tool) != "" && strings.TrimSpace(obs.Tool) != expectedTool {
				mismatches = append(mismatches, fmt.Sprintf("tool: expected %q, detected %q", expectedTool, strings.TrimSpace(obs.Tool)))
			}
			if len(mismatches) > 0 {
				return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
					language: expectedLanguage,
					tool:     "stackdetect",
					code:     "BUILD_GATE_STACK_MISMATCH",
					message:  "stack mismatch: " + strings.Join(mismatches, "; "),
					evidence: formatEvidenceForLog(obs.Evidence),
				})
			}
		}

		// Select tool: expected tool wins; otherwise use detected tool (from strict detect),
		// and fall back to tool-only detection when strict detection failed.
		chosenTool := expectedTool
		if chosenTool == "" && obs != nil && strings.TrimSpace(obs.Tool) != "" {
			chosenTool = strings.TrimSpace(obs.Tool)
		}
		if chosenTool == "" && (detectErr != nil || expIncomplete) {
			toolObs, toolErr := stackdetect.DetectTool(ctx, workspace)
			if toolErr == nil && toolObs != nil {
				chosenTool = strings.TrimSpace(toolObs.Tool)
			}
		}

		if chosenTool == "" {
			// Detection couldn't determine tool. Apply default/cancel policy.
			if !stackDetectCfg.Default {
				return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
					tool:    "stackdetect",
					code:    "BUILD_GATE_STACK_DETECT_FAILED",
					message: "stack detection could not determine build tool",
					err:     ErrRepoCancelled,
				})
			}
			return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
				tool:    "stackdetect",
				code:    "BUILD_GATE_STACK_DETECT_FAILED",
				message: "stack detection fallback is enabled but build tool could not be determined (set build_gate.<phase>.stack.tool or ensure workspace has an unambiguous build file)",
			})
		}

		language = expectedLanguage
		tool = chosenTool
		exp = &contracts.StackExpectation{
			Language: expectedLanguage,
			Tool:     chosenTool,
			Release:  expectedRelease,
		}
		// When StackDetect is enabled, we always use the configured language+release
		// for image resolution, regardless of detected release.
		expIncomplete = exp == nil || strings.TrimSpace(exp.Language) == "" || strings.TrimSpace(exp.Release) == ""
	}

	if detectErr != nil || expIncomplete {
		var detErr *stackdetect.DetectionError
		var evidenceStr string
		msg := "stack detection failed"
		if detectErr != nil {
			msg = detectErr.Error()
			if errors.As(detectErr, &detErr) {
				msg = detErr.Message
				evidenceStr = formatEvidenceForLog(detErr.Evidence)
			}
		} else {
			msg = "stack detection produced incomplete result; language and release are required"
		}

		// Policy: when StackDetect is enabled and default=false, treat a detection failure
		// as a repo-level cancellation (no healing / no further execution).
		if stackDetectCfg != nil && stackDetectCfg.Enabled && !stackDetectCfg.Default {
			return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
				tool:     "stackdetect",
				code:     "BUILD_GATE_STACK_DETECT_FAILED",
				message:  msg,
				evidence: evidenceStr,
				err:      ErrRepoCancelled,
			})
		}

		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			// Default behavior: fail the gate (not cancelled).
			return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
				tool:     "stackdetect",
				code:     "BUILD_GATE_STACK_DETECT_FAILED",
				message:  msg,
				evidence: evidenceStr,
			})
		}
	} else {
		// No StackDetect: use detected language/tool.
		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			language = exp.Language
			tool = obs.Tool
		}
	}

	image := envImage
	if image == "" {
		resolvedImage, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *exp, true)
		if err != nil {
			code := "BUILD_GATE_IMAGE_MAPPING_ERROR"
			if errors.Is(err, errBuildGateImageRuleMatch) {
				code = "BUILD_GATE_NO_IMAGE_RULE"
			}
			return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
				language: language,
				tool:     tool,
				code:     code,
				message:  err.Error(),
			})
		}
		image = resolvedImage
	}

	cmd, err := buildCommandForTool(tool)
	if err != nil {
		return gateExecutionPlan{}, newGateTerminal(gateTerminalOptions{
			language: language,
			tool:     tool,
			code:     "BUILD_GATE_UNKNOWN_TOOL",
			message:  err.Error(),
		})
	}

	return gateExecutionPlan{
		image:    image,
		cmd:      cmd,
		language: language,
		tool:     tool,
	}, nil
}
