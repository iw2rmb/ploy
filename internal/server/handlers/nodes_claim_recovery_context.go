package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/logchunk"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

func buildRecoveryClaimContext(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
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
	if err != nil || jobMeta.RecoveryMetadata == nil {
		return nil, nil
	}

	recovery := jobMeta.RecoveryMetadata
	kind, ok := contracts.ParseRecoveryErrorKind(recovery.ErrorKind)
	if !ok {
		kind = contracts.DefaultRecoveryErrorKind()
	}
	ctxPayload := &contracts.RecoveryClaimContext{
		LoopKind:     strings.TrimSpace(recovery.LoopKind),
		Expectations: cloneRawJSON(recovery.Expectations),
	}
	if kind == contracts.RecoveryErrorKindDeps && recovery.DepsBumps != nil {
		ctxPayload.DepsBumps = lifecycle.CloneDepsBumpsMap(recovery.DepsBumps)
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
		if gateMeta, err := contracts.UnmarshalJobMeta(gateJob.Meta); err == nil && gateMeta.GateMetadata != nil {
			if stack := gateMeta.GateMetadata.DetectedStack(); stack != "" && stack != contracts.MigStackUnknown {
				ctxPayload.DetectedStack = stack
			}
			if expectation := gateMeta.GateMetadata.DetectedStackExpectation(); expectation != nil {
				detectedExpectation = &contracts.StackExpectation{
					Language: strings.TrimSpace(expectation.Language),
					Release:  strings.TrimSpace(expectation.Release),
					Tool:     strings.TrimSpace(expectation.Tool),
				}
			}
			if logPayload := gateLogPayloadFromClaimMetadata(gateMeta.GateMetadata); strings.TrimSpace(logPayload) != "" {
				ctxPayload.BuildGateLog = logPayload
			}
		}
	}
	if strings.TrimSpace(ctxPayload.BuildGateLog) == "" && jobType == domaintypes.JobTypeHeal {
		prev := lifecycle.RecoveryChainPredecessor(job.ID, jobsByID)
		if prev != nil && domaintypes.JobType(prev.JobType) == domaintypes.JobTypeSBOM {
			logPayload, logErr := sbomLogPayloadFromClaimLogs(ctx, st, bs, runID, prev.ID)
			if logErr != nil {
				return nil, &ClaimJobTerminalError{
					Message: fmt.Sprintf("resolve sbom recovery log for predecessor job %s", prev.ID),
					Err:     logErr,
				}
			}
			ctxPayload.BuildGateLog = logPayload
		}
	}
	if detectedExpectation == nil {
		detectedExpectation = lifecycle.StackExpectationFromMigStack(ctxPayload.DetectedStack)
	}
	if kind == contracts.RecoveryErrorKindDeps {
		ctxPayload.DepsCompatEndpoint = buildDepsCompatEndpoint(detectedExpectation)
	}

	if contracts.IsInfraRecoveryErrorKind(kind) {
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
		prev := lifecycle.RecoveryChainPredecessor(current.ID, jobsByID)
		if prev != nil && isGateJobTypeForClaim(domaintypes.JobType(prev.JobType)) {
			return prev, &heal
		}
		return nil, &heal
	case domaintypes.JobTypeReGate:
		prev := lifecycle.RecoveryChainPredecessor(current.ID, jobsByID)
		if prev == nil {
			return nil, nil
		}
		if domaintypes.JobType(prev.JobType) == domaintypes.JobTypeHeal {
			heal := *prev
			prevGate := lifecycle.RecoveryChainPredecessor(prev.ID, jobsByID)
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

func sbomLogPayloadFromClaimLogs(
	ctx context.Context,
	st store.Store,
	bs blobstore.Store,
	runID domaintypes.RunID,
	jobID domaintypes.JobID,
) (string, error) {
	if bs == nil {
		return "", errors.New("blob store is required")
	}

	jobIDCopy := jobID
	logs, err := st.ListLogsByRunAndJob(ctx, store.ListLogsByRunAndJobParams{
		RunID: runID,
		JobID: &jobIDCopy,
	})
	if err != nil {
		return "", fmt.Errorf("list sbom logs: %w", err)
	}
	if len(logs) == 0 {
		return "", errors.New("no sbom logs found")
	}

	var out strings.Builder
	for _, chunk := range logs {
		if chunk.ObjectKey == nil || strings.TrimSpace(*chunk.ObjectKey) == "" {
			return "", fmt.Errorf("log chunk %d has empty object_key", chunk.ID)
		}
		data, readErr := blobstore.ReadAll(ctx, bs, *chunk.ObjectKey)
		if readErr != nil {
			return "", fmt.Errorf("read sbom log chunk %d: %w", chunk.ID, readErr)
		}
		records, decodeErr := logchunk.DecodeGzip(data)
		if decodeErr != nil {
			return "", fmt.Errorf("decode sbom log chunk %d: %w", chunk.ID, decodeErr)
		}
		for _, record := range records {
			line := strings.TrimRight(record.Line, "\r\n")
			if strings.TrimSpace(line) == "" {
				continue
			}
			out.WriteString(line)
			out.WriteByte('\n')
		}
	}
	if out.Len() == 0 {
		return "", errors.New("sbom logs contain no non-empty lines")
	}
	return out.String(), nil
}
