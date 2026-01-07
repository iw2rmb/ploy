package nodeagent

import (
	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// newTestRunController creates a runController with shared uploaders initialized
// for use in tests. This mirrors the initialization done in agent.go's New function
// but is designed for test configurations.
//
// Returns the controller and any error from uploader creation. Tests should skip
// or fail if an error is returned.
//
// Usage:
//
//	controller, err := newTestRunController(cfg)
//	if err != nil {
//	    t.Fatalf("failed to create test controller: %v", err)
//	}
func newTestRunController(cfg Config) (*runController, error) {
	diffUploader, err := NewDiffUploader(cfg)
	if err != nil {
		return nil, err
	}

	artifactUploader, err := NewArtifactUploader(cfg)
	if err != nil {
		return nil, err
	}

	statusUploader, err := NewStatusUploader(cfg)
	if err != nil {
		return nil, err
	}

	return &runController{
		cfg:              cfg,
		jobs:             make(map[types.JobID]*jobContext),
		diffUploader:     diffUploader,
		artifactUploader: artifactUploader,
		statusUploader:   statusUploader,
	}, nil
}
