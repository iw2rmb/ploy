package buildgate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

type fakeFetcher struct {
	data []byte
	err  error
}

func (f *fakeFetcher) Fetch(ctx context.Context, ref ArtifactReference) ([]byte, error) {
	if f == nil {
		return nil, errors.New("nil fetcher")
	}
	return f.data, f.err
}

func TestLogRetrieverPrefersPrimary(t *testing.T) {
	primary := &fakeFetcher{data: []byte("primary log")}
	fallback := &fakeFetcher{data: []byte("fallback log")}
	retriever := &LogRetriever{
		Primary:        primary,
        PrimarySource:  LogSourcePrimary,
		Fallback:       fallback,
		FallbackSource: LogSourceIPFS,
		MaxBytes:       1024,
	}

	result, err := retriever.Retrieve(context.Background(), ArtifactReference{CID: "bafy"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result.Content) != "primary log" {
		t.Fatalf("expected primary content, got %q", string(result.Content))
	}
if result.Source != LogSourcePrimary {
		t.Fatalf("expected grid source, got %q", result.Source)
	}
	expectedDigest := sha256.Sum256([]byte("primary log"))
	if result.Digest != "sha256:"+hex.EncodeToString(expectedDigest[:]) {
		t.Fatalf("unexpected digest: %q", result.Digest)
	}
	if result.Truncated {
		t.Fatal("expected result not to be truncated")
	}
}

func TestLogRetrieverFallsBackWhenPrimaryFails(t *testing.T) {
	primary := &fakeFetcher{err: errors.New("boom")}
	fallback := &fakeFetcher{data: []byte("fallback")}
	retriever := &LogRetriever{
		Primary:        primary,
        PrimarySource:  LogSourcePrimary,
		Fallback:       fallback,
		FallbackSource: LogSourceIPFS,
	}

	result, err := retriever.Retrieve(context.Background(), ArtifactReference{CID: "bafy"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result.Content) != "fallback" {
		t.Fatalf("expected fallback content, got %q", string(result.Content))
	}
	if result.Source != LogSourceIPFS {
		t.Fatalf("expected IPFS source, got %q", result.Source)
	}
}

func TestLogRetrieverTruncatesLargeLogs(t *testing.T) {
	primary := &fakeFetcher{data: []byte("0123456789")}
	retriever := &LogRetriever{
		Primary:       primary,
        PrimarySource: LogSourcePrimary,
		MaxBytes:      4,
	}

	result, err := retriever.Retrieve(context.Background(), ArtifactReference{CID: "bafy"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if string(result.Content) != "0123" {
		t.Fatalf("expected truncated content, got %q", string(result.Content))
	}
	if !result.Truncated {
		t.Fatal("expected result to be truncated")
	}
}

func TestLogRetrieverErrorsWhenNoFetcherSucceeds(t *testing.T) {
	retriever := &LogRetriever{
		Primary:       &fakeFetcher{err: errors.New("primary failed")},
		Fallback:      &fakeFetcher{err: errors.New("fallback failed")},
        PrimarySource: LogSourcePrimary,
	}

	_, err := retriever.Retrieve(context.Background(), ArtifactReference{CID: "bafy"})
	if err == nil {
		t.Fatal("expected error when both fetchers fail")
	}
}
