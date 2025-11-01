package httpserver

import (
    "encoding/json"
    "errors"
    "io"
    "net/http"
    "strings"
)

func writeError(w http.ResponseWriter, status int, err error) {
	if err == nil {
		writeErrorMessage(w, status, http.StatusText(status))
		return
	}
	writeErrorMessage(w, status, err.Error())
}

func writeErrorMessage(w http.ResponseWriter, status int, message string) {
	writeErrorWithCode(w, status, "", message)
}

func writeErrorWithCode(w http.ResponseWriter, status int, code, message string) {
	if strings.TrimSpace(message) == "" {
		message = http.StatusText(status)
	}
	payload := map[string]any{"error": message}
	code = strings.TrimSpace(code)
	if code != "" {
		payload["error_code"] = code
	}
	writeJSON(w, status, payload)
}

// decodeJSON decodes a JSON request with a default 1 MiB body cap.
func decodeJSON(r *http.Request, dst any) error {
    return decodeJSONWithLimit(r, dst, 1<<20)
}

// decodeJSONWithLimit decodes a JSON request with an explicit byte limit.
// It also rejects extra trailing JSON tokens.
func decodeJSONWithLimit(r *http.Request, dst any, limit int64) error {
    defer func() { _ = r.Body.Close() }()
    dec := json.NewDecoder(io.LimitReader(r.Body, limit))
    dec.DisallowUnknownFields()
    if err := dec.Decode(dst); err != nil {
        return err
    }
    if err := dec.Decode(new(struct{})); err != io.EOF {
        if err == nil {
            return errors.New("unexpected trailing json data")
        }
        return err
    }
    return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}
