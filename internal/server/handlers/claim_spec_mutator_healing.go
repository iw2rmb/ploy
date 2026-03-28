package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/store"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func applyHealingSelectedKindMutator(m map[string]any, job store.Job, jobType domaintypes.JobType) error {
	if jobType != domaintypes.JobTypeHeal {
		return nil
	}
	if len(job.Meta) == 0 {
		return nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil {
		return nil
	}
	if jobMeta.RecoveryMetadata == nil || strings.TrimSpace(jobMeta.RecoveryMetadata.ErrorKind) == "" {
		return nil
	}

	buildGate, err := ensureObjectField(m, "build_gate", "spec")
	if err != nil {
		return err
	}
	healing, err := ensureObjectField(buildGate, "healing", "spec.build_gate")
	if err != nil {
		return err
	}
	kind, ok := contracts.ParseRecoveryErrorKind(jobMeta.RecoveryMetadata.ErrorKind)
	if !ok {
		kind = contracts.DefaultRecoveryErrorKind()
	}
	healing["selected_error_kind"] = kind.String()
	if len(jobMeta.RecoveryMetadata.Expectations) > 0 {
		var ex struct {
			Artifacts []struct {
				Path string `json:"path"`
			} `json:"artifacts"`
		}
		if err := json.Unmarshal(jobMeta.RecoveryMetadata.Expectations, &ex); err == nil && len(ex.Artifacts) > 0 {
			existing := map[string]struct{}{}
			var paths []any
			if cur, ok := m["artifact_paths"]; ok && cur != nil {
				switch vv := cur.(type) {
				case []any:
					for _, item := range vv {
						if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
							existing[s] = struct{}{}
							paths = append(paths, s)
						}
					}
				}
			}
			for _, artifact := range ex.Artifacts {
				p := strings.TrimSpace(artifact.Path)
				if p == "" {
					continue
				}
				if _, ok := existing[p]; ok {
					continue
				}
				existing[p] = struct{}{}
				paths = append(paths, p)
			}
			if len(paths) > 0 {
				m["artifact_paths"] = paths
			}
		}
	}
	return nil
}

func applyHealingSchemaMutator(m map[string]any, job store.Job, jobType domaintypes.JobType) error {
	if jobType != domaintypes.JobTypeHeal {
		return nil
	}
	if len(job.Meta) == 0 {
		return nil
	}
	jobMeta, err := contracts.UnmarshalJobMeta(job.Meta)
	if err != nil || jobMeta.RecoveryMetadata == nil {
		return nil
	}
	kind, ok := contracts.ParseRecoveryErrorKind(jobMeta.RecoveryMetadata.ErrorKind)
	if !ok || !contracts.IsInfraRecoveryErrorKind(kind) {
		return nil
	}

	schemaRaw, err := contracts.ReadGateProfileSchemaJSON()
	if err != nil {
		return err
	}
	if !json.Valid(schemaRaw) {
		return fmt.Errorf("gate profile schema JSON is invalid")
	}

	env, err := ensureObjectField(m, "env", "spec")
	if err != nil {
		return err
	}
	env[contracts.GateProfileSchemaJSONEnv] = string(schemaRaw)
	return nil
}
