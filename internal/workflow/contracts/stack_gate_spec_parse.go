// stack_gate_spec_parse.go provides parsing functions for Stack Gate configuration.
//
// These functions parse Stack Gate specs from map[string]any intermediate
// representations (from JSON/YAML input). They use the existing expect*
// helpers from parse_helpers.go for consistent error handling.
package contracts

import (
	"strings"
)

// parseStackGateSpec parses a StackGateSpec from a raw map.
// Returns nil if the input is nil or empty (no stack configuration).
func parseStackGateSpec(raw map[string]any, prefix string) (*StackGateSpec, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	spec := &StackGateSpec{}

	// Parse inbound.
	if v, ok := raw["inbound"]; ok && v != nil {
		inboundRaw, err := expectMap(v, prefix+".inbound")
		if err != nil {
			return nil, err
		}
		inbound, err := parseStackGatePhaseSpec(inboundRaw, prefix+".inbound")
		if err != nil {
			return nil, err
		}
		spec.Inbound = inbound
	}

	// Parse outbound.
	if v, ok := raw["outbound"]; ok && v != nil {
		outboundRaw, err := expectMap(v, prefix+".outbound")
		if err != nil {
			return nil, err
		}
		outbound, err := parseStackGatePhaseSpec(outboundRaw, prefix+".outbound")
		if err != nil {
			return nil, err
		}
		spec.Outbound = outbound
	}

	// Return nil if no configuration was present.
	if spec.IsEmpty() {
		return nil, nil
	}

	return spec, nil
}

// parseStackGatePhaseSpec parses a StackGatePhaseSpec from a raw map.
func parseStackGatePhaseSpec(raw map[string]any, prefix string) (*StackGatePhaseSpec, error) {
	if raw == nil {
		return nil, nil
	}

	phase := &StackGatePhaseSpec{}

	// Parse enabled.
	if v, ok := raw["enabled"]; ok && v != nil {
		b, err := expectBool(v, prefix+".enabled")
		if err != nil {
			return nil, err
		}
		phase.Enabled = b
	}

	// Parse expect.
	if v, ok := raw["expect"]; ok && v != nil {
		expectRaw, err := expectMap(v, prefix+".expect")
		if err != nil {
			return nil, err
		}
		exp, err := parseStackExpectation(expectRaw, prefix+".expect")
		if err != nil {
			return nil, err
		}
		phase.Expect = exp
	}

	return phase, nil
}

// parseStackExpectation parses a StackExpectation from a raw map.
// Handles numeric release values (YAML `release: 11` parses as float64 or int).
func parseStackExpectation(raw map[string]any, prefix string) (*StackExpectation, error) {
	if raw == nil {
		return nil, nil
	}

	exp := &StackExpectation{}

	// Parse language.
	if v, ok := raw["language"]; ok && v != nil {
		s, err := expectString(v, prefix+".language")
		if err != nil {
			return nil, err
		}
		exp.Language = strings.TrimSpace(s)
	}

	// Parse tool.
	if v, ok := raw["tool"]; ok && v != nil {
		s, err := expectString(v, prefix+".tool")
		if err != nil {
			return nil, err
		}
		exp.Tool = strings.TrimSpace(s)
	}

	// Parse release. Handle both string and numeric values.
	// YAML `release: 11` becomes float64 in JSON, but we want "11" as string.
	if v, ok := raw["release"]; ok && v != nil {
		release, err := ParseReleaseValue(v, prefix+".release")
		if err != nil {
			return nil, err
		}
		exp.Release = release
	}

	return exp, nil
}
