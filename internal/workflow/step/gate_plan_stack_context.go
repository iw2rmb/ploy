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
	terminals gateTerminalFactory,
) (gateStackContext, *gateExecutionTerminal) {
	stackGateMode := spec.StackGate != nil && spec.StackGate.Enabled && spec.StackGate.Expect != nil
	if stackGateMode {
		return resolveStackGateContext(spec, obs, detectErr, mappingPath, terminals)
	}
	return resolveDetectedStackContext(ctx, workspace, spec, obs, detectErr, terminals)
}

func resolveStackGateContext(
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	mappingPath string,
	terminals gateTerminalFactory,
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
		return gateStackContext{}, terminals.failure(failureTerminalOpts{
			language:           expectation.Language,
			stackGate:          sgResult,
			stackGateResult:    "unknown",
			message:            reason,
			code:               "STACK_GATE_UNKNOWN",
			evidence:           evidence,
			runtimeImage:       runtimeImage,
			reportRuntimeImage: true,
		})
	}

	sgResult.Detected = observationToStackExpectation(obs)
	if matched, reason := matchStack(obs, spec.StackGate.Expect); !matched {
		evidence := formatEvidenceForLog(nilSafeEvidence(obs))
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
		return gateStackContext{}, terminals.failure(failureTerminalOpts{
			language:           expectation.Language,
			stackGate:          sgResult,
			stackGateResult:    "mismatch",
			message:            reason,
			code:               "STACK_GATE_MISMATCH",
			evidence:           evidence,
			runtimeImage:       runtimeImage,
			reportRuntimeImage: true,
		})
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
		return gateStackContext{}, terminals.failure(failureTerminalOpts{
			language:        language,
			stackGate:       sgResult,
			stackGateResult: "unknown",
			message:         "stack gate expectation missing release; cannot resolve runtime image",
			code:            "STACK_GATE_INVALID_EXPECTATION",
		})
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
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	obs *stackdetect.Observation,
	detectErr error,
	terminals gateTerminalFactory,
) (gateStackContext, *gateExecutionTerminal) {
	exp := observationToStackExpectation(obs)
	expIncomplete := exp == nil || strings.TrimSpace(exp.Language) == "" || strings.TrimSpace(exp.Release) == ""
	stackDetectCfg := spec.StackDetect
	language := ""
	tool := ""

	if stackDetectCfg != nil && stackDetectCfg.Enabled {
		expectedLanguage := strings.TrimSpace(stackDetectCfg.Language)
		expectedRelease := strings.TrimSpace(stackDetectCfg.Release)
		expectedTool := strings.TrimSpace(stackDetectCfg.Tool)

		if detectErr == nil && !expIncomplete && obs != nil {
			expected := &contracts.StackExpectation{
				Language: expectedLanguage,
				Tool:     expectedTool,
				Release:  expectedRelease,
			}
			if matched, reason := matchStackForStackDetectConfig(obs, expected); !matched {
				return gateStackContext{}, terminals.failure(failureTerminalOpts{
					language: expectedLanguage,
					tool:     "stackdetect",
					code:     "BUILD_GATE_STACK_MISMATCH",
					message:  reason,
					evidence: formatEvidenceForLog(obs.Evidence),
				})
			}
		}

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
			if !stackDetectCfg.Default {
				return gateStackContext{}, terminals.failure(failureTerminalOpts{
					tool:    "stackdetect",
					code:    "BUILD_GATE_STACK_DETECT_FAILED",
					message: "stack detection could not determine build tool",
					err:     ErrRepoCancelled,
				})
			}
			return gateStackContext{}, terminals.failure(failureTerminalOpts{
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
		expIncomplete = exp == nil || strings.TrimSpace(exp.Language) == "" || strings.TrimSpace(exp.Release) == ""
	}

	if detectErr != nil || expIncomplete {
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
			msg = "stack detection produced incomplete result; language and release are required"
		}

		if stackDetectCfg != nil && stackDetectCfg.Enabled && !stackDetectCfg.Default {
			return gateStackContext{}, terminals.failure(failureTerminalOpts{
				tool:     "stackdetect",
				code:     "BUILD_GATE_STACK_DETECT_FAILED",
				message:  msg,
				evidence: evidence,
				err:      ErrRepoCancelled,
			})
		}

		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			return gateStackContext{}, terminals.failure(failureTerminalOpts{
				tool:     "stackdetect",
				code:     "BUILD_GATE_STACK_DETECT_FAILED",
				message:  msg,
				evidence: evidence,
			})
		}
	} else if stackDetectCfg == nil || !stackDetectCfg.Enabled {
		language = strings.TrimSpace(exp.Language)
		if obs != nil {
			tool = strings.TrimSpace(obs.Tool)
		}
	}

	if exp == nil {
		return gateStackContext{}, terminals.failure(failureTerminalOpts{
			tool:    "stackdetect",
			code:    "BUILD_GATE_STACK_DETECT_FAILED",
			message: "stack detection produced incomplete result; language and release are required",
		})
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

func nilSafeEvidence(obs *stackdetect.Observation) []stackdetect.EvidenceItem {
	if obs == nil {
		return nil
	}
	return obs.Evidence
}
