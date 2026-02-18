package step

import (
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type buildGateImageMappingError struct {
	err error
}

func (e *buildGateImageMappingError) Error() string { return e.err.Error() }
func (e *buildGateImageMappingError) Unwrap() error { return e.err }

type buildGateImageRuleMatchError struct {
	err error
}

func (e *buildGateImageRuleMatchError) Error() string { return e.err.Error() }
func (e *buildGateImageRuleMatchError) Unwrap() error { return e.err }

func resolveImageForExpectation(
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	exp contracts.StackExpectation,
	required bool,
) (string, error) {
	resolver, err := NewBuildGateImageResolver(mappingPath, overrides, required)
	if err != nil {
		return "", &buildGateImageMappingError{err: err}
	}
	resolved, err := resolver.Resolve(exp)
	if err != nil {
		return "", &buildGateImageRuleMatchError{err: err}
	}
	return resolved, nil
}

func resolveExpectedRuntimeImageForStackGate(
	envImage string,
	mappingPath string,
	overrides []contracts.BuildGateImageRule,
	expect *contracts.StackExpectation,
) (string, error) {
	if envImage != "" {
		return envImage, nil
	}
	if expect == nil || strings.TrimSpace(expect.Release) == "" {
		return "", fmt.Errorf("stack gate expectation missing release")
	}
	return resolveImageForExpectation(mappingPath, overrides, *expect, true)
}
