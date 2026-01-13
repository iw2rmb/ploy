package nodeagent

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
)

const (
	// MaxUploadSize is the maximum size for gzipped uploads (1 MiB).
	// Used by artifact bundles, diffs, and log chunks to enforce server limits.
	MaxUploadSize = 1 << 20

	// SoftUploadSize provides headroom for gzip footer so finalized chunks
	// never exceed the hard cap. Keep conservative margin.
	SoftUploadSize = MaxUploadSize - 64
)

// ErrPayloadTooLarge is returned when compressed data exceeds MaxUploadSize.
var ErrPayloadTooLarge = errors.New("payload exceeds size cap")

// validateUploadSize checks if the data size is within the allowed limit.
// Returns a descriptive error if the size exceeds MaxUploadSize.
func validateUploadSize(data []byte, dataType string) error {
	if len(data) > MaxUploadSize {
		return fmt.Errorf("%s exceeds size cap: %d > %d bytes: %w",
			dataType, len(data), MaxUploadSize, ErrPayloadTooLarge)
	}
	return nil
}

// gzipCompress compresses data using gzip and validates the result size.
// Returns ErrPayloadTooLarge (wrapped) if the compressed data exceeds MaxUploadSize.
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
