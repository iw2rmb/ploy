package orchestration

import (
	"fmt"

	nomadapi "github.com/hashicorp/nomad/api"
)

type sdkNomadAdapter struct{ client *nomadapi.Client }

func newSDKNomadAdapter() *sdkNomadAdapter {
	// Reuse orchestration client helper to ensure RetryTransport is installed
	c, _ := newNomadClient()
	return &sdkNomadAdapter{client: c}
}

func (a *sdkNomadAdapter) ListAllocations(jobID string) ([]*AllocationStatus, error) {
	if a.client == nil {
		return nil, fmt.Errorf("nomad client unavailable")
	}
	allocs, _, err := a.client.Jobs().Allocations(jobID, false, nil)
	if err != nil {
		return nil, err
	}
	out := make([]*AllocationStatus, 0, len(allocs))
	for _, al := range allocs {
		st := &AllocationStatus{ID: al.ID, ClientStatus: al.ClientStatus, DesiredStatus: al.DesiredStatus}
		if al.DeploymentStatus != nil {
			ds := &AllocDeploymentStatus{}
			ds.Healthy = al.DeploymentStatus.Healthy
			st.DeploymentStatus = ds
		}
		out = append(out, st)
	}
	return out, nil
}

func (a *sdkNomadAdapter) AllocationEndpoint(allocID string) (string, error) {
	if a.client == nil {
		return "", fmt.Errorf("nomad client unavailable")
	}
	alloc, _, err := a.client.Allocations().Info(allocID, nil)
	if err != nil {
		return "", err
	}
	if alloc == nil || alloc.Resources == nil || len(alloc.Resources.Networks) == 0 {
		return "", fmt.Errorf("no network")
	}
	net := alloc.Resources.Networks[0]
	ip := net.IP
	var port int
	if len(net.DynamicPorts) > 0 {
		port = net.DynamicPorts[0].Value
	}
	if ip == "" || port == 0 {
		return "", fmt.Errorf("no endpoint")
	}
	return fmt.Sprintf("http://%s:%d", ip, port), nil
}
