package step

import (
	"strings"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func buildJavaToolCacheMountsFromStackEnv(env map[string]string) ([]ContainerMount, error) {
	if len(env) == 0 {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(env[contracts.PLOYStackLanguageEnv]), "java") {
		return nil, nil
	}
	tool := strings.ToLower(strings.TrimSpace(env[contracts.PLOYStackToolEnv]))
	switch tool {
	case "gradle", "maven":
		return buildGateToolCacheMounts("java", tool, strings.TrimSpace(env[contracts.PLOYStackReleaseEnv]))
	default:
		return nil, nil
	}
}
