package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/gateprofile"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func evaluateAndAttachInfraCandidate(
	ctx context.Context,
	bp *blobpersist.Service,
	runID domaintypes.RunID,
	failedJob store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
	detectedExpectation *contracts.StackExpectation,
	meta *contracts.BuildGateRecoveryMetadata,
) {
	if meta == nil {
		return
	}
	candidatePromoted := false
	meta.CandidatePromoted = &candidatePromoted
	path := strings.TrimSpace(meta.CandidateArtifactPath)
	if path == "" {
		path = contracts.GateProfileCandidateArtifactPath
		meta.CandidateArtifactPath = path
	}
	prevHeal := resolvePreviousHealJob(failedJob, jobsByID)
	if prevHeal == nil {
		meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusMissing
		meta.CandidateValidationError = "candidate artifact unavailable: no previous heal job found"
		return
	}

	raw, err := loadRecoveryArtifact(ctx, bp, runID, prevHeal.ID, path)
	if err != nil {
		switch {
		case errors.Is(err, blobpersist.ErrRecoveryArtifactNotFound):
			meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusMissing
		case errors.Is(err, blobpersist.ErrRecoveryArtifactUnreadable):
			meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusUnavailable
		default:
			meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusInvalid
		}
		meta.CandidateValidationError = err.Error()
		return
	}
	if err := contracts.ValidateGateProfileJSONForSchema(raw, contracts.GateProfileCandidateSchemaID); err != nil {
		meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusInvalid
		meta.CandidateValidationError = err.Error()
		return
	}
	profile, err := contracts.ParseGateProfileJSON(raw)
	if err != nil {
		meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusInvalid
		meta.CandidateValidationError = err.Error()
		return
	}
	if !candidateMatchesDetectedStack(profile, detectedExpectation) {
		meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusInvalid
		meta.CandidateValidationError = "gate_profile stack does not match detected stack"
		return
	}
	meta.CandidateValidationStatus = contracts.RecoveryCandidateStatusValid
	meta.CandidateValidationError = ""
	meta.CandidateGateProfile = append([]byte(nil), raw...)
}

func candidateMatchesDetectedStack(profile *contracts.GateProfile, detected *contracts.StackExpectation) bool {
	if profile == nil || detected == nil {
		return false
	}
	return gateprofile.StackMatches(profile, detected.Language, detected.Tool, detected.Release)
}

func resolvePreviousHealJob(
	failedJob store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
) *store.Job {
	prev := lifecycle.RecoveryChainPredecessor(failedJob.ID, jobsByID)
	if prev == nil {
		return nil
	}
	if domaintypes.JobType(prev.JobType) != domaintypes.JobTypeHeal {
		return nil
	}
	return prev
}

func loadRecoveryArtifact(
	ctx context.Context,
	bp *blobpersist.Service,
	runID domaintypes.RunID,
	healJobID domaintypes.JobID,
	artifactPath string,
) ([]byte, error) {
	if bp == nil {
		return nil, fmt.Errorf("load recovery artifact: blobpersist service is required")
	}
	return bp.LoadRecoveryArtifact(ctx, runID, healJobID, artifactPath)
}
