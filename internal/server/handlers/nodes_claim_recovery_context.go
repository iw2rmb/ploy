package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func buildRecoveryClaimContext(
	ctx context.Context,
	st store.Store,
	runID domaintypes.RunID,
	job store.Job,
	jobType domaintypes.JobType,
) (*contracts.RecoveryClaimContext, error) {
	if jobType != domaintypes.JobTypeHeal && jobType != domaintypes.JobTypeReGate {
		return nil, nil
	}
	if len(job.Meta) == 0 {
		return nil, nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || jobMeta.Recovery == nil {
		return nil, nil
	}

	recovery := jobMeta.Recovery
	kind, ok := contracts.ParseRecoveryErrorKind(recovery.ErrorKind)
	if !ok {
		kind = contracts.DefaultRecoveryErrorKind()
	}
	selectedKind := kind.String()
	ctxPayload := &contracts.RecoveryClaimContext{
		LoopKind:          strings.TrimSpace(recovery.LoopKind),
		SelectedErrorKind: selectedKind,
		Expectations:      cloneRawJSON(recovery.Expectations),
	}
	if kind == contracts.RecoveryErrorKindDeps && recovery.DepsBumps != nil {
		ctxPayload.DepsBumps = cloneDepsBumpsMap(recovery.DepsBumps)
	}
	if loopKind, ok := contracts.ParseRecoveryLoopKind(ctxPayload.LoopKind); ok {
		ctxPayload.LoopKind = loopKind.String()
	} else {
		ctxPayload.LoopKind = contracts.DefaultRecoveryLoopKind().String()
	}
	if jobType == domaintypes.JobTypeHeal && strings.TrimSpace(job.JobImage) != "" {
		ctxPayload.ResolvedHealingImage = strings.TrimSpace(job.JobImage)
	}

	jobs, err := st.ListJobsByRunRepoAttempt(ctx, store.ListJobsByRunRepoAttemptParams{
		RunID:   runID,
		RepoID:  job.RepoID,
		Attempt: job.Attempt,
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs for recovery context: %w", err)
	}

	jobsByID := make(map[domaintypes.JobID]store.Job, len(jobs))
	for _, j := range jobs {
		jobsByID[j.ID] = j
	}

	gateJob, healJob := resolveRecoverySourceJobs(job, jobsByID)
	if healJob != nil && strings.TrimSpace(ctxPayload.ResolvedHealingImage) == "" {
		ctxPayload.ResolvedHealingImage = strings.TrimSpace(healJob.JobImage)
	}

	var detectedExpectation *contracts.StackExpectation
	if gateJob != nil && len(gateJob.Meta) > 0 {
		if gateMeta, err := contracts.UnmarshalJobMeta(gateJob.Meta); err == nil && gateMeta.Gate != nil {
			if stack := gateMeta.Gate.DetectedStack(); stack != "" && stack != contracts.ModStackUnknown {
				ctxPayload.DetectedStack = stack
			}
			if expectation := gateMeta.Gate.DetectedStackExpectation(); expectation != nil {
				detectedExpectation = &contracts.StackExpectation{
					Language: strings.TrimSpace(expectation.Language),
					Release:  strings.TrimSpace(expectation.Release),
					Tool:     strings.TrimSpace(expectation.Tool),
				}
			}
			if logPayload := gateLogPayloadFromClaimMetadata(gateMeta.Gate); strings.TrimSpace(logPayload) != "" {
				ctxPayload.BuildGateLog = logPayload
			}
		}
	}
	if detectedExpectation == nil {
		detectedExpectation = stackExpectationFromModStack(ctxPayload.DetectedStack)
	}
	if kind == contracts.RecoveryErrorKindDeps {
		ctxPayload.DepsCompatEndpoint = buildDepsCompatEndpoint(detectedExpectation)
	}

	if kind, ok := contracts.ParseRecoveryErrorKind(ctxPayload.SelectedErrorKind); ok && contracts.IsInfraRecoveryErrorKind(kind) {
		schemaRaw, err := contracts.ReadGateProfileSchemaJSON()
		if err != nil {
			return nil, err
		}
		if !json.Valid(schemaRaw) {
			return nil, fmt.Errorf("gate profile schema JSON is invalid")
		}
		ctxPayload.GateProfileSchemaJSON = string(schemaRaw)
	}

	return ctxPayload, nil
}

func resolveRecoverySourceJobs(
	current store.Job,
	jobsByID map[domaintypes.JobID]store.Job,
) (*store.Job, *store.Job) {
	switch domaintypes.JobType(current.JobType) {
	case domaintypes.JobTypeHeal:
		heal := current
		prev := recoveryChainPredecessor(current.ID, jobsByID)
		if prev != nil && isGateJobTypeForClaim(domaintypes.JobType(prev.JobType)) {
			return prev, &heal
		}
		return nil, &heal
	case domaintypes.JobTypeReGate:
		prev := recoveryChainPredecessor(current.ID, jobsByID)
		if prev == nil {
			return nil, nil
		}
		if domaintypes.JobType(prev.JobType) == domaintypes.JobTypeHeal {
			heal := *prev
			prevGate := recoveryChainPredecessor(prev.ID, jobsByID)
			if prevGate != nil && isGateJobTypeForClaim(domaintypes.JobType(prevGate.JobType)) {
				return prevGate, &heal
			}
			return nil, &heal
		}
		if isGateJobTypeForClaim(domaintypes.JobType(prev.JobType)) {
			return prev, nil
		}
	}
	return nil, nil
}

func isGateJobTypeForClaim(jobType domaintypes.JobType) bool {
	return jobType == domaintypes.JobTypePreGate ||
		jobType == domaintypes.JobTypePostGate ||
		jobType == domaintypes.JobTypeReGate
}

func gateLogPayloadFromClaimMetadata(gateMetadata *contracts.BuildGateStageMetadata) string {
	if gateMetadata == nil {
		return ""
	}
	if len(gateMetadata.LogFindings) == 0 {
		return ""
	}
	logPayload := strings.TrimSpace(gateMetadata.LogFindings[0].Message)
	if logPayload == "" {
		return ""
	}
	if !strings.HasSuffix(logPayload, "\n") {
		logPayload += "\n"
	}
	return logPayload
}

func cloneRawJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), in...)
}

func buildDepsCompatEndpoint(stack *contracts.StackExpectation) string {
	if stack == nil {
		return ""
	}
	lang := strings.ToLower(strings.TrimSpace(stack.Language))
	release := strings.TrimSpace(stack.Release)
	tool := strings.ToLower(strings.TrimSpace(stack.Tool))
	if lang == "" || release == "" || tool == "" {
		return ""
	}
	return fmt.Sprintf(
		"/v1/sboms/compat?lang=%s&release=%s&tool=%s&libs=",
		url.QueryEscape(lang),
		url.QueryEscape(release),
		url.QueryEscape(tool),
	)
}
