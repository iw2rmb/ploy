package step

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type gateExecutionPlan struct {
	image     string
	language  string
	tool      string
	release   string
	stackGate *contracts.StackGateResult
}

type gateStackContext struct {
	expectation *contracts.StackExpectation
	language    string
	tool        string
	release     string
	stackGate   *contracts.StackGateResult
}

type gateExecutionTerminal struct {
	meta *contracts.BuildGateMetadata
	err  error
}

func resolveGateExecutionPlan(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	stackCtx, terminal := resolveGateStackContext(ctx, workspace, spec, mappingPath)
	if terminal != nil {
		return gateExecutionPlan{}, terminal
	}

	image, err := resolveImageForExpectation(mappingPath, spec.ImageOverrides, *stackCtx.expectation)
	if err != nil {
		return gateExecutionPlan{}, imageResolutionTerminal(stackCtx, err)
	}

	if stackCtx.stackGate != nil {
		stackCtx.stackGate.RuntimeImage = image
	}

	return gateExecutionPlan{
		image:     image,
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
		if errors.Is(err, errImageRuleMatch) {
			code = "STACK_GATE_NO_IMAGE_RULE"
			prefix = "no matching image rule"
		}
		return stackGateFailureTerminal(stackCtx.stackGate, stackCtx.language, code,
			fmt.Sprintf("%s: %s", prefix, err.Error()), "", "unknown", "")
	}

	code := "BUILD_GATE_IMAGE_MAPPING_ERROR"
	if errors.Is(err, errImageRuleMatch) {
		code = "BUILD_GATE_NO_IMAGE_RULE"
	}
	return gateFailureTerminal(stackCtx.language, stackCtx.tool, code, err.Error(), "",
		gateInternalError(code, err.Error()))
}

func gateInternalError(code string, message string) error {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		return errors.New(message)
	}
	if message == "" {
		return errors.New(code)
	}
	return fmt.Errorf("%s: %s", code, message)
}

// gateFailureTerminal builds a terminal for non-stack-gate (build-gate) failures.
func gateFailureTerminal(
	language, tool, code, message, evidence string,
	err error,
) *gateExecutionTerminal {
	return &gateExecutionTerminal{
		meta: gateFailureMetadata(language, tool, code, message, evidence),
		err:  err,
	}
}

// stackGateFailureTerminal builds a terminal for stack-gate failures. It
// records stackGateResult ("unknown" / "mismatch"), the reason, and the
// runtime image on sg. The terminal carries no error: the failure reaches the
// caller only through meta.
func stackGateFailureTerminal(
	sg *contracts.StackGateResult,
	language, code, message, evidence, stackGateResult, runtimeImage string,
) *gateExecutionTerminal {
	runtimeImage = strings.TrimSpace(runtimeImage)
	sg.Result = stackGateResult
	sg.Reason = message
	if runtimeImage != "" {
		sg.RuntimeImage = runtimeImage
	}

	meta := gateFailureMetadata(language, "stack-gate", code, message, evidence)
	meta.StackGate = sg
	if runtimeImage != "" {
		meta.RuntimeImage = runtimeImage
	}
	return &gateExecutionTerminal{meta: meta}
}

func gateFailureMetadata(
	language string,
	tool string,
	code string,
	message string,
	evidence string,
) *contracts.BuildGateMetadata {
	return &contracts.BuildGateMetadata{
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
