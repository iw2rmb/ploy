// mods_spec_wire.go provides wire serialization for Mods specifications.
//
// The ToMap method converts a typed ModsSpec back to map[string]any for
// wire transmission through systems that expect untyped map representations.
// The resulting map can be serialized to JSON for control plane submission.
//
// Wire compatibility is maintained by:
//   - Using the same field names as the JSON tags on ModsSpec
//   - Omitting empty/nil fields to match omitempty behavior
//   - Preserving polymorphic field forms (string vs map for images)
package contracts

// ToMap converts the ModsSpec to a map[string]any for wire serialization.
// This is useful when the spec needs to be passed through systems that
// expect untyped map representations.
//
// The result can be serialized to JSON for control plane submission.
func (s ModsSpec) ToMap() map[string]any {
	result := make(map[string]any)

	// Server-injected metadata.
	if !s.JobID.IsZero() {
		result["job_id"] = s.JobID.String()
	}

	// Metadata.
	if s.APIVersion != "" {
		result["apiVersion"] = s.APIVersion
	}
	if s.Kind != "" {
		result["kind"] = s.Kind
	}

	if len(s.Env) > 0 {
		result["env"] = s.Env
	}

	// Steps.
	if len(s.Steps) > 0 {
		steps := make([]map[string]any, 0, len(s.Steps))
		for _, step := range s.Steps {
			steps = append(steps, modStepToMap(step))
		}
		result["steps"] = steps
	}

	// Build gate.
	if s.BuildGate != nil {
		bg := make(map[string]any)
		if s.BuildGate.Enabled {
			bg["enabled"] = true
		}
		if s.BuildGate.Profile != "" {
			bg["profile"] = s.BuildGate.Profile
		}
		if s.BuildGate.Healing != nil {
			heal := make(map[string]any)
			if s.BuildGate.Healing.Retries > 0 {
				heal["retries"] = s.BuildGate.Healing.Retries
			}
			if s.BuildGate.Healing.Mod != nil {
				heal["mod"] = healingModToMap(s.BuildGate.Healing.Mod)
			}
			if len(heal) > 0 {
				bg["healing"] = heal
			}
		}
		if len(s.BuildGate.Images) > 0 {
			bg["images"] = buildGateImageRulesToAny(s.BuildGate.Images)
		}
		if len(bg) > 0 {
			result["build_gate"] = bg
		}
	}

	// GitLab.
	if s.GitLabPAT != "" {
		result["gitlab_pat"] = s.GitLabPAT
	}
	if s.GitLabDomain != "" {
		result["gitlab_domain"] = s.GitLabDomain
	}
	if s.MROnSuccess != nil {
		result["mr_on_success"] = *s.MROnSuccess
	}
	if s.MROnFail != nil {
		result["mr_on_fail"] = *s.MROnFail
	}

	// Artifacts.
	if s.ArtifactName != "" {
		result["artifact_name"] = s.ArtifactName
	}
	if len(s.ArtifactPaths) > 0 {
		result["artifact_paths"] = s.ArtifactPaths
	}

	return result
}

// modImageToAny converts ModImage to its wire representation.
func modImageToAny(img ModImage) any {
	if img.Universal != "" {
		return img.Universal
	}
	if len(img.ByStack) > 0 {
		result := make(map[string]string, len(img.ByStack))
		for k, v := range img.ByStack {
			result[string(k)] = v
		}
		return result
	}
	return nil
}

// commandSpecToAny converts CommandSpec to its wire representation.
func commandSpecToAny(cmd CommandSpec) any {
	if len(cmd.Exec) > 0 {
		return cmd.Exec
	}
	if cmd.Shell != "" {
		return cmd.Shell
	}
	return nil
}

// modLikeFieldsToMap serializes the common fields shared by ModStep and HealingModSpec.
func modLikeFieldsToMap(img ModImage, cmd CommandSpec, env map[string]string, retain bool) map[string]any {
	result := make(map[string]any)
	if !img.IsEmpty() {
		result["image"] = modImageToAny(img)
	}
	if !cmd.IsEmpty() {
		result["command"] = commandSpecToAny(cmd)
	}
	if len(env) > 0 {
		result["env"] = env
	}
	if retain {
		result["retain_container"] = true
	}
	return result
}

// modStepToMap converts ModStep to a map for wire serialization.
func modStepToMap(mod ModStep) map[string]any {
	result := modLikeFieldsToMap(mod.Image, mod.Command, mod.Env, mod.RetainContainer)
	if mod.Name != "" {
		result["name"] = mod.Name
	}
	if mod.Stack != nil && !mod.Stack.IsEmpty() {
		result["stack"] = stackGateSpecToMap(mod.Stack)
	}
	return result
}

// healingModToMap converts HealingModSpec to a map for wire serialization.
func healingModToMap(mod *HealingModSpec) map[string]any {
	return modLikeFieldsToMap(mod.Image, mod.Command, mod.Env, mod.RetainContainer)
}
