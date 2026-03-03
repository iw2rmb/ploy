package handlers

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func resolveFailedGateRecoveryContext(failedJob store.Job) (*contracts.BuildGateRecoveryMetadata, contracts.ModStack, *contracts.StackExpectation) {
	meta := &contracts.BuildGateRecoveryMetadata{
		LoopKind:  contracts.DefaultRecoveryLoopKind().String(),
		ErrorKind: contracts.DefaultRecoveryErrorKind().String(),
	}
	detectedStack := contracts.ModStackUnknown
	var detectedExpectation *contracts.StackExpectation

	if len(failedJob.Meta) == 0 {
		return meta, detectedStack, detectedExpectation
	}

	jobMeta, err := contracts.UnmarshalJobMeta(failedJob.Meta)
	if err != nil {
		slog.Warn("maybeCreateHealingJobs: failed to parse failed gate job meta; defaulting recovery classification",
			"run_id", failedJob.RunID,
			"job_id", failedJob.ID,
			"error", err,
		)
		return meta, detectedStack, detectedExpectation
	}

	if jobMeta.Gate != nil {
		detectedStack = jobMeta.Gate.DetectedStack()
		detectedExpectation = jobMeta.Gate.DetectedStackExpectation()
		if jobMeta.Gate.Recovery != nil {
			meta = cloneRecoveryMetadata(jobMeta.Gate.Recovery)
		}
	}
	if detectedExpectation == nil {
		detectedExpectation = stackExpectationFromModStack(detectedStack)
	}
	if kind, ok := contracts.ParseRecoveryErrorKind(meta.ErrorKind); (!ok || kind == contracts.RecoveryErrorKindUnknown) && jobMeta.Recovery != nil {
		meta = cloneRecoveryMetadata(jobMeta.Recovery)
	}
	if loopKind, ok := contracts.ParseRecoveryLoopKind(meta.LoopKind); ok {
		meta.LoopKind = loopKind.String()
	} else {
		meta.LoopKind = contracts.DefaultRecoveryLoopKind().String()
	}
	if kind, ok := contracts.ParseRecoveryErrorKind(meta.ErrorKind); ok {
		meta.ErrorKind = kind.String()
	} else {
		meta.ErrorKind = contracts.DefaultRecoveryErrorKind().String()
	}
	normalizeRecoveryErrorKind(meta)
	if meta.StrategyID == "" {
		meta.StrategyID = fmt.Sprintf("%s-default", meta.ErrorKind)
	}
	return meta, detectedStack, detectedExpectation
}

func normalizeRecoveryErrorKind(meta *contracts.BuildGateRecoveryMetadata) {
	if meta == nil {
		return
	}
	kind, ok := contracts.ParseRecoveryErrorKind(meta.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return
	}
	if !isDependencyToolchainMismatchReason(meta.Reason) {
		return
	}
	meta.ErrorKind = contracts.RecoveryErrorKindDeps.String()
	strategyID := strings.TrimSpace(meta.StrategyID)
	if strategyID == "" || strategyID == fmt.Sprintf("%s-default", contracts.RecoveryErrorKindInfra.String()) {
		meta.StrategyID = fmt.Sprintf("%s-default", contracts.RecoveryErrorKindDeps.String())
	}
}

func isDependencyToolchainMismatchReason(reason string) bool {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return false
	}
	// Gradle/Groovy classloader failures on JDK-compiled bytecode are dependency/toolchain issues.
	if strings.Contains(reason, "unsupported class file major version") {
		return true
	}
	return false
}

func stackExpectationFromModStack(stack contracts.ModStack) *contracts.StackExpectation {
	switch stack {
	case contracts.ModStackJavaMaven:
		return &contracts.StackExpectation{Language: "java", Tool: "maven"}
	case contracts.ModStackJavaGradle:
		return &contracts.StackExpectation{Language: "java", Tool: "gradle"}
	case contracts.ModStackJava:
		return &contracts.StackExpectation{Language: "java", Tool: "java"}
	default:
		return nil
	}
}
