// stack_gate_spec_wire.go provides wire serialization for Stack Gate configuration.
//
// These functions convert typed Stack Gate specs back to map[string]any for
// wire transmission. Only non-empty fields are included (omitempty semantics).
package contracts

// stackGateSpecToMap converts a StackGateSpec to map[string]any for wire serialization.
// Returns nil if the spec is nil or empty.
func stackGateSpecToMap(spec *StackGateSpec) map[string]any {
	if spec == nil || spec.IsEmpty() {
		return nil
	}

	result := make(map[string]any)

	if spec.Inbound != nil && !spec.Inbound.IsEmpty() {
		result["inbound"] = stackGatePhaseSpecToMap(spec.Inbound)
	}

	if spec.Outbound != nil && !spec.Outbound.IsEmpty() {
		result["outbound"] = stackGatePhaseSpecToMap(spec.Outbound)
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// stackGatePhaseSpecToMap converts a StackGatePhaseSpec to map[string]any.
// Returns nil if the phase spec is nil.
func stackGatePhaseSpecToMap(phase *StackGatePhaseSpec) map[string]any {
	if phase == nil {
		return nil
	}

	result := make(map[string]any)

	if phase.Enabled {
		result["enabled"] = true
	}

	if phase.Expect != nil && !phase.Expect.IsEmpty() {
		result["expect"] = stackExpectationToMap(phase.Expect)
	}

	return result
}

// stackExpectationToMap converts a StackExpectation to map[string]any.
// Returns nil if the expectation is nil or empty.
func stackExpectationToMap(exp *StackExpectation) map[string]any {
	if exp == nil || exp.IsEmpty() {
		return nil
	}

	result := make(map[string]any)

	if exp.Language != "" {
		result["language"] = exp.Language
	}

	if exp.Tool != "" {
		result["tool"] = exp.Tool
	}

	if exp.Release != "" {
		result["release"] = exp.Release
	}

	return result
}
