package api

import "github.com/iw2rmb/ploy/internal/domain/types"

type PullResolutionResponse struct {
	RunID           types.RunID  `json:"run_id"`
	RepoID          types.RepoID `json:"repo_id"`
	RepoURL         string       `json:"repo_url,omitempty"`
	SourceCommitSHA string       `json:"source_commit_sha,omitempty"`
}
