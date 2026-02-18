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
		return gateExecutionPlan{}, &gateExecutionTerminal{
			meta: &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: spec.StackGate.Expect.Language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				RuntimeImage: runtimeImage,
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_UNKNOWN",
					Message:  sgResult.Reason,
					Evidence: evidenceStr,
				}},
			},
			reportRuntimeImage: true,
			runtimeImage:       runtimeImage,
		}
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
		return gateExecutionPlan{}, &gateExecutionTerminal{
			meta: &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: spec.StackGate.Expect.Language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				RuntimeImage: runtimeImage,
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_MISMATCH",
					Message:  sgResult.Reason,
					Evidence: evidenceStr,
				}},
			},
			reportRuntimeImage: true,
			runtimeImage:       runtimeImage,
		}
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
			return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_INVALID_EXPECTATION",
					Message:  sgResult.Reason,
				}},
			}}
		}

		resolvedImage, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *spec.StackGate.Expect, true)
		if err != nil {
			var mappingErr *buildGateImageMappingError
			if errors.As(err, &mappingErr) {
				sgResult.Result = "unknown"
				sgResult.Reason = fmt.Sprintf("image mapping error: %s", mappingErr.Error())
				return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
					StackGate: sgResult,
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     "stack-gate",
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "STACK_GATE_IMAGE_MAPPING_ERROR",
						Message:  sgResult.Reason,
					}},
				}}
			}
			var ruleErr *buildGateImageRuleMatchError
			if errors.As(err, &ruleErr) {
				sgResult.Result = "unknown"
				sgResult.Reason = fmt.Sprintf("no matching image rule: %s", ruleErr.Error())
				return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
					StackGate: sgResult,
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     "stack-gate",
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "STACK_GATE_NO_IMAGE_RULE",
						Message:  sgResult.Reason,
					}},
				}}
			}
			return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_IMAGE_MAPPING_ERROR",
					Message:  err.Error(),
				}},
			}}
		}
		image = resolvedImage
	}

	sgResult.RuntimeImage = image
	cmd, err := buildCommandForTool(tool)
	if err != nil {
		sgResult.Result = "unknown"
		sgResult.Reason = err.Error()
		return gateExecutionPlan{}, &gateExecutionTerminal{
			meta: &contracts.BuildGateStageMetadata{
				StackGate: sgResult,
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: language,
					Tool:     "stack-gate",
					Passed:   false,
				}},
				RuntimeImage: image,
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "STACK_GATE_UNKNOWN",
					Message:  sgResult.Reason,
				}},
			},
			reportRuntimeImage: true,
			runtimeImage:       image,
		}
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
				return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: expectedLanguage,
						Tool:     "stackdetect",
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "BUILD_GATE_STACK_MISMATCH",
						Message:  "stack mismatch: " + strings.Join(mismatches, "; "),
						Evidence: formatEvidenceForLog(obs.Evidence),
					}},
				}}
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
				meta := &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Tool:   "stackdetect",
						Passed: false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "BUILD_GATE_STACK_DETECT_FAILED",
						Message:  "stack detection could not determine build tool",
					}},
				}
				return gateExecutionPlan{}, &gateExecutionTerminal{meta: meta, err: ErrRepoCancelled}
			}
			return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Tool:   "stackdetect",
					Passed: false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_STACK_DETECT_FAILED",
					Message:  "stack detection fallback is enabled but build tool could not be determined (set build_gate.<phase>.stack.tool or ensure workspace has an unambiguous build file)",
				}},
			}}
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
			meta := &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Tool:   "stackdetect",
					Passed: false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_STACK_DETECT_FAILED",
					Message:  msg,
					Evidence: evidenceStr,
				}},
			}
			return gateExecutionPlan{}, &gateExecutionTerminal{meta: meta, err: ErrRepoCancelled}
		}

		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			// Default behavior: fail the gate (not cancelled).
			return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Tool:   "stackdetect",
					Passed: false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_STACK_DETECT_FAILED",
					Message:  msg,
					Evidence: evidenceStr,
				}},
			}}
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
			var mappingErr *buildGateImageMappingError
			if errors.As(err, &mappingErr) {
				return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
					StaticChecks: []contracts.BuildGateStaticCheckReport{{
						Language: language,
						Tool:     tool,
						Passed:   false,
					}},
					LogFindings: []contracts.BuildGateLogFinding{{
						Severity: "error",
						Code:     "BUILD_GATE_IMAGE_MAPPING_ERROR",
						Message:  mappingErr.Error(),
					}},
				}}
			}
			return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{
					Language: language,
					Tool:     tool,
					Passed:   false,
				}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Code:     "BUILD_GATE_NO_IMAGE_RULE",
					Message:  err.Error(),
				}},
			}}
		}
		image = resolvedImage
	}

	cmd, err := buildCommandForTool(tool)
	if err != nil {
		return gateExecutionPlan{}, &gateExecutionTerminal{meta: &contracts.BuildGateStageMetadata{
			StaticChecks: []contracts.BuildGateStaticCheckReport{{
				Language: language,
				Tool:     tool,
				Passed:   false,
			}},
			LogFindings: []contracts.BuildGateLogFinding{{
				Severity: "error",
				Code:     "BUILD_GATE_UNKNOWN_TOOL",
				Message:  err.Error(),
			}},
		}}
	}

	return gateExecutionPlan{
		image:    image,
		cmd:      cmd,
		language: language,
		tool:     tool,
	}, nil
}

// observationToStackExpectation converts a stackdetect.Observation to a StackExpectation.
func observationToStackExpectation(obs *stackdetect.Observation) *contracts.StackExpectation {
	if obs == nil {
		return nil
	}
	exp := &contracts.StackExpectation{
		Language: obs.Language,
		Tool:     obs.Tool,
	}
	if obs.Release != nil {
		exp.Release = *obs.Release
	}
	return exp
}

// stackMatchesExpectation compares a detected observation against expected values.
// Returns true if all non-empty expected fields match the observation.
func stackMatchesExpectation(obs *stackdetect.Observation, expect *contracts.StackExpectation) bool {
	if expect == nil {
		return true
	}
	if expect.Language != "" && obs.Language != expect.Language {
		return false
	}
	if expect.Tool != "" && obs.Tool != expect.Tool {
		return false
	}
	if expect.Release != "" {
		if obs.Release == nil || *obs.Release != expect.Release {
			return false
		}
	}
	return true
}

// formatMismatchReason generates a human-readable explanation of stack mismatches.
func formatMismatchReason(obs *stackdetect.Observation, expect *contracts.StackExpectation) string {
	var mismatches []string
	if expect.Language != "" && obs.Language != expect.Language {
		mismatches = append(mismatches, fmt.Sprintf("language: expected %q, detected %q", expect.Language, obs.Language))
	}
	if expect.Tool != "" && obs.Tool != expect.Tool {
		mismatches = append(mismatches, fmt.Sprintf("tool: expected %q, detected %q", expect.Tool, obs.Tool))
	}
	if expect.Release != "" {
		detected := "<nil>"
		if obs.Release != nil {
			detected = *obs.Release
		}
		if obs.Release == nil || *obs.Release != expect.Release {
			mismatches = append(mismatches, fmt.Sprintf("release: expected %q, detected %q", expect.Release, detected))
		}
	}
	msg := "stack mismatch: " + strings.Join(mismatches, "; ")

	// Append evidence for debugging.
	if len(obs.Evidence) > 0 {
		msg += "\nevidence:"
		for _, e := range obs.Evidence {
			msg += fmt.Sprintf("\n  - %s: %s = %q", e.Path, e.Key, e.Value)
		}
	}
	return msg
}

// formatEvidenceForLog formats evidence items for the LogFinding.Evidence field.
func formatEvidenceForLog(evidence []stackdetect.EvidenceItem) string {
	if len(evidence) == 0 {
		return ""
	}
	var lines []string
	for _, e := range evidence {
		lines = append(lines, fmt.Sprintf("%s: %s = %q", e.Path, e.Key, e.Value))
	}
	return strings.Join(lines, "\n")
}
