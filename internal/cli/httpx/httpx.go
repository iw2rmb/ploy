package httpx

import (
	"bufio"
	"compress/gzip"
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
	// MaxGunzipOutputBytes caps decompressed bodies produced by streaming gunzip helpers.
	// This protects the CLI from gzip "zip bombs" while still allowing large patches.
	MaxGunzipOutputBytes int64 = 256 << 20 // 256 MiB
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

// GunzipToBytes reads a gzipped stream from r and returns the decompressed bytes.
// If maxBytes <= 0, MaxGunzipOutputBytes is used.
// An empty input stream returns an empty slice with no error.
func GunzipToBytes(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = MaxGunzipOutputBytes
	}

	br := bufio.NewReader(r)
	if _, err := br.Peek(1); err != nil {
		if err == io.EOF {
			return []byte{}, nil
		}
		return nil, err
	}

	gr, err := gzip.NewReader(br)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gr.Close() }()

	lr := &io.LimitedReader{R: gr, N: maxBytes + 1}
	out, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > maxBytes {
		return nil, fmt.Errorf("gunzip: output exceeds %d bytes", maxBytes)
	}
	return out, nil
}
