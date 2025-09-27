package buildgate

import (
	"context"
	"errors"
)

// LogIngestionSpec describes the artifact reference that should be ingested.
type LogIngestionSpec struct {
	Artifact ArtifactReference
}

// LogIngestionResult combines the retrieved log content with parsed findings.
type LogIngestionResult struct {
	LogRetrievalResult
	Findings []LogFinding
}

// LogIngestor orchestrates log retrieval and parsing for build gate stages.
type LogIngestor struct {
	Retriever *LogRetriever
	Parser    LogParser
}

var errLogRetrieverMissing = errors.New("buildgate: log retriever missing")

// Ingest downloads the log artifact and parses it into knowledge base findings.
func (i *LogIngestor) Ingest(ctx context.Context, spec LogIngestionSpec) (LogIngestionResult, error) {
	if i == nil || i.Retriever == nil {
		return LogIngestionResult{}, errLogRetrieverMissing
	}
	retrieval, err := i.Retriever.Retrieve(ctx, spec.Artifact)
	if err != nil {
		return LogIngestionResult{}, err
	}
	parser := i.Parser
	if parser == nil {
		parser = NewDefaultLogParser()
	}
	findings := parser.Parse(string(retrieval.Content))
	return LogIngestionResult{
		LogRetrievalResult: retrieval,
		Findings:           findings,
	}, nil
}
