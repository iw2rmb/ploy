package handlers

import (
	"fmt"
	"log/slog"

	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func resolveFailedGateRecoveryContext(failedJob store.Job) (*contracts.BuildGateRecoveryMetadata, contracts.ModStack, *contracts.StackExpectation) {
	meta := &contracts.BuildGateRecoveryMetadata{
		LoopKind:  "healing",
		ErrorKind: "unknown",
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
	if meta.ErrorKind == "unknown" && jobMeta.Recovery != nil {
		meta = cloneRecoveryMetadata(jobMeta.Recovery)
	}
	if meta.StrategyID == "" {
		meta.StrategyID = fmt.Sprintf("%s-default", meta.ErrorKind)
	}
	return meta, detectedStack, detectedExpectation
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
