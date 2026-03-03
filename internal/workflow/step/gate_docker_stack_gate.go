package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/stackdetect"
)

type gateExecutionPlan struct {
	image                string
	cmd                  []string
	env                  map[string]string
	language             string
	tool                 string
	release              string
	stackGate            *contracts.StackGateResult
	generatedGateProfile json.RawMessage
}

var errGateTargetUnsupported = errors.New("gate target unsupported")

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

type failureTerminalOpts struct {
	language           string
	tool               string
	code               string
	message            string
	evidence           string
	err                error
	stackGate          *contracts.StackGateResult
	stackGateResult    string
	runtimeImage       string
	reportRuntimeImage bool
}

func newFailureTerminal(o failureTerminalOpts) *gateExecutionTerminal {
	if o.stackGate != nil {
		if o.stackGateResult != "" {
			o.stackGate.Result = o.stackGateResult
			o.stackGate.Reason = o.message
		}
		if o.runtimeImage != "" {
			o.stackGate.RuntimeImage = o.runtimeImage
		}
		o.tool = "stack-gate"
	}
	meta := buildGateFailureMetadata(o.language, o.tool, o.code, o.message, o.evidence)
	if o.stackGate != nil {
		meta.StackGate = o.stackGate
	}
	if o.reportRuntimeImage || o.runtimeImage != "" {
		meta.RuntimeImage = o.runtimeImage
	}
	return newGateExecutionTerminal(meta, o.err, o.runtimeImage, o.reportRuntimeImage)
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
		return resolveStackGateExecutionPlan(ctx, workspace, spec, obs, detectErr, mappingPath)
	}
	return resolveDetectedStackExecutionPlan(ctx, workspace, spec, obs, detectErr, mappingPath)
}

func resolveStackGateExecutionPlan(
	_ context.Context,
	workspace string,
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
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language:           spec.StackGate.Expect.Language,
			stackGate:          sgResult,
			stackGateResult:    "unknown",
			message:            reason,
			code:               "STACK_GATE_UNKNOWN",
			evidence:           evidenceStr,
			runtimeImage:       runtimeImage,
			reportRuntimeImage: true,
		})
	}

	sgResult.Detected = observationToStackExpectation(obs)

	if matched, reason := matchStack(obs, spec.StackGate.Expect); !matched {
		evidenceStr := formatEvidenceForLog(obs.Evidence)
		runtimeImage := resolveStackGateRuntimeImageForTerminal(mappingPath, spec.ImageOverrides, spec.StackGate.Expect)
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language:           spec.StackGate.Expect.Language,
			stackGate:          sgResult,
			stackGateResult:    "mismatch",
			message:            reason,
			code:               "STACK_GATE_MISMATCH",
			evidence:           evidenceStr,
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

	if strings.TrimSpace(spec.StackGate.Expect.Release) == "" {
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language:        language,
			stackGate:       sgResult,
			stackGateResult: "unknown",
			message:         "stack gate expectation missing release; cannot resolve runtime image",
			code:            "STACK_GATE_INVALID_EXPECTATION",
		})
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
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language:        language,
			stackGate:       sgResult,
			stackGateResult: "unknown",
			message:         reason,
			code:            code,
		})
	}

	sgResult.RuntimeImage = image
	cmd, prepEnv, err := resolveGateCommand(workspace, language, tool, spec.StackGate.Expect.Release, spec.GateProfile, spec.Target)
	if err != nil {
		code := "STACK_GATE_UNKNOWN"
		terminalErr := error(nil)
		if errors.Is(err, errGateTargetUnsupported) {
			code = "STACK_GATE_TARGET_UNSUPPORTED"
			if spec.EnforceTargetLock {
				terminalErr = fmt.Errorf("%w: %s", ErrRepoCancelled, err.Error())
			}
		}
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language:           language,
			stackGate:          sgResult,
			stackGateResult:    "unknown",
			message:            err.Error(),
			code:               code,
			err:                terminalErr,
			runtimeImage:       image,
			reportRuntimeImage: true,
		})
	}

	return gateExecutionPlan{
		image:     image,
		cmd:       cmd,
		env:       prepEnv,
		language:  language,
		tool:      tool,
		release:   strings.TrimSpace(spec.StackGate.Expect.Release),
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
			if expectedLanguage != "" &&
				!contracts.StackFieldsMatch(obs.Language, "", "", expectedLanguage, "", "") {
				mismatches = append(mismatches, fmt.Sprintf("language: expected %q, detected %q", expectedLanguage, obs.Language))
			}
			if expectedRelease != "" && obs.Release != nil &&
				!contracts.StackFieldsMatch("", "", *obs.Release, "", "", expectedRelease) {
				mismatches = append(mismatches, fmt.Sprintf("release: expected %q, detected %q", expectedRelease, strings.TrimSpace(*obs.Release)))
			}
			detectedTool := strings.TrimSpace(obs.Tool)
			if expectedTool != "" && detectedTool != "" &&
				!contracts.StackFieldsMatch("", detectedTool, "", "", expectedTool, "") {
				mismatches = append(mismatches, fmt.Sprintf("tool: expected %q, detected %q", expectedTool, detectedTool))
			}
			if len(mismatches) > 0 {
				return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
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
				return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
					tool:    "stackdetect",
					code:    "BUILD_GATE_STACK_DETECT_FAILED",
					message: "stack detection could not determine build tool",
					err:     ErrRepoCancelled,
				})
			}
			return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
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
			return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
				tool:     "stackdetect",
				code:     "BUILD_GATE_STACK_DETECT_FAILED",
				message:  msg,
				evidence: evidenceStr,
				err:      ErrRepoCancelled,
			})
		}

		if stackDetectCfg == nil || !stackDetectCfg.Enabled {
			// Default behavior: fail the gate (not cancelled).
			return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
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

	image, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *exp, true)
	if err != nil {
		code := "BUILD_GATE_IMAGE_MAPPING_ERROR"
		if errors.Is(err, errBuildGateImageRuleMatch) {
			code = "BUILD_GATE_NO_IMAGE_RULE"
		}
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language: language,
			tool:     tool,
			code:     code,
			message:  err.Error(),
		})
	}

	release := ""
	if exp != nil {
		release = exp.Release
	}
	cmd, prepEnv, err := resolveGateCommand(workspace, language, tool, release, spec.GateProfile, spec.Target)
	if err != nil {
		code := "BUILD_GATE_UNKNOWN_TOOL"
		terminalErr := error(nil)
		if errors.Is(err, errGateTargetUnsupported) {
			code = "BUILD_GATE_TARGET_UNSUPPORTED"
			if spec.EnforceTargetLock {
				terminalErr = fmt.Errorf("%w: %s", ErrRepoCancelled, err.Error())
			}
		}
		return gateExecutionPlan{}, newFailureTerminal(failureTerminalOpts{
			language: language,
			tool:     tool,
			code:     code,
			message:  err.Error(),
			err:      terminalErr,
		})
	}

	var generatedGateProfile json.RawMessage
	if spec.AutoBootstrapRepoGateProfile && spec.GateProfile == nil {
		generatedGateProfile, _ = buildAutoGeneratedPreGateProfile(spec.RepoID.String(), workspace, language, tool, release, cmd, prepEnv)
	}

	return gateExecutionPlan{
		image:                image,
		cmd:                  cmd,
		env:                  prepEnv,
		language:             language,
		tool:                 tool,
		release:              strings.TrimSpace(release),
		generatedGateProfile: generatedGateProfile,
	}, nil
}

func buildAutoGeneratedPreGateProfile(
	repoID string,
	workspace string,
	language string,
	tool string,
	release string,
	cmd []string,
	env map[string]string,
) (json.RawMessage, error) {
	repoID = strings.TrimSpace(repoID)
	language = strings.TrimSpace(language)
	tool = strings.TrimSpace(tool)
	release = strings.TrimSpace(release)
	if repoID == "" || language == "" || tool == "" {
		return nil, fmt.Errorf("missing required stack metadata for auto-generated gate_profile")
	}
	shellCommand, ok := commandSliceToShellCommand(cmd)
	if !ok {
		return nil, fmt.Errorf("unsupported command form for auto-generated gate_profile")
	}
	buildCommand := ""
	if buildCmd, err := buildCommandForToolTarget(workspace, tool, contracts.GateProfileTargetBuild); err == nil {
		if buildShell, buildOK := commandSliceToShellCommand(buildCmd); buildOK {
			buildCommand = buildShell
		}
	}

	profile := contracts.GateProfile{
		SchemaVersion: 1,
		RepoID:        repoID,
		RunnerMode:    contracts.PrepRunnerModeSimple,
		Stack: contracts.GateProfileStack{
			Language: language,
			Tool:     tool,
			Release:  release,
		},
		Targets: contracts.GateProfileTargets{
			Active: contracts.GateProfileTargetAllTests,
			Build: &contracts.GateProfileTarget{
				Status:  contracts.PrepTargetStatusNotAttempted,
				Command: buildCommand,
				Env:     map[string]string{},
			},
			Unit: &contracts.GateProfileTarget{
				Status: contracts.PrepTargetStatusNotAttempted,
				Env:    map[string]string{},
			},
			AllTests: &contracts.GateProfileTarget{
				Status:      contracts.PrepTargetStatusPassed,
				Command:     shellCommand,
				Env:         contracts.CopyEnv(env),
				FailureCode: nil,
			},
		},
		Orchestration: contracts.GateProfileOrchestration{
			Pre:  []json.RawMessage{},
			Post: []json.RawMessage{},
		},
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	if _, err := contracts.ParseGateProfileJSON(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func commandSliceToShellCommand(cmd []string) (string, bool) {
	if len(cmd) != 3 {
		return "", false
	}
	if strings.TrimSpace(cmd[0]) != "/bin/sh" {
		return "", false
	}
	switch strings.TrimSpace(cmd[1]) {
	case "-c", "-lc":
		return strings.TrimSpace(cmd[2]), strings.TrimSpace(cmd[2]) != ""
	default:
		return "", false
	}
}

func resolveGateCommand(
	workspace string,
	language string,
	tool string,
	release string,
	prep *contracts.BuildGateProfileOverride,
	target string,
) ([]string, map[string]string, error) {
	wantedTarget := strings.TrimSpace(target)

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
		prepTarget := strings.TrimSpace(prep.Target)
		if wantedTarget == "" || prepTarget == wantedTarget {
			return prep.Command.ToSlice(), contracts.CopyEnv(prep.Env), nil
		}
	}

	if wantedTarget != "" {
		cmd, err := buildCommandForToolTarget(workspace, tool, wantedTarget)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %s", errGateTargetUnsupported, err.Error())
		}
		return cmd, nil, nil
	}

	cmd, err := buildCommandForTool(workspace, tool)
	if err != nil {
		return nil, nil, err
	}
	return cmd, nil, nil
}

func stackMatchesPrepOverride(stack *contracts.GateProfileStack, language, tool, release string) bool {
	if stack == nil {
		return true
	}
	return contracts.StackFieldsMatch(language, tool, release, stack.Language, stack.Tool, stack.Release)
}
