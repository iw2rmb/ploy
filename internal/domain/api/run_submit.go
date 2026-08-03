package api

import (
	"encoding/json"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// RunSubmitRequest is the canonical request DTO for POST /v1/runs.
// It is the authoritative shape for single-repo run submission shared between
// the server handler and CLI clients.
//
// Wire shape is stable: JSON field names must not change.
type RunSubmitRequest struct {
	RepoURL       domaintypes.RepoURL `json:"repo_url"`
	Ref           domaintypes.GitRef  `json:"ref"`
	CommitSHA     string              `json:"commit_sha,omitempty"`
	Spec          json.RawMessage     `json:"spec,omitempty"`
	SpecSelector  string              `json:"spec_selector,omitempty"`
	SpecOverrides *RunSpecOverrides   `json:"spec_overrides,omitempty"`
	CreatedBy     string              `json:"created_by,omitempty"`

	GitLabToken *string `json:"gitlab_token,omitempty"`
}

// RunSpecOverrides contains mutations that the server applies only to a named
// spec after it compiles the selected repository source.
type RunSpecOverrides struct {
	StepEnvs        map[string][]string          `json:"step_envs,omitempty"`
	BuildGateForced *RunBuildGateForcedOverrides `json:"build_gate_forced,omitempty"`
}

type RunBuildGateForcedOverrides struct {
	Pre  *RunBuildGateForcedStack `json:"pre,omitempty"`
	Post *RunBuildGateForcedStack `json:"post,omitempty"`
}

type RunBuildGateForcedStack struct {
	Language string `json:"language"`
	Release  string `json:"release"`
	Tool     string `json:"tool,omitempty"`
}

// CreateSingleRepoRunResponse is returned by POST /v1/runs.
type CreateSingleRepoRunResponse struct {
	WaveID domaintypes.WaveID `json:"wave_id"`
	RunID  domaintypes.RunID  `json:"run_id"`
	MigID  domaintypes.MigID  `json:"mig_id"`
	SpecID domaintypes.SpecID `json:"spec_id"`
}

type CreateMigRunRequest struct {
	RepoSelector MigRepoSelector `json:"repo_selector"`
	CreatedBy    *string         `json:"created_by,omitempty"`

	GitLabToken *string `json:"gitlab_token,omitempty"`
}

type RunRestartRequest struct {
	GitLabToken *string `json:"gitlab_token,omitempty"`
}

type MigRepoSelector struct {
	Mode  string                `json:"mode"`
	Repos []domaintypes.RepoURL `json:"repos,omitempty"`
}
