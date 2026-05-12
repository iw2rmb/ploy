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
	release   string
	stackGate *contracts.StackGateResult
}

type gateStackContext struct {
	expectation contracts.StackExpectation
	language    string
	tool        string
	release     string
	stackGate   *contracts.StackGateResult
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
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	obs, detectErr := stackdetect.Detect(ctx, workspace)
	stackCtx, terminal := resolveGateStackContext(ctx, workspace, spec, obs, detectErr, mappingPath)
	if terminal != nil {
		return gateExecutionPlan{}, terminal
	}

	image, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, stackCtx.expectation, true)
	if err != nil {
		return gateExecutionPlan{}, imageResolutionTerminal(stackCtx, err)
	}

	cmd, prepEnv, err := resolveGateCommand(workspace, stackCtx.language, stackCtx.tool, stackCtx.release, spec.GateProfile, spec.Target)
	if err != nil {
		return gateExecutionPlan{}, commandResolutionTerminal(stackCtx, err, spec.EnforceTargetLock, image)
	}

	if stackCtx.stackGate != nil {
		stackCtx.stackGate.RuntimeImage = image
	}

	return gateExecutionPlan{
		image:     image,
		cmd:       cmd,
		env:       prepEnv,
		language:  stackCtx.language,
		tool:      stackCtx.tool,
		release:   stackCtx.release,
		stackGate: stackCtx.stackGate,
	}, nil
}

func imageResolutionTerminal(stackCtx gateStackContext, err error) *gateExecutionTerminal {
	if stackCtx.stackGate != nil {
		code := "STACK_GATE_IMAGE_MAPPING_ERROR"
		prefix := "image mapping error"
		if errors.Is(err, errBuildGateImageRuleMatch) {
			code = "STACK_GATE_NO_IMAGE_RULE"
			prefix = "no matching image rule"
		}
		return stackGateFailureTerminal(stackCtx.stackGate, stackCtx.language, code,
			fmt.Sprintf("%s: %s", prefix, err.Error()), "", "unknown", "", nil)
	}

	code := "BUILD_GATE_IMAGE_MAPPING_ERROR"
	if errors.Is(err, errBuildGateImageRuleMatch) {
		code = "BUILD_GATE_NO_IMAGE_RULE"
	}
	return buildGateFailureTerminal(stackCtx.language, stackCtx.tool, code, err.Error(), "", nil, "")
}

func commandResolutionTerminal(
	stackCtx gateStackContext,
	err error,
	enforceTargetLock bool,
	runtimeImage string,
) *gateExecutionTerminal {
	unknownCode := "BUILD_GATE_UNKNOWN_TOOL"
	unsupportedCode := "BUILD_GATE_TARGET_UNSUPPORTED"
	if stackCtx.stackGate != nil {
		unknownCode = "STACK_GATE_UNKNOWN"
		unsupportedCode = "STACK_GATE_TARGET_UNSUPPORTED"
	}

	code, terminalErr := mapGateCommandTerminal(err, unsupportedCode, unknownCode, enforceTargetLock)
	if stackCtx.stackGate != nil {
		return stackGateFailureTerminal(stackCtx.stackGate, stackCtx.language, code, err.Error(), "", "unknown", runtimeImage, terminalErr)
	}
	return buildGateFailureTerminal(stackCtx.language, stackCtx.tool, code, err.Error(), "", terminalErr, runtimeImage)
}

func mapGateCommandTerminal(err error, unsupportedCode string, unknownCode string, enforceTargetLock bool) (string, error) {
	if !errors.Is(err, errGateTargetUnsupported) {
		return unknownCode, nil
	}
	if enforceTargetLock {
		return unsupportedCode, fmt.Errorf("%w: %s", ErrRepoCancelled, err.Error())
	}
	return unsupportedCode, nil
}

// buildGateFailureTerminal builds a terminal for non-stack-gate (build-gate) failures.
// runtimeImage may be empty; when non-empty it is recorded on the metadata and
// reported to the runtime-image observer.
func buildGateFailureTerminal(
	language, tool, code, message, evidence string,
	err error,
	runtimeImage string,
) *gateExecutionTerminal {
	meta := buildGateFailureMetadata(language, tool, code, message, evidence)
	runtimeImage = strings.TrimSpace(runtimeImage)
	if runtimeImage != "" {
		meta.RuntimeImage = runtimeImage
	}
	return &gateExecutionTerminal{
		meta:               meta,
		err:                err,
		reportRuntimeImage: runtimeImage != "",
		runtimeImage:       runtimeImage,
	}
}

// stackGateFailureTerminal builds a terminal for stack-gate failures. The
// supplied StackGateResult is mutated to record result+reason+runtimeImage.
// stackGateResult ("unknown" / "mismatch" / "pass") may be empty to leave the
// existing value untouched.
func stackGateFailureTerminal(
	sg *contracts.StackGateResult,
	language, code, message, evidence, stackGateResult, runtimeImage string,
	err error,
) *gateExecutionTerminal {
	runtimeImage = strings.TrimSpace(runtimeImage)
	if sg != nil {
		if stackGateResult != "" {
			sg.Result = stackGateResult
			sg.Reason = message
		}
		if runtimeImage != "" {
			sg.RuntimeImage = runtimeImage
		}
	}

	meta := buildGateFailureMetadata(language, "stack-gate", code, message, evidence)
	if sg != nil {
		meta.StackGate = sg
	}
	if runtimeImage != "" {
		meta.RuntimeImage = runtimeImage
	}
	return &gateExecutionTerminal{
		meta:               meta,
		err:                err,
		reportRuntimeImage: true,
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
	return &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{{
			Language: language,
			Tool:     tool,
			Passed:   false,
		}},
		LogFindings: []contracts.BuildGateLogFinding{{
			Severity: "error",
			Code:     code,
			Message:  message,
			Evidence: evidence,
		}},
	}
}
