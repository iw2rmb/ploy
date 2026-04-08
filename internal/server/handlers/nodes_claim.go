package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
)

// claimJobHandler allows nodes to claim a queued job for execution.
// Returns the claimed job with its parent run metadata or 204 No Content if no work is available.
func claimJobHandler(st store.Store, bs blobstore.Store, configHolder *ConfigHolder, gateProfileResolver ...GateProfileResolver) http.HandlerFunc {
	var resolver GateProfileResolver
	if len(gateProfileResolver) > 0 {
		resolver = gateProfileResolver[0]
	}
	service := NewClaimService(st, bs, configHolder, resolver)

	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, err := parseRequiredPathID[domaintypes.NodeID](r, "id")
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "%s", err)
			return
		}

		result, err := service.Claim(r.Context(), nodeID)
		if err != nil {
			switch e := err.(type) {
			case *ClaimBadRequest:
				writeHTTPError(w, http.StatusBadRequest, "%s", e.Message)
				return
			case *ClaimNotFound:
				writeHTTPError(w, http.StatusNotFound, "%s", e.Message)
				return
			case *ClaimNoWork:
				w.WriteHeader(http.StatusNoContent)
				return
			case *ClaimInternal:
				writeHTTPError(w, http.StatusInternalServerError, "%s", e.Error())
				return
			default:
				writeHTTPError(w, http.StatusInternalServerError, "claim failed: %v", err)
				return
			}
		}

		if err := writeClaimResponse(w, result.Payload); err != nil {
			writeHTTPError(w, http.StatusInternalServerError, "failed to encode claim response: %v", err)
			return
		}
	}
}

func isNoRowsError(err error) bool {
	if err == nil {
		return false
	}
	if err == pgx.ErrNoRows {
		return true
	}
	return errors.Is(err, pgx.ErrNoRows)
}

func safeErrorString(err error) (msg string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			msg = fmt.Sprintf("unprintable error (%T): panic while reading error string: %v", err, recovered)
		}
	}()
	return err.Error()
}

func nodeIDPtrOrZero(id *domaintypes.NodeID) domaintypes.NodeID {
	if id == nil {
		return ""
	}
	return *id
}

func shouldResolveGateProfile(jobType domaintypes.JobType) bool {
	switch jobType {
	case domaintypes.JobTypePreGate, domaintypes.JobTypePostGate, domaintypes.JobTypeReGate:
		return true
	default:
		return false
	}
}
