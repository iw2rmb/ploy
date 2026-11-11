package step

import (
	"context"
	"errors"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

// resolveGateStackContext selects the stack policy for the gate and resolves
// the stack it runs with. Forced stack-detect mode skips workspace detection.
//
// spec.StackDetect comes from a validated MigSpec (contracts
// validateBuildGateStackConfig), so each mode receives the fields it requires.
func resolveGateStackContext(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	mappingPath string,
) (gateStackContext, *gateExecutionTerminal) {
	if spec.StackGatePhase != nil && spec.StackGatePhase.Enabled && spec.StackGatePhase.Expect != nil {
		obs, detectErr := stackdetect.Detect(ctx, workspace)
		return resolveStackGateContext(spec, obs, detectErr, mappingPath)
	}
	if stackDetectMode(spec.Stack) == contracts.BuildGateStackModeForced {
		return resolveForcedStackDetectContext(ctx, workspace, spec.Stack)
	}
	obs, detectErr := stackdetect.Detect(ctx, workspace)
	return resolveDetectedStackContext(spec.Stack, obs, detectErr)
}

func resolveStackGateContext(
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	mappingPath string,
) (gateStackContext, *gateExecutionTerminal) {
	expectation := contracts.NormalizeStackExpectation(spec.StackGatePhase.Expect)
	sgResult := &contracts.StackGateResult{
		Enabled:  true,
		Expected: expectation,
	}
	if expectation == nil {
		return gateStackContext{}, stackGateFailureTerminal(sgResult, "",
			"STACK_GATE_INVALID_EXPECTATION", "stack gate expectation is empty",
			"", "unknown", "")
	}

	if detectErr != nil {
		var detErr *stackdetect.DetectionError
		reason := detectErr.Error()
		var evidence string
		if errors.As(detectErr, &detErr) {
			reason = detErr.Message
			evidence = formatEvidenceForLog(detErr.Evidence)
		}
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, expectation)
		return gateStackContext{}, stackGateFailureTerminal(sgResult, expectation.Language,
			"STACK_GATE_UNKNOWN", reason, evidence, "unknown", runtimeImage)
	}

	sgResult.Detected = observationToStackExpectation(obs)
	if matched, reason := matchStack(obs, expectation); !matched {
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, expectation)
		return gateStackContext{}, stackGateFailureTerminal(sgResult, expectation.Language,
			"STACK_GATE_MISMATCH", reason, formatEvidenceForLog(obs.Evidence), "mismatch", runtimeImage)
	}

	language := expectation.Language
	if language == "" {
		language = strings.TrimSpace(obs.Language)
	}
	if expectation.Release == "" {
		return gateStackContext{}, stackGateFailureTerminal(sgResult, language,
			"STACK_GATE_INVALID_EXPECTATION",
			"stack gate expectation missing release; cannot resolve runtime image",
			"", "unknown", "")
	}

	sgResult.Result = "pass"
	return gateStackContext{
		expectation: expectation,
		language:    language,
		tool:        strings.TrimSpace(obs.Tool),
		release:     expectation.Release,
		stackGate:   sgResult,
	}, nil
}

func resolveDetectedStackContext(
	stackDetectCfg *contracts.BuildGateStackConfig,
	obs *stackdetect.Observation,
	detectErr error,
) (gateStackContext, *gateExecutionTerminal) {
	switch stackDetectMode(stackDetectCfg) {
	case contracts.BuildGateStackModeStrict:
		return resolveStrictStackDetectContext(stackDetectCfg, obs, detectErr)
	case contracts.BuildGateStackModeFallback:
		return resolveFallbackStackDetectContext(stackDetectCfg, obs, detectErr)
	}

	exp := observationToStackExpectation(obs)
	if detectErr != nil || exp == nil || exp.Language == "" || exp.Release == "" {
		return gateStackContext{}, stackDetectionFailureTerminal(detectErr,
			"stack detection produced incomplete result; language and release are required")
	}
	return stackContextFromExpectation(exp), nil
}

func resolveForcedStackDetectContext(
	ctx context.Context,
	workspace string,
	stackDetectCfg *contracts.BuildGateStackConfig,
) (gateStackContext, *gateExecutionTerminal) {
	expected := configuredStackExpectation(stackDetectCfg)
	if expected.Tool == "" {
		return resolveForcedStackDetectToolContext(ctx, workspace, expected)
	}
	return stackContextFromExpectation(expected), nil
}

func resolveForcedStackDetectToolContext(
	ctx context.Context,
	workspace string,
	expected *contracts.StackExpectation,
) (gateStackContext, *gateExecutionTerminal) {
	obs, detectErr := stackdetect.DetectTool(ctx, workspace)
	if detectErr != nil {
		return gateStackContext{}, stackDetectionFailureTerminal(detectErr,
			"stack tool detection failed")
	}
	if obs == nil || strings.TrimSpace(obs.Language) == "" || strings.TrimSpace(obs.Tool) == "" {
		return gateStackContext{}, stackDetectionFailureTerminal(nil,
			"stack tool detection produced incomplete result; language and tool are required")
	}
	if got, want := strings.TrimSpace(obs.Language), expected.Language; got != want {
		reason := "language mismatch: detected " + got + ", expected " + want
		return gateStackContext{}, gateFailureTerminal(want, "stackdetect",
			"BUILD_GATE_STACK_MISMATCH", reason, formatEvidenceForLog(obs.Evidence), nil)
	}
	return stackContextFromExpectation(&contracts.StackExpectation{
		Language: expected.Language,
		Tool:     strings.TrimSpace(obs.Tool),
		Release:  expected.Release,
	}), nil
}

func resolveStrictStackDetectContext(
	stackDetectCfg *contracts.BuildGateStackConfig,
	obs *stackdetect.Observation,
	detectErr error,
) (gateStackContext, *gateExecutionTerminal) {
	if detectErr != nil || !observationComplete(obs) {
		return gateStackContext{}, stackDetectionFailureTerminal(detectErr,
			"stack detection produced incomplete result; language, tool, and release are required")
	}
	expected := configuredStackExpectation(stackDetectCfg)
	if matched, reason := matchStack(obs, expected); !matched {
		return gateStackContext{}, gateFailureTerminal(expected.Language, "stackdetect",
			"BUILD_GATE_STACK_MISMATCH", reason, formatEvidenceForLog(obs.Evidence), nil)
	}
	return stackContextFromExpectation(observationToStackExpectation(obs)), nil
}

func resolveFallbackStackDetectContext(
	stackDetectCfg *contracts.BuildGateStackConfig,
	obs *stackdetect.Observation,
	detectErr error,
) (gateStackContext, *gateExecutionTerminal) {
	if detectErr != nil || !observationComplete(obs) {
		return stackContextFromExpectation(configuredStackExpectation(stackDetectCfg)), nil
	}
	return stackContextFromExpectation(observationToStackExpectation(obs)), nil
}

func stackContextFromExpectation(exp *contracts.StackExpectation) gateStackContext {
	return gateStackContext{
		expectation: exp,
		language:    exp.Language,
		tool:        exp.Tool,
		release:     exp.Release,
	}
}

func stackDetectMode(stackDetectCfg *contracts.BuildGateStackConfig) contracts.BuildGateStackMode {
	if stackDetectCfg == nil {
		return ""
	}
	return contracts.BuildGateStackMode(strings.TrimSpace(string(stackDetectCfg.Mode)))
}

func configuredStackExpectation(stackDetectCfg *contracts.BuildGateStackConfig) *contracts.StackExpectation {
	return contracts.NormalizeStackExpectation(&contracts.StackExpectation{
		Language: stackDetectCfg.Language,
		Tool:     stackDetectCfg.Tool,
		Release:  stackDetectCfg.Release,
	})
}

func observationComplete(obs *stackdetect.Observation) bool {
	exp := observationToStackExpectation(obs)
	return exp != nil && exp.Language != "" && exp.Tool != "" && exp.Release != ""
}

func stackDetectionFailureTerminal(detectErr error, incompleteMsg string) *gateExecutionTerminal {
	msg := incompleteMsg
	evidence := ""
	if detectErr != nil {
		msg = detectErr.Error()
		var detErr *stackdetect.DetectionError
		if errors.As(detectErr, &detErr) {
			msg = detErr.Message
			evidence = formatEvidenceForLog(detErr.Evidence)
		}
	}
	code := "BUILD_GATE_STACK_DETECT_FAILED"
	return gateFailureTerminal("", "stackdetect",
		code, msg, evidence, gateInternalError(code, msg))
}

// resolveStackGateRuntimeImageForTerminal returns the image the stack gate
// would have run, for reporting on a failed stack gate. It returns "" when the
// expectation lacks a release or no image resolves.
func resolveStackGateRuntimeImageForTerminal(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	expect *contracts.StackExpectation,
) string {
	if expect.Release == "" {
		return ""
	}
	image, err := resolveImageForExpectation(mappingPath, overrides, *expect)
	if err != nil {
		return ""
	}
	return image
}
