package nodeagent

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// --- Base uploader (from httpuploader.go) ---

// baseUploader provides common HTTP client functionality for all uploaders.
type baseUploader struct {
	cfg    Config
	client *http.Client
}

func newBaseUploader(cfg Config) (*baseUploader, error) {
	client, err := createHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	return &baseUploader{cfg: cfg, client: client}, nil
}

// --- HTTP error helpers (from httperror.go) ---

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func readErrorBody(resp *http.Response) string {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "(failed to read body)"
	}
	return string(body)
}

func httpError(resp *http.Response, expected int, action string) error {
	if resp.StatusCode == expected {
		return nil
	}
	return fmt.Errorf("%s failed: status %d: %s", action, resp.StatusCode, readErrorBody(resp))
}

// --- URL helpers (from httputil.go) ---

// BuildURL resolves a base URL and a path-only reference, preserving scheme/host.
func BuildURL(base, p string) (string, error) {
	bu, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	pu, err := url.Parse(p)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	if pu.IsAbs() || pu.Scheme != "" || pu.Host != "" || pu.User != nil {
		return "", fmt.Errorf("path must not include scheme or host")
	}
	return bu.ResolveReference(pu).String(), nil
}

// MustBuildURL is like BuildURL but panics on error.
func MustBuildURL(base, p string) string {
	u, err := BuildURL(base, p)
	if err != nil {
		panic(fmt.Sprintf("MustBuildURL: %v", err))
	}
	return u
}

// --- Compression helpers (from compression.go) ---

const (
	// MaxUploadSize is the maximum size for gzipped uploads (10 MiB).
	MaxUploadSize = 10 << 20

	// SoftUploadSize provides headroom for gzip footer.
	SoftUploadSize = MaxUploadSize - 64
)

// ErrPayloadTooLarge is returned when compressed data exceeds MaxUploadSize.
var ErrPayloadTooLarge = errors.New("payload exceeds size cap")

func validateUploadSize(data []byte, dataType string) error {
	if len(data) > MaxUploadSize {
		return fmt.Errorf("%s exceeds size cap: %d > %d bytes: %w",
			dataType, len(data), MaxUploadSize, ErrPayloadTooLarge)
	}
	return nil
}

func gzipCompress(data []byte, dataType string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	if _, err := gzWriter.Write(data); err != nil {
		return nil, fmt.Errorf("gzip %s: %w", dataType, err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("finalize gzip %s: %w", dataType, err)
	}

	compressed := buf.Bytes()
	if err := validateUploadSize(compressed, dataType); err != nil {
		return nil, err
	}

	return compressed, nil
}
