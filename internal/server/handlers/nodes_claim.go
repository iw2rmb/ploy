package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/iw2rmb/ploy/internal/blobstore"
	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/events"
	"github.com/iw2rmb/ploy/internal/store"
)

func claimJobHandlerWithEvents(st store.Store, bs blobstore.Store, eventsService *events.Service, configHolder *ConfigHolder) http.HandlerFunc {
	service := newClaimer(st, bs, configHolder, eventsService)

	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, ok := parseRequiredPathIDOrWriteError[domaintypes.NodeID](w, r, "id")
		if !ok {
			return
		}

		result, err := service.Claim(r.Context(), nodeID)
		if err != nil {
			switch e := err.(type) {
			case *claimBadRequest:
				writeHTTPError(w, http.StatusBadRequest, "%s", e.Message)
				return
			case *claimNotFound:
				writeHTTPError(w, http.StatusNotFound, "%s", e.Message)
				return
			case *claimNoWork:
				w.WriteHeader(http.StatusNoContent)
				return
			case *claimInternalError:
				writeHTTPError(w, http.StatusInternalServerError, "%s", e.Error())
				return
			default:
				writeHTTPError(w, http.StatusInternalServerError, "claim failed: %v", err)
				return
			}
		}

		if result.Response != nil {
			writeJSON(w, http.StatusOK, result.Response)
			return
		}
		writeJSON(w, http.StatusOK, result.Payload)
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
