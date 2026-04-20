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

	release := strings.TrimSpace(env[contracts.PLOYStackReleaseEnv])
	tools := []string{"maven", "gradle"}
	mounts := make([]ContainerMount, 0, len(tools))
	for _, tool := range tools {
		toolMounts, err := buildGateToolCacheMounts("java", tool, release)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, toolMounts...)
	}
	return mounts, nil
}
