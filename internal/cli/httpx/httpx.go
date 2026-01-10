package httpx

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const (
	// MaxErrorBodyBytes caps response bodies read on error paths.
	MaxErrorBodyBytes int64 = 2048
	// MaxJSONBodyBytes caps JSON response bodies decoded into structs.
	MaxJSONBodyBytes int64 = 1 << 20 // 1 MiB
	// MaxDownloadBodyBytes caps large download bodies read into memory.
	MaxDownloadBodyBytes int64 = 64 << 20 // 64 MiB
)

func DecodeJSON(r io.Reader, out any, limit int64) error {
	if limit > 0 {
		r = io.LimitReader(r, limit)
	}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

func ReadErrorMessage(r io.Reader, status string, limit int64) string {
	if limit <= 0 {
		limit = MaxErrorBodyBytes
	}
	data, _ := io.ReadAll(io.LimitReader(r, limit))
	body := strings.TrimSpace(string(data))
	if body == "" {
		return status
	}

	var apiErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &apiErr); err == nil {
		msg := strings.TrimSpace(apiErr.Error)
		if msg != "" {
			return msg
		}
	}

	return body
}

func WrapError(prefix string, status string, r io.Reader) error {
	msg := ReadErrorMessage(r, status, MaxErrorBodyBytes)
	return fmt.Errorf("%s: %s", prefix, msg)
}
