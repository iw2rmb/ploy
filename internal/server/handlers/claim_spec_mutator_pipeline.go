package handlers

import (
	"encoding/json"
	"fmt"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/server/config"
	"github.com/iw2rmb/ploy/internal/store"
)

type claimSpecMutatorInput struct {
	spec            json.RawMessage
	job             store.Job
	jobType         domaintypes.JobType
	gitLab          config.GitLabConfig
	globalEnv       map[string][]GlobalEnvVar
	repoGateProfile []byte
	hydraOverlays   map[string]*HydraJobConfig
	bundleMap       map[string]string // server-side shortHash → bundleID
}

type claimSpecMutator struct {
	errContext string
	apply      func(map[string]any, claimSpecMutatorInput) error
}

// mutateClaimSpec applies all claim-time spec mutators in a fixed order with
// one parse at the beginning and one marshal at the end.
func mutateClaimSpec(input claimSpecMutatorInput) (json.RawMessage, error) {
	specMap, err := parseSpecObjectStrict(input.spec)
	if err != nil {
		return nil, fmt.Errorf("merge job_id into spec: %w", err)
	}

	pipeline := []claimSpecMutator{
		{
			errContext: "merge job_id into spec",
			apply: func(m map[string]any, in claimSpecMutatorInput) error {
				return applyJobIDMutator(m, in.job.ID)
			},
		},
		{
			errContext: "merge gitlab defaults into spec",
			apply: func(m map[string]any, in claimSpecMutatorInput) error {
				return applyGitLabConfigMutator(m, in.gitLab)
			},
		},
		{
			errContext: "merge hydra overlay into spec",
			apply:     applyHydraOverlayMutator,
		},
		{
			errContext: "merge recovery candidate prep into spec",
			apply: func(m map[string]any, in claimSpecMutatorInput) error {
				return applyRecoveryCandidatePrepMutator(m, in.job, in.jobType)
			},
		},
		{
			errContext: "merge repo gate_profile into spec",
			apply: func(m map[string]any, in claimSpecMutatorInput) error {
				return applyRepoGateProfileMutator(m, in.repoGateProfile, in.jobType)
			},
		},
		{
			errContext: "merge healing selected_error_kind into spec",
			apply: func(m map[string]any, in claimSpecMutatorInput) error {
				return applyHealingSelectedKindMutator(m, in.job, in.jobType)
			},
		},
		{
			errContext: "merge healing schema into spec",
			apply: func(m map[string]any, in claimSpecMutatorInput) error {
				return applyHealingSchemaMutator(m, in.job, in.jobType)
			},
		},
		{
			errContext: "merge server bundle map into spec",
			apply:     applyBundleMapMutator,
		},
	}

	for _, mutator := range pipeline {
		if err := mutator.apply(specMap, input); err != nil {
			return nil, fmt.Errorf("%s: %w", mutator.errContext, err)
		}
	}

	return marshalSpecObject(specMap)
}
