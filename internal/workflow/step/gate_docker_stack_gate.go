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
	env       map[string]string
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

func newGateExecutionTerminal(
	meta *contracts.BuildGateStageMetadata,
	err error,
	runtimeImage string,
	reportRuntimeImage bool,
) *gateExecutionTerminal {
	return &gateExecutionTerminal{
		meta:               meta,
		err:                err,
		reportRuntimeImage: reportRuntimeImage,
		runtimeImage:       runtimeImage,
	}
}

func buildGateFailureMetadata(
	language string,
	tool string,
	code string,
	message string,
	evidence string,
) *contracts.BuildGateStageMetadata {
	meta := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{Language: language, Tool: tool, Passed: false}},
		LogFindings:  []contracts.BuildGateLogFinding{{Severity: "error", Code: code, Message: message, Evidence: evidence}},
	}
	return meta
}

func newDetectedStackFailureTerminal(
	language string,
	tool string,
	code string,
	message string,
	evidence string,
	err error,
) *gateExecutionTerminal {
	meta := buildGateFailureMetadata(language, tool, code, message, evidence)
	return newGateExecutionTerminal(meta, err, "", false)
}

func newStackGateFailureTerminal(
	language string,
	stackGate *contracts.StackGateResult,
	code string,
	message string,
	evidence string,
	runtimeImage string,
	reportRuntimeImage bool,
	err error,
) *gateExecutionTerminal {
	meta := buildGateFailureMetadata(language, "stack-gate", code, message, evidence)
	meta.StackGate = stackGate
	if reportRuntimeImage || runtimeImage != "" {
		meta.RuntimeImage = runtimeImage
	}
	return newGateExecutionTerminal(meta, err, runtimeImage, reportRuntimeImage)
}

func stackGateTerminalWithResult(
	language string,
	stackGate *contracts.StackGateResult,
	result string,
	reason string,
	code string,
	evidence string,
	runtimeImage string,
	reportRuntimeImage bool,
	err error,
) *gateExecutionTerminal {
	stackGate.Result = result
	stackGate.Reason = reason
	if runtimeImage != "" {
		stackGate.RuntimeImage = runtimeImage
	}
	return newStackGateFailureTerminal(language, stackGate, code, reason, evidence, runtimeImage, reportRuntimeImage, err)
}

func resolveStackGateRuntimeImageForTerminal(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	expect *contracts.StackExpectation,
) string {
	img, err := resolveExpectedRuntimeImageForStackGate(mappingPath, overrides, expect)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(img)
}

func resolveGateExecutionPlan(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil
	if stackGateMode {
		return resolveStackGateExecutionPlan(ctx, spec, obs, detectErr, mappingPath)
	}
	return resolveDetectedStackExecutionPlan(ctx, workspace, spec, obs, detectErr, mappingPath)
}

func resolveStackGateExecutionPlan(
	_ context.Context,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	sgResult := &contracts.StackGateResult{
		Enabled:  true,
		Expected: spec.StackGate.Expect,
	}

	if detectErr != nil {
		var detErr *stackdetect.DetectionError
		reason := detectErr.Error()
		var evidenceStr string
		if errors.As(detectErr, &detErr) {
			reason = detErr.Message
			evidenceStr = formatEvidenceForLog(detErr.Evidence)
		}
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
		return gateExecutionPlan{}, stackGateTerminalWithResult(
			spec.StackGate.Expect.Language,
			sgResult,
			"unknown",
			reason,
			"STACK_GATE_UNKNOWN",
			evidenceStr,
			runtimeImage,
			true,
			nil,
		)
	}

	sgResult.Detected = observationToStackExpectation(obs)

	if !stackMatchesExpectation(obs, spec.StackGate.Expect) {
		reason := formatMismatchReason(obs, spec.StackGate.Expect)
		evidenceStr := formatEvidenceForLog(obs.Evidence)
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
		return gateExecutionPlan{}, stackGateTerminalWithResult(
			spec.StackGate.Expect.Language,
			sgResult,
			"mismatch",
			reason,
			"STACK_GATE_MISMATCH",
			evidenceStr,
			runtimeImage,
			true,
			nil,
		)
	}

	sgResult.Result = "pass"

	language := strings.TrimSpace(spec.StackGate.Expect.Language)
	if language == "" {
		language = strings.TrimSpace(obs.Language)
	}
	tool := strings.TrimSpace(obs.Tool)

	if strings.TrimSpace(spec.StackGate.Expect.Release) == "" {
		reason := "stack gate expectation missing release; cannot resolve runtime image"
		return gateExecutionPlan{}, stackGateTerminalWithResult(
			language,
			sgResult,
			"unknown",
			reason,
			"STACK_GATE_INVALID_EXPECTATION",
			"",
			"",
			false,
			nil,
		)
	}

	image, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *spec.StackGate.Expect, true)
	if err != nil {
		code := "STACK_GATE_IMAGE_MAPPING_ERROR"
		prefix := "image mapping error"
		if errors.Is(err, errBuildGateImageRuleMatch) {
			code = "STACK_GATE_NO_IMAGE_RULE"
			prefix = "no matching image rule"
		}
		reason := fmt.Sprintf("%s: %s", prefix, err.Error())
		return gateExecutionPlan{}, stackGateTerminalWithResult(
			language,
			sgResult,
			"unknown",
			reason,
			code,
			"",
			"",
			false,
			nil,
		)
	}

	sgResult.RuntimeImage = image
	cmd, prepEnv, err := resolveGateCommand(language, tool, spec.StackGate.Expect.Release, spec.Prep)
	if err != nil {
		return gateExecutionPlan{}, stackGateTerminalWithResult(
			language,
			sgResult,
			"unknown",
			err.Error(),
			"STACK_GATE_UNKNOWN",
			"",
			image,
			true,
			nil,
		)
	}

	return gateExecutionPlan{
		image:     image,
		cmd:       cmd,
		env:       prepEnv,
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
				return gateExecutionPlan{}, newDetectedStackFailureTerminal(
					expectedLanguage,
					"stackdetect",
					"BUILD_GATE_STACK_MISMATCH",
					"stack mismatch: "+strings.Join(mismatches, "; "),
					formatEvidenceForLog(obs.Evidence),
					nil,
				)
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
				return gateExecutionPlan{}, newDetectedStackFailureTerminal(
					"",
					"stackdetect",
					"BUILD_GATE_STACK_DETECT_FAILED",
					"stack detection could not determine build tool",
					"",
					ErrRepoCancelled,
				)
			}
			return gateExecutionPlan{}, newDetectedStackFailureTerminal(
				"",
				"stackdetect",
				"BUILD_GATE_STACK_DETECT_FAILED",
				"stack detection fallback is enabled but build tool could not be determined (set build_gate.<phase>.stack.tool or ensure workspace has an unambiguous build file)",
				"",
				nil,
			)
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
			return gateExecutionPlan{}, newDetectedStackFailureTerminal(
				"",
				"stackdetect",
				"BUILD_GATE_STACK_DETECT_FAILED",
				msg,
				evidenceStr,
				ErrRepoCancelled,
			)
		}

		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			// Default behavior: fail the gate (not cancelled).
			return gateExecutionPlan{}, newDetectedStackFailureTerminal(
				"",
				"stackdetect",
				"BUILD_GATE_STACK_DETECT_FAILED",
				msg,
				evidenceStr,
				nil,
			)
		}
	} else {
		// No StackDetect: use detected language/tool.
		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			language = exp.Language
			tool = obs.Tool
		}
	}

	image, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *exp, true)
	if err != nil {
		code := "BUILD_GATE_IMAGE_MAPPING_ERROR"
		if errors.Is(err, errBuildGateImageRuleMatch) {
			code = "BUILD_GATE_NO_IMAGE_RULE"
		}
		return gateExecutionPlan{}, newDetectedStackFailureTerminal(
			language,
			tool,
			code,
			err.Error(),
			"",
			nil,
		)
	}

	release := ""
	if exp != nil {
		release = exp.Release
	}
	cmd, prepEnv, err := resolveGateCommand(language, tool, release, spec.Prep)
	if err != nil {
		return gateExecutionPlan{}, newDetectedStackFailureTerminal(
			language,
			tool,
			"BUILD_GATE_UNKNOWN_TOOL",
			err.Error(),
			"",
			nil,
		)
	}

	return gateExecutionPlan{
		image:    image,
		cmd:      cmd,
		env:      prepEnv,
		language: language,
		tool:     tool,
	}, nil
}

func resolveGateCommand(
	language string,
	tool string,
	release string,
	prep *contracts.BuildGatePrepOverride,
) ([]string, map[string]string, error) {
	if prep != nil && !prep.Command.IsEmpty() {
		if prep.Stack != nil {
			if !stackMatchesPrepOverride(prep.Stack, language, tool, release) {
				return nil, nil, fmt.Errorf("prep stack mismatch: expected %s/%s/%s, got %s/%s/%s",
					strings.TrimSpace(prep.Stack.Language),
					strings.TrimSpace(prep.Stack.Tool),
					strings.TrimSpace(prep.Stack.Release),
					strings.TrimSpace(language),
					strings.TrimSpace(tool),
					strings.TrimSpace(release),
				)
			}
		}
		return prep.Command.ToSlice(), copyGateEnv(prep.Env), nil
	}

	cmd, err := buildCommandForTool(tool)
	if err != nil {
		return nil, nil, err
	}
	return cmd, nil, nil
}

func stackMatchesPrepOverride(stack *contracts.PrepProfileStack, language, tool, release string) bool {
	if stack == nil {
		return true
	}
	if strings.TrimSpace(strings.ToLower(stack.Language)) != strings.TrimSpace(strings.ToLower(language)) {
		return false
	}
	if strings.TrimSpace(strings.ToLower(stack.Tool)) != strings.TrimSpace(strings.ToLower(tool)) {
		return false
	}
	wantRelease := strings.TrimSpace(stack.Release)
	if wantRelease == "" {
		return true
	}
	return wantRelease == strings.TrimSpace(release)
}

func copyGateEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}
