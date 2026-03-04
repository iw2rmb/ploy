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

type gateTerminalFactory struct{}

func resolveGateExecutionPlan(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
	mappingPath string,
) (gateExecutionPlan, *gateExecutionTerminal) {
	resolver := gatePlanResolver{
		mappingPath: mappingPath,
		terminals:   gateTerminalFactory{},
	}
	return resolver.Resolve(ctx, workspace, spec)
}

type gatePlanResolver struct {
	mappingPath string
	terminals   gateTerminalFactory
}

func (r gatePlanResolver) Resolve(
	ctx context.Context,
	workspace string,
	spec *contracts.StepGateSpec,
) (gateExecutionPlan, *gateExecutionTerminal) {
	obs, detectErr := stackdetect.Detect(ctx, workspace)
	stackCtx, terminal := resolveGateStackContext(ctx, workspace, spec, obs, detectErr, r.mappingPath, r.terminals)
	if terminal != nil {
		return gateExecutionPlan{}, terminal
	}

	image, err := resolveImageForExpectation(r.mappingPath, spec.ImageOverrides, stackCtx.expectation, true)
	if err != nil {
		return gateExecutionPlan{}, r.imageResolutionTerminal(stackCtx, err)
	}

	cmd, prepEnv, err := resolveGateCommand(workspace, stackCtx.language, stackCtx.tool, stackCtx.release, spec.GateProfile, spec.Target)
	if err != nil {
		return gateExecutionPlan{}, r.commandResolutionTerminal(stackCtx, err, spec.EnforceTargetLock, image)
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

func (r gatePlanResolver) imageResolutionTerminal(stackCtx gateStackContext, err error) *gateExecutionTerminal {
	if stackCtx.stackGate != nil {
		code := "STACK_GATE_IMAGE_MAPPING_ERROR"
		prefix := "image mapping error"
		if errors.Is(err, errBuildGateImageRuleMatch) {
			code = "STACK_GATE_NO_IMAGE_RULE"
			prefix = "no matching image rule"
		}
		message := fmt.Sprintf("%s: %s", prefix, err.Error())
		return r.terminals.failure(failureTerminalOpts{
			language:        stackCtx.language,
			stackGate:       stackCtx.stackGate,
			stackGateResult: "unknown",
			message:         message,
			code:            code,
		})
	}

	code := "BUILD_GATE_IMAGE_MAPPING_ERROR"
	if errors.Is(err, errBuildGateImageRuleMatch) {
		code = "BUILD_GATE_NO_IMAGE_RULE"
	}
	return r.terminals.failure(failureTerminalOpts{
		language: stackCtx.language,
		tool:     stackCtx.tool,
		code:     code,
		message:  err.Error(),
	})
}

func (r gatePlanResolver) commandResolutionTerminal(
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
	return r.terminals.failure(failureTerminalOpts{
		language:           stackCtx.language,
		tool:               stackCtx.tool,
		stackGate:          stackCtx.stackGate,
		stackGateResult:    "unknown",
		message:            err.Error(),
		code:               code,
		err:                terminalErr,
		runtimeImage:       runtimeImage,
		reportRuntimeImage: true,
	})
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

func (gateTerminalFactory) failure(opts failureTerminalOpts) *gateExecutionTerminal {
	if opts.stackGate != nil {
		if opts.stackGateResult != "" {
			opts.stackGate.Result = opts.stackGateResult
			opts.stackGate.Reason = opts.message
		}
		if opts.runtimeImage != "" {
			opts.stackGate.RuntimeImage = opts.runtimeImage
		}
		opts.tool = "stack-gate"
	}

	meta := buildGateFailureMetadata(opts.language, opts.tool, opts.code, opts.message, opts.evidence)
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
		runtimeImage:       strings.TrimSpace(opts.runtimeImage),
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
