package buildgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const defaultLogMaxBytes int64 = 1 << 20 // 1 MiB

// LogSource identifies the backend used to obtain a build log.
type LogSource string

const (
    // LogSourceGrid indicates the log originated from a legacy Grid artifact download.
    LogSourceGrid LogSource = "grid"
    // LogSourcePrimary is a compatibility alias for the primary log source.
    // Prefer LogSourceIPFS when applicable.
    LogSourcePrimary LogSource = LogSourceGrid
	// LogSourceIPFS indicates the log was downloaded from IPFS using the artifact CID.
	LogSourceIPFS LogSource = "ipfs"
	// LogSourceStub indicates the log came from an in-memory stub (typically workstation tests).
	LogSourceStub LogSource = "stub"
)

// ArtifactReference identifies the artifact that stores a build log.
type ArtifactReference struct {
	CID         string
	Description string
}

// ArtifactFetcher retrieves artifact contents for build logs.
type ArtifactFetcher interface {
	Fetch(ctx context.Context, ref ArtifactReference) ([]byte, error)
}

// LogRetrievalResult captures the contents and metadata for a retrieved build log.
type LogRetrievalResult struct {
	Source    LogSource
	Digest    string
	Content   []byte
	Truncated bool
}

// LogRetriever downloads build logs from Grid artifacts with optional fallbacks.
type LogRetriever struct {
	Primary        ArtifactFetcher
	PrimarySource  LogSource
	Fallback       ArtifactFetcher
	FallbackSource LogSource
	MaxBytes       int64
}

var (
	// ErrLogFetcherMissing is returned when the retriever has no fetchers configured.
	ErrLogFetcherMissing = errors.New("buildgate: log fetcher not configured")
	// ErrLogUnavailable indicates no fetcher could return the requested log artifact.
	ErrLogUnavailable = errors.New("buildgate: log artifact unavailable")
)

// Retrieve downloads the requested log, preferring the primary fetcher before falling back.
func (r *LogRetriever) Retrieve(ctx context.Context, ref ArtifactReference) (LogRetrievalResult, error) {
	if r == nil || (r.Primary == nil && r.Fallback == nil) {
		return LogRetrievalResult{}, ErrLogFetcherMissing
	}

	maxBytes := r.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultLogMaxBytes
	}

	type candidate struct {
		fetcher ArtifactFetcher
		source  LogSource
	}

	candidates := []candidate{
		{fetcher: r.Primary, source: r.PrimarySource},
		{fetcher: r.Fallback, source: r.FallbackSource},
	}

	var lastErr error
	for _, c := range candidates {
		if c.fetcher == nil {
			continue
		}
		data, err := c.fetcher.Fetch(ctx, ref)
		if err != nil {
			lastErr = err
			continue
		}
		truncated := false
		if int64(len(data)) > maxBytes {
			data = data[:maxBytes]
			truncated = true
		}
		digest := sha256.Sum256(data)
		source := c.source
		if source == "" {
			if c.fetcher == r.Primary {
				source = LogSourceGrid
			} else {
				source = LogSourceIPFS
			}
		}
		// Copy the data to avoid callers mutating fetcher-owned buffers.
		copied := append([]byte(nil), data...)
		return LogRetrievalResult{
			Source:    source,
			Digest:    "sha256:" + hex.EncodeToString(digest[:]),
			Content:   copied,
			Truncated: truncated,
		}, nil
	}
	if lastErr != nil {
		return LogRetrievalResult{}, lastErr
	}
	return LogRetrievalResult{}, ErrLogUnavailable
}
