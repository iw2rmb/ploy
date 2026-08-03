package run

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/speccompiler"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

type buildGateForcedOverrides = speccompiler.BuildGateForcedOverrides

func parseBuildGateForcedFlags(
	globalChanged bool,
	globalValue string,
	preChanged bool,
	preValue string,
	postChanged bool,
	postValue string,
) (buildGateForcedOverrides, error) {
	if globalChanged && (preChanged || postChanged) {
		return buildGateForcedOverrides{}, errors.New("--build-gate-forced cannot be combined with --build-gate-forced-pre or --build-gate-forced-post")
	}

	var out buildGateForcedOverrides
	if globalChanged {
		stack, err := parseBuildGateForcedValue("--build-gate-forced", globalValue)
		if err != nil {
			return buildGateForcedOverrides{}, err
		}
		out.Pre = stack
		out.Post = cloneBuildGateForcedStack(stack)
		return out, nil
	}
	if preChanged {
		stack, err := parseBuildGateForcedValue("--build-gate-forced-pre", preValue)
		if err != nil {
			return buildGateForcedOverrides{}, err
		}
		out.Pre = stack
	}
	if postChanged {
		stack, err := parseBuildGateForcedValue("--build-gate-forced-post", postValue)
		if err != nil {
			return buildGateForcedOverrides{}, err
		}
		out.Post = stack
	}
	return out, nil
}

func parseBuildGateForcedValue(flagName string, raw string) (*contracts.BuildGateStackConfig, error) {
	raw = strings.TrimSpace(raw)
	syntaxErr := func() (*contracts.BuildGateStackConfig, error) {
		return nil, fmt.Errorf("invalid %s value %q: expected <lang>@<release>[/<tool>]", flagName, raw)
	}
	if strings.Count(raw, "@") != 1 {
		return syntaxErr()
	}
	language, releaseAndTool, _ := strings.Cut(raw, "@")
	if strings.Count(releaseAndTool, "/") > 1 {
		return syntaxErr()
	}
	release, tool, hasTool := strings.Cut(releaseAndTool, "/")
	language = strings.TrimSpace(language)
	release = strings.TrimSpace(release)
	tool = strings.TrimSpace(tool)
	if language == "" || release == "" || (hasTool && tool == "") {
		return syntaxErr()
	}
	return &contracts.BuildGateStackConfig{
		Mode:     contracts.BuildGateStackModeForced,
		Language: language,
		Tool:     tool,
		Release:  release,
	}, nil
}

func cloneBuildGateForcedStack(stack *contracts.BuildGateStackConfig) *contracts.BuildGateStackConfig {
	if stack == nil {
		return nil
	}
	clone := *stack
	return &clone
}
