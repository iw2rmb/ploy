package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server"
	"github.com/iw2rmb/ploy/internal/server/auth"
	"github.com/iw2rmb/ploy/internal/server/blobpersist"
	"github.com/iw2rmb/ploy/internal/store"
)

// completeJobRequest represents the request body for job completion.
// This is a simpler contract than the node-based endpoint since job_id
// is in the URL path and node identity comes from mTLS.
type completeJobRequest struct {
	Status     string          `json:"status"`                 // Terminal status: Success, Fail, or Cancelled
	ExitCode   *int32          `json:"exit_code,omitempty"`    // Exit code from job execution
	Stats      json.RawMessage `json:"stats,omitempty"`        // Optional job statistics (must be JSON object)
	RepoSHAOut string          `json:"repo_sha_out,omitempty"` // Optional lowercase 40-hex output SHA reported by node.
}

// completeJobHandler marks a job as completed with terminal status and stats.
func completeJobHandler(st store.Store, eventsService *server.EventsService, bp *blobpersist.Service, gateProfileBlobstore ...blobstore.Store) http.HandlerFunc {
	var gateProfilesBS blobstore.Store
	if len(gateProfileBlobstore) > 0 {
		gateProfilesBS = gateProfileBlobstore[0]
	}
	service := NewCompleteJobService(st, eventsService, bp, gateProfilesBS)

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		jobID, err := parseRequiredPathID[domaintypes.JobID](r, "job_id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		if _, ok := auth.IdentityFromContext(ctx); !ok {
			writeHTTPError(w, http.StatusUnauthorized, "unauthorized: no identity in context")
			return
		}

		var req completeJobRequest
		if err := decodeRequestJSON(w, r, &req, DefaultMaxBodySize); err != nil {
			return
		}

		normalizedStatus, statsPayload, statsBytes, repoSHAOut, nodeID, err := validateCompleteJobRequest(r, req)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		_, err = service.Complete(ctx, CompleteJobInput{
			JobID:        jobID,
			NodeID:       nodeID,
			Status:       normalizedStatus,
			ExitCode:     req.ExitCode,
			StatsPayload: statsPayload,
			StatsBytes:   statsBytes,
			RepoSHAOut:   repoSHAOut,
		})
		if err != nil {
			switch e := err.(type) {
			case *CompleteJobBadRequest:
				writeHTTPError(w, http.StatusBadRequest, "%s", e.Message)
				return
			case *CompleteJobForbidden:
				writeHTTPError(w, http.StatusForbidden, "%s", e.Message)
				return
			case *CompleteJobConflict:
				writeHTTPError(w, http.StatusConflict, "%s", e.Message)
				return
			case *CompleteJobNotFound:
				writeHTTPError(w, http.StatusNotFound, "%s", e.Message)
				return
			case *CompleteJobInternal:
				writeHTTPError(w, http.StatusInternalServerError, "%s", e.Error())
				return
			default:
				slog.Error("complete job: unhandled service error", "job_id", jobID, "err", err)
				writeHTTPError(w, http.StatusInternalServerError, "complete job failed: %v", err)
				return
			}
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func validateCompleteJobRequest(r *http.Request, req completeJobRequest) (
	domaintypes.JobStatus,
	JobStatsPayload,
	[]byte,
	string,
	domaintypes.NodeID,
	error,
) {
	if strings.TrimSpace(req.Status) == "" {
		return "", JobStatsPayload{}, nil, "", "", completeBadRequest("status is required")
	}

	normalizedStatus, err := domaintypes.ParseJobStatus(strings.TrimSpace(req.Status))
	if err != nil {
		return "", JobStatsPayload{}, nil, "", "", completeBadRequest("invalid status: %v", err)
	}
	if normalizedStatus != domaintypes.JobStatusSuccess &&
		normalizedStatus != domaintypes.JobStatusFail &&
		normalizedStatus != domaintypes.JobStatusCancelled {
		return "", JobStatsPayload{}, nil, "", "", completeBadRequest("status must be Success, Fail, or Cancelled, got %s", req.Status)
	}

	statsBytes := []byte("{}")
	var statsPayload JobStatsPayload
	if len(req.Stats) > 0 {
		if !json.Valid(req.Stats) {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("stats field must be valid JSON")
		}
		var rawCheck json.RawMessage
		if err := json.Unmarshal(req.Stats, &rawCheck); err != nil {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("invalid stats JSON")
		}
		trimmed := strings.TrimSpace(string(rawCheck))
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("stats must be a JSON object")
		}
		if err := json.Unmarshal(req.Stats, &statsPayload); err != nil {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("invalid stats payload: %v", err)
		}
		statsBytes = req.Stats
		if err := statsPayload.ValidateJobMeta(); err != nil {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("%s", err)
		}
		if err := statsPayload.ValidateJobResources(); err != nil {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("%s", err)
		}
	}

	repoSHAOut := ""
	if candidate := strings.TrimSpace(req.RepoSHAOut); candidate != "" {
		if !sha40Pattern.MatchString(candidate) {
			return "", JobStatsPayload{}, nil, "", "", completeBadRequest("repo_sha_out must match ^[0-9a-f]{40}$")
		}
		repoSHAOut = candidate
	}

	nodeIDHeaderStr := strings.TrimSpace(r.Header.Get(nodeUUIDHeader))
	if nodeIDHeaderStr == "" {
		return "", JobStatsPayload{}, nil, "", "", completeBadRequest("PLOY_NODE_UUID header is required")
	}
	var nodeIDHeader domaintypes.NodeID
	if err := nodeIDHeader.UnmarshalText([]byte(nodeIDHeaderStr)); err != nil {
		return "", JobStatsPayload{}, nil, "", "", completeBadRequest("invalid PLOY_NODE_UUID header")
	}

	return normalizedStatus, statsPayload, statsBytes, repoSHAOut, nodeIDHeader, nil
}
