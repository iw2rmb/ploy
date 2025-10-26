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

func decodeJSON(r *http.Request, dst any) error {
	defer func() {
		_ = r.Body.Close()
	}()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
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
