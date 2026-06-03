package handlers

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/iw2rmb/ploy/internal/trimmer/java/gradle"
	"gopkg.in/yaml.v3"
)

type gradleTrimmerRequest struct {
	Log string `json:"log"`
}

func javaGradleTrimmerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format, err := trimmerResponseFormat(r)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, err.Error())
			return
		}

		logText, err := readGradleTrimmerLog(w, r)
		if err != nil {
			return
		}

		result := gradle.Trim(logText)
		if strings.TrimSpace(result.Message) == "" && result.Evidence == nil {
			writeHTTPError(w, http.StatusBadRequest, "log is empty")
			return
		}

		filename := "gradle-trimmed." + format
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		switch format {
		case "json":
			writeJSON(w, http.StatusOK, result)
		case "yaml":
			w.Header().Set("Content-Type", "application/x-yaml")
			w.WriteHeader(http.StatusOK)
			if err := yaml.NewEncoder(w).Encode(result); err != nil {
				// yaml encoding of this fixed struct should not fail.
				return
			}
		}
	}
}

func readGradleTrimmerLog(w http.ResponseWriter, r *http.Request) (string, error) {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return readPlainGradleTrimmerLog(w, r)
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, "unsupported content type")
		return "", err
	}

	switch strings.ToLower(mediaType) {
	case "application/json":
		var req gradleTrimmerRequest
		if err := decodeRequestJSON(w, r, &req, ingestMaxBodySize); err != nil {
			return "", err
		}
		if len([]byte(req.Log)) > ingestMaxDataSize {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "log exceeds data size cap")
			return "", errors.New("log exceeds data size cap")
		}
		if strings.TrimSpace(req.Log) == "" {
			writeHTTPError(w, http.StatusBadRequest, "log is required")
			return "", errors.New("log is required")
		}
		return req.Log, nil
	case "text/plain":
		return readPlainGradleTrimmerLog(w, r)
	default:
		writeHTTPError(w, http.StatusBadRequest, "unsupported content type")
		return "", fmt.Errorf("unsupported content type %q", mediaType)
	}
}

func readPlainGradleTrimmerLog(w http.ResponseWriter, r *http.Request) (string, error) {
	if rejectOversizedContentLength(w, r, ingestMaxDataSize) {
		return "", errors.New("payload exceeds body size cap")
	}
	r.Body = http.MaxBytesReader(w, r.Body, ingestMaxDataSize)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeHTTPError(w, http.StatusRequestEntityTooLarge, "payload exceeds body size cap")
			return "", err
		}
		writeHTTPError(w, http.StatusBadRequest, "invalid request: %v", err)
		return "", err
	}
	if strings.TrimSpace(string(data)) == "" {
		writeHTTPError(w, http.StatusBadRequest, "log is required")
		return "", errors.New("log is required")
	}
	return string(data), nil
}

func trimmerResponseFormat(r *http.Request) (string, error) {
	if raw := strings.TrimSpace(r.URL.Query().Get("format")); raw != "" {
		switch strings.ToLower(raw) {
		case "json", "yaml":
			return strings.ToLower(raw), nil
		default:
			return "", fmt.Errorf("unsupported format %q", raw)
		}
	}
	if acceptPrefersYAML(r.Header.Get("Accept")) {
		return "yaml", nil
	}
	return "json", nil
}

func acceptPrefersYAML(header string) bool {
	bestYAML := -1.0
	bestJSON := -1.0
	bestAny := -1.0
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		mediaType, params, err := mime.ParseMediaType(part)
		if err != nil {
			continue
		}
		q := 1.0
		if rawQ := strings.TrimSpace(params["q"]); rawQ != "" {
			parsed, err := strconv.ParseFloat(rawQ, 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		switch strings.ToLower(mediaType) {
		case "application/x-yaml", "application/yaml", "text/yaml", "text/x-yaml":
			if q > bestYAML {
				bestYAML = q
			}
		case "application/json":
			if q > bestJSON {
				bestJSON = q
			}
		case "*/*":
			if q > bestAny {
				bestAny = q
			}
		}
	}
	if bestYAML < 0 {
		return false
	}
	if bestJSON < 0 {
		bestJSON = bestAny
	}
	return bestYAML > bestJSON
}
