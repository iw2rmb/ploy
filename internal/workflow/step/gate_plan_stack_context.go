package step

import (
	"context"
	"errors"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

func resolveGateStackContext(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	mappingPath string,
) (gateStackContext, *gateExecutionTerminal) {
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil
	if stackGateMode {
		return resolveStackGateContext(spec, obs, detectErr, mappingPath)
	}
	return resolveDetectedStackContext(ctx, workspace, spec, obs, detectErr)
}

func useForcedStackDetect(spec *contracts.StepGateSpec) bool {
	if spec == nil {
		return false
	}
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil
	return !stackGateMode && stackDetectMode(spec.StackDetect) == contracts.BuildGateStackModeForced
}

func resolveStackGateContext(
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	mappingPath string,
) (gateStackContext, *gateExecutionTerminal) {
	expectation := normalizeStackExpectation(spec.StackGate.Expect)
	sgResult := &contracts.StackGateResult{
		Enabled:  true,
		Expected: spec.StackGate.Expect,
	}

	if detectErr != nil {
		var detErr *stackdetect.DetectionError
		reason := detectErr.Error()
		var evidence string
		if errors.As(detectErr, &detErr) {
			reason = detErr.Message
			evidence = formatEvidenceForLog(detErr.Evidence)
		}
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
		return gateStackContext{}, stackGateFailureTerminal(sgResult, expectation.Language,
			"STACK_GATE_UNKNOWN", reason, evidence, "unknown", runtimeImage, nil)
	}

	sgResult.Detected = observationToStackExpectation(obs)
	if matched, reason := matchStack(obs, spec.StackGate.Expect); !matched {
		var evidenceItems []stackdetect.EvidenceItem
		if obs != nil {
			evidenceItems = obs.Evidence
		}
		evidence := formatEvidenceForLog(evidenceItems)
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
		return gateStackContext{}, stackGateFailureTerminal(sgResult, expectation.Language,
			"STACK_GATE_MISMATCH", reason, evidence, "mismatch", runtimeImage, nil)
	}

	language := expectation.Language
	if language == "" && obs != nil {
		language = strings.TrimSpace(obs.Language)
	}
	tool := ""
	if obs != nil {
		tool = strings.TrimSpace(obs.Tool)
	}
	release := expectation.Release
	if release == "" {
		return gateStackContext{}, stackGateFailureTerminal(sgResult, language,
			"STACK_GATE_INVALID_EXPECTATION",
			"stack gate expectation missing release; cannot resolve runtime image",
			"", "unknown", "", nil)
	}

	sgResult.Result = "pass"
	return gateStackContext{
		expectation: expectation,
		language:    language,
		tool:        tool,
		release:     release,
		stackGate:   sgResult,
	}, nil
}

func resolveDetectedStackContext(
	_ context.Context,
	_ string,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
) (gateStackContext, *gateExecutionTerminal) {
	switch stackDetectMode(spec.StackDetect) {
	case contracts.BuildGateStackModeForced:
		return resolveForcedStackDetectContext(spec.StackDetect)
	case contracts.BuildGateStackModeStrict:
		return resolveStrictStackDetectContext(spec.StackDetect, obs, detectErr)
	case contracts.BuildGateStackModeFallback:
		return resolveFallbackStackDetectContext(spec.StackDetect, obs, detectErr)
	}

	exp := observationToStackExpectation(obs)
	expIncomplete := exp == nil || strings.TrimSpace(exp.Language) == "" || strings.TrimSpace(exp.Release) == ""
	language := ""
	tool := ""

	if detectErr != nil || expIncomplete {
		return gateStackContext{}, stackDetectionFailureTerminal(detectErr,
			"stack detection produced incomplete result; language and release are required")
	}

	language = strings.TrimSpace(exp.Language)
	if obs != nil {
		tool = strings.TrimSpace(obs.Tool)
	}

	if exp == nil {
		code := "BUILD_GATE_STACK_DETECT_FAILED"
		msg := "stack detection produced incomplete result; language and release are required"
		return gateStackContext{}, gateFailureTerminal("", "stackdetect",
			code,
			msg,
			"", gateInternalError(code, msg), "")
	}

	normalized := normalizeStackExpectation(exp)
	if language == "" {
		language = normalized.Language
	}
	if tool == "" {
		tool = normalized.Tool
	}

	return gateStackContext{
		expectation: normalized,
		language:    language,
		tool:        tool,
		release:     normalized.Release,
	}, nil
}

func resolveForcedStackDetectContext(stackDetectCfg *contracts.BuildGateStackConfig) (gateStackContext, *gateExecutionTerminal) {
	expected := configuredStackExpectation(stackDetectCfg)
	if !stackExpectationComplete(expected) {
		return gateStackContext{}, gateStackConfigTerminal("build gate stack mode requires language, tool, and release")
	}
	normalized := normalizeStackExpectation(expected)
	return gateStackContext{
		expectation: normalized,
		language:    normalized.Language,
		tool:        normalized.Tool,
		release:     normalized.Release,
	}, nil
}

func resolveStrictStackDetectContext(
	stackDetectCfg *contracts.BuildGateStackConfig,
	obs *stackdetect.Observation,
	detectErr error,
) (gateStackContext, *gateExecutionTerminal) {
	expected := configuredStackExpectation(stackDetectCfg)
	if expected == nil || expected.IsEmpty() {
		return gateStackContext{}, gateStackConfigTerminal("strict build gate stack mode requires language, tool, or release")
	}
	if detectErr != nil || !observationComplete(obs) {
		return gateStackContext{}, stackDetectionFailureTerminal(detectErr,
			"stack detection produced incomplete result; language, tool, and release are required")
	}
	if matched, reason := matchStackWithOptions(obs, expected, stackMatchOptions{}); !matched {
		return gateStackContext{}, gateFailureTerminal(expected.Language, "stackdetect",
			"BUILD_GATE_STACK_MISMATCH", reason, formatEvidenceForLog(obs.Evidence), nil, "")
	}
	normalized := normalizeStackExpectation(observationToStackExpectation(obs))
	return gateStackContext{
		expectation: normalized,
		language:    normalized.Language,
		tool:        normalized.Tool,
		release:     normalized.Release,
	}, nil
}

func resolveFallbackStackDetectContext(
	stackDetectCfg *contracts.BuildGateStackConfig,
	obs *stackdetect.Observation,
	detectErr error,
) (gateStackContext, *gateExecutionTerminal) {
	expected := configuredStackExpectation(stackDetectCfg)
	if !stackExpectationComplete(expected) {
		return gateStackContext{}, gateStackConfigTerminal("build gate stack mode requires language, tool, and release")
	}
	if detectErr != nil || !observationComplete(obs) {
		normalized := normalizeStackExpectation(expected)
		return gateStackContext{
			expectation: normalized,
			language:    normalized.Language,
			tool:        normalized.Tool,
			release:     normalized.Release,
		}, nil
	}
	detected := normalizeStackExpectation(observationToStackExpectation(obs))
	return gateStackContext{
		expectation: detected,
		language:    detected.Language,
		tool:        detected.Tool,
		release:     detected.Release,
	}, nil
}

func stackDetectMode(stackDetectCfg *contracts.BuildGateStackConfig) contracts.BuildGateStackMode {
	if stackDetectCfg == nil {
		return ""
	}
	return contracts.BuildGateStackMode(strings.TrimSpace(string(stackDetectCfg.Mode)))
}

func configuredStackExpectation(stackDetectCfg *contracts.BuildGateStackConfig) *contracts.StackExpectation {
	if stackDetectCfg == nil {
		return nil
	}
	return &contracts.StackExpectation{
		Language: strings.TrimSpace(stackDetectCfg.Language),
		Tool:     strings.TrimSpace(stackDetectCfg.Tool),
		Release:  strings.TrimSpace(stackDetectCfg.Release),
	}
}

func observationComplete(obs *stackdetect.Observation) bool {
	exp := observationToStackExpectation(obs)
	return stackExpectationComplete(exp)
}

func stackExpectationComplete(exp *contracts.StackExpectation) bool {
	return exp != nil &&
		strings.TrimSpace(exp.Language) != "" &&
		strings.TrimSpace(exp.Tool) != "" &&
		strings.TrimSpace(exp.Release) != ""
}

func stackDetectionFailureTerminal(detectErr error, incompleteMsg string) *gateExecutionTerminal {
	var detErr *stackdetect.DetectionError
	msg := "stack detection failed"
	evidence := ""
	if detectErr != nil {
		msg = detectErr.Error()
		if errors.As(detectErr, &detErr) {
			msg = detErr.Message
			evidence = formatEvidenceForLog(detErr.Evidence)
		}
	} else {
		msg = incompleteMsg
	}
	code := "BUILD_GATE_STACK_DETECT_FAILED"
	return gateFailureTerminal("", "stackdetect",
		code, msg, evidence, gateInternalError(code, msg), "")
}

func gateStackConfigTerminal(msg string) *gateExecutionTerminal {
	code := "BUILD_GATE_STACK_CONFIG_INVALID"
	return gateFailureTerminal("", "stackdetect",
		code, msg, "", gateInternalError(code, msg), "")
}

func resolveStackGateRuntimeImageForTerminal(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	expect *contracts.StackExpectation,
) string {
	image, err := resolveExpectedRuntimeImageForStackGate(mappingPath, overrides, expect)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(image)
}

func normalizeStackExpectation(expect *contracts.StackExpectation) contracts.StackExpectation {
	if expect == nil {
		return contracts.StackExpectation{}
	}
	return contracts.StackExpectation{
		Language: strings.TrimSpace(expect.Language),
		Tool:     strings.TrimSpace(expect.Tool),
		Release:  strings.TrimSpace(expect.Release),
	}
}
