package handlers

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/lifecycle"
)

// requiredPathParam extracts and validates a required path parameter from the request.
// Returns the trimmed value or an error if the parameter is missing or empty.
func requiredPathParam(r *http.Request, key string) (string, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return "", fmt.Errorf("%s path parameter is required", key)
	}
	return val, nil
}

// writeHTTPError writes a plain-text HTTP error response. It accepts printf-style
// formatting for the message body.
func writeHTTPError(w http.ResponseWriter, code int, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	http.Error(w, msg, code)
}

// DefaultMaxBodySize is the default request body size limit (1 MiB).
const DefaultMaxBodySize = 1 << 20

// decodeRequestJSON decodes a JSON request body with strict validation:
//   - Caps request body at maxBytes using http.MaxBytesReader
//   - Rejects unknown JSON fields (fails fast on contract drift)
//
// Returns nil on success. On error, writes an appropriate HTTP response and returns the error.
// Callers should return immediately after a non-nil error.
func decodeRequestJSON(w http.ResponseWriter, r *http.Request, v any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		// Return 413 when MaxBytesReader trips the size cap.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
			return err
		}
		writeHTTPError(w, http.StatusBadRequest, "invalid request: %v", err)
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeHTTPError(w, http.StatusBadRequest, "invalid request: request body must contain exactly one JSON value")
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return fmt.Errorf("request body must contain exactly one JSON value: %w", err)
	}
	return nil
}

// parseRequiredPathID extracts and validates a typed ID from a path parameter.
// T must implement encoding.TextUnmarshaler (all domain ID types do).
// Returns an error if the parameter is missing, empty, or fails validation.
func parseRequiredPathID[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (T, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		var zero T
		return zero, fmt.Errorf("%s path parameter is required", key)
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		var zero T
		return zero, fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// optionalParam extracts an optional typed ID from a path parameter.
// Returns nil if the parameter is missing or empty.
func optionalParam[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (*T, error) {
	val := strings.TrimSpace(r.PathValue(key))
	if val == "" {
		return nil, nil
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// parseQuery extracts and validates a typed ID from a query parameter.
// Returns an error if the parameter is missing, empty, or fails validation.
func parseQuery[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (T, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		var zero T
		return zero, fmt.Errorf("%s query parameter is required", key)
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		var zero T
		return zero, fmt.Errorf("%s: %w", key, err)
	}
	return id, nil
}

// optionalQuery extracts an optional typed ID from a query parameter.
// Returns nil if the parameter is missing or empty.
func optionalQuery[T any, PT interface {
	*T
	encoding.TextUnmarshaler
}](r *http.Request, key string) (*T, error) {
	val := strings.TrimSpace(r.URL.Query().Get(key))
	if val == "" {
		return nil, nil
	}
	var id T
	if err := PT(&id).UnmarshalText([]byte(val)); err != nil {
		return nil, fmt.Errorf("%s: %w", key, err)
	}
	return &id, nil
}

// parsePagination extracts limit/offset from query parameters with validation.
// Defaults: limit=50, offset=0. Limit is capped at 100.
func parsePagination(r *http.Request) (limit, offset int32, err error) {
	limit = 50
	if l := r.URL.Query().Get("limit"); l != "" {
		parsed, parseErr := strconv.ParseInt(l, 10, 32)
		if parseErr != nil || parsed < 1 {
			return 0, 0, fmt.Errorf("invalid limit parameter")
		}
		limit = int32(parsed)
		if limit > 100 {
			limit = 100
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		parsed, parseErr := strconv.ParseInt(o, 10, 32)
		if parseErr != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("invalid offset parameter")
		}
		offset = int32(parsed)
	}
	return limit, offset, nil
}

// getRunOrFail fetches a run by ID. On error it writes the HTTP response
// (404 for not found, 500 for other errors) and returns ok=false.
func getRunOrFail(w http.ResponseWriter, r *http.Request, st store.Store, runID domaintypes.RunID, logPrefix string) (store.Run, bool) {
	run, err := st.GetRun(r.Context(), runID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusNotFound, "run not found")
			return store.Run{}, false
		}
		slog.Error(logPrefix+": database error", "run_id", runID.String(), "err", err)
		writeHTTPError(w, http.StatusInternalServerError, "failed to get run: %v", err)
		return store.Run{}, false
	}
	return run, true
}

// getActiveRunOrFail fetches a run and rejects terminal runs with 409 Conflict.
func getActiveRunOrFail(w http.ResponseWriter, r *http.Request, st store.Store, runID domaintypes.RunID, logPrefix string) (store.Run, bool) {
	run, ok := getRunOrFail(w, r, st, runID, logPrefix)
	if !ok {
		return store.Run{}, false
	}
	if lifecycle.IsTerminalRunStatus(run.Status) {
		writeHTTPError(w, http.StatusConflict, "run is in terminal state")
		return store.Run{}, false
	}
	return run, true
}

// getMigByRefOrFail parses the "mig_ref" path parameter, resolves the mig by
// ID-or-name, and writes an HTTP error response on failure (400/404/500).
// Returns (mig, true) on success, (zero, false) when the response has already
// been written.
func getMigByRefOrFail(w http.ResponseWriter, r *http.Request, st store.Store, logPrefix string) (store.Mig, bool) {
	ref, err := parseRequiredPathID[domaintypes.MigRef](r, "mig_ref")
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "%s", err)
		return store.Mig{}, false
	}
	mig, err := resolveMigByRef(r.Context(), st, ref)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusNotFound, "mig not found")
			return store.Mig{}, false
		}
		slog.Error(logPrefix+": get mig failed", "mig_ref", ref, "err", err)
		writeHTTPError(w, http.StatusInternalServerError, "failed to get mig: %v", err)
		return store.Mig{}, false
	}
	return mig, true
}

// getMigByIDOrFail parses the "mig_id" path parameter, fetches the mig by ID,
// and writes an HTTP error response on failure (400/404/500).
// Returns (mig, true) on success, (zero, false) when the response has already
// been written.
func getMigByIDOrFail(w http.ResponseWriter, r *http.Request, st store.Store, logPrefix string) (store.Mig, bool) {
	modID, err := parseRequiredPathID[domaintypes.MigID](r, "mig_id")
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "%s", err)
		return store.Mig{}, false
	}
	mig, err := st.GetMig(r.Context(), modID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeHTTPError(w, http.StatusNotFound, "mig not found")
			return store.Mig{}, false
		}
		slog.Error(logPrefix+": get mig failed", "mig_id", modID, "err", err)
		writeHTTPError(w, http.StatusInternalServerError, "failed to get mig: %v", err)
		return store.Mig{}, false
	}
	return mig, true
}

// streamBlob writes standard download headers and streams content from r to w.
// The caller is responsible for opening/closing the reader.
func streamBlob(w http.ResponseWriter, reader io.Reader, size int64, filename, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, reader); err != nil {
		slog.Error("stream blob failed", "filename", filename, "err", err)
	}
}
