package nodeagent

import "github.com/moby/moby/client"

func installNoopStartupReconciler(claimer *ClaimManager) {
	if claimer == nil {
		return
	}
	claimer.startupReconciler = &startupCrashReconciler{
		docker: &fakeDockerClient{
			listResult: client.ContainerListResult{},
		},
	}
}
