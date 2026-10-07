package step

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/client"
)

// DockerJobOwner fences cleanup to one execution, including retries of a job.
type DockerJobOwner struct {
	RunID       types.RunID
	JobID       types.JobID
	ResumeCount int
}

func DockerJobOwnerFromLabels(labels map[string]string) (DockerJobOwner, error) {
	owner := DockerJobOwner{RunID: types.RunID(strings.TrimSpace(labels[types.LabelRunID])), JobID: types.JobID(strings.TrimSpace(labels[types.LabelJobID]))}
	if owner.RunID.IsZero() || owner.JobID.IsZero() {
		return owner, errors.New("Docker socket requires run and job ownership")
	}
	if raw := labels[types.LabelResumeCount]; raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return owner, errors.New("invalid Docker resource resume count")
		}
		owner.ResumeCount = n
	}
	return owner, nil
}

func (o DockerJobOwner) labels() map[string]string {
	return map[string]string{types.LabelRunID: o.RunID.String(), types.LabelJobID: o.JobID.String(), types.LabelResumeCount: strconv.Itoa(o.ResumeCount), types.LabelJobResource: "true"}
}

func (o DockerJobOwner) owns(labels map[string]string) bool {
	if labels[types.LabelJobResource] != "true" {
		return false
	}
	other, err := DockerJobOwnerFromLabels(labels)
	return err == nil && o == other
}

type DockerJobResourceClient interface {
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	VolumeList(context.Context, client.VolumeListOptions) (client.VolumeListResult, error)
	VolumeInspect(context.Context, string, client.VolumeInspectOptions) (client.VolumeInspectResult, error)
	VolumeRemove(context.Context, string, client.VolumeRemoveOptions) (client.VolumeRemoveResult, error)
}

type dockerJobResource struct {
	kind   string
	id     string
	labels map[string]string
}

func listDockerJobResources(ctx context.Context, docker DockerJobResourceClient) ([]dockerJobResource, error) {
	filters := make(client.Filters).Add("label", types.LabelJobResource+"=true")
	containers, err := docker.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filters})
	if err != nil {
		return nil, fmt.Errorf("list job containers: %w", err)
	}
	var resources []dockerJobResource
	for _, c := range containers.Items {
		resources = append(resources, dockerJobResource{"container", c.ID, c.Labels})
	}
	networks, err := docker.NetworkList(ctx, client.NetworkListOptions{Filters: filters})
	if err != nil {
		return resources, fmt.Errorf("list job networks: %w", err)
	}
	for _, n := range networks.Items {
		resources = append(resources, dockerJobResource{"network", n.ID, n.Labels})
	}
	volumes, err := docker.VolumeList(ctx, client.VolumeListOptions{Filters: filters})
	if err != nil {
		return resources, fmt.Errorf("list job volumes: %w", err)
	}
	for _, v := range volumes.Items {
		resources = append(resources, dockerJobResource{"volume", v.Name, v.Labels})
	}
	return resources, nil
}

func ListDockerJobOwners(ctx context.Context, docker DockerJobResourceClient) ([]DockerJobOwner, error) {
	resources, err := listDockerJobResources(ctx, docker)
	owners := make(map[DockerJobOwner]bool)
	for _, resource := range resources {
		if resource.labels[types.LabelJobResource] != "true" {
			continue
		}
		owner, parseErr := DockerJobOwnerFromLabels(resource.labels)
		if parseErr == nil {
			owners[owner] = true
		}
	}
	result := make([]DockerJobOwner, 0, len(owners))
	for owner := range owners {
		result = append(result, owner)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].RunID != result[j].RunID {
			return result[i].RunID < result[j].RunID
		}
		if result[i].JobID != result[j].JobID {
			return result[i].JobID < result[j].JobID
		}
		return result[i].ResumeCount < result[j].ResumeCount
	})
	return result, err
}

// RemoveDockerJobResources requires the owner to have stopped creating resources.
// It never adopts external resources or force-removes volumes used elsewhere.
func RemoveDockerJobResources(ctx context.Context, docker DockerJobResourceClient, owner DockerJobOwner) error {
	resources, listErr := listDockerJobResources(ctx, docker)
	errs := []error{listErr}
	// Containers precede their networks and volumes so attachments are released.
	for _, resource := range resources {
		if !owner.owns(resource.labels) {
			continue
		}
		var err error
		switch resource.kind {
		case "container":
			_, err = docker.ContainerRemove(ctx, resource.id, client.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		case "network":
			_, err = docker.NetworkRemove(ctx, resource.id, client.NetworkRemoveOptions{})
		case "volume":
			// Names can be reused after a volume is removed; recheck its current owner.
			var inspected client.VolumeInspectResult
			inspected, err = docker.VolumeInspect(ctx, resource.id, client.VolumeInspectOptions{})
			if err == nil && owner.owns(inspected.Volume.Labels) {
				_, err = docker.VolumeRemove(ctx, resource.id, client.VolumeRemoveOptions{})
			}
		}
		if err != nil && !isContainerNotFound(err) {
			errs = append(errs, fmt.Errorf("remove job %s %s: %w", resource.kind, resource.id, err))
		}
	}
	return errors.Join(errs...)
}
