package buildgate

import (
	"context"
	"testing"
)

func TestLogIngestorProducesFindings(t *testing.T) {
	retriever := &LogRetriever{
		Primary:       &fakeFetcher{data: []byte("undefined reference to `symbol'")},
		PrimarySource: LogSourcePrimary,
	}
	ingestor := &LogIngestor{Retriever: retriever, Parser: NewDefaultLogParser()}

	result, err := ingestor.Ingest(context.Background(), LogIngestionSpec{Artifact: ArtifactReference{CID: "bafy"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Digest == "" {
		t.Fatal("expected digest to be populated")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected single finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Code != "kb.linker.undefined_reference" {
		t.Fatalf("unexpected finding code %q", result.Findings[0].Code)
	}
}

func TestLogIngestorDefaultsParser(t *testing.T) {
	retriever := &LogRetriever{
		Primary:       &fakeFetcher{data: []byte("go: module example.com/foo found in multiple modules")},
		PrimarySource: LogSourcePrimary,
	}
	ingestor := &LogIngestor{Retriever: retriever}

	result, err := ingestor.Ingest(context.Background(), LogIngestionSpec{Artifact: ArtifactReference{CID: "bafy"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected one finding, got %d", len(result.Findings))
	}
}

func TestLogIngestorErrorsWithoutRetriever(t *testing.T) {
	ingestor := &LogIngestor{}
	_, err := ingestor.Ingest(context.Background(), LogIngestionSpec{Artifact: ArtifactReference{CID: "bafy"}})
	if err == nil {
		t.Fatal("expected error when retriever missing")
	}
}
