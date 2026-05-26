package nodeagent

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestExecuteNodeMaintenanceAction(t *testing.T) {
	cases := []struct {
		name        string
		actionType  string
		wantCommand string
		wantErr     bool
	}{
		{name: "cleanup disk", actionType: types.NodeActionCleanupDisk, wantCommand: "run_cleanup_cycle"},
		{name: "update updater", actionType: types.NodeActionUpdateUpdater, wantCommand: "maybe_update_self"},
		{name: "unsupported", actionType: "node.shell", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &nodeActionDockerFake{}
			old := newNodeActionDockerExecAPI
			newNodeActionDockerExecAPI = func() (step.DockerExecAPI, error) { return fake, nil }
			t.Cleanup(func() { newNodeActionDockerExecAPI = old })

			_, err := executeNodeMaintenanceAction(context.Background(), tc.actionType)
			if tc.wantErr {
				if err == nil {
					t.Fatal("executeNodeMaintenanceAction() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("executeNodeMaintenanceAction() error = %v", err)
			}
			if !strings.Contains(fake.execCreateOpts.Cmd[2], tc.wantCommand) {
				t.Fatalf("exec command %q does not contain %q", fake.execCreateOpts.Cmd[2], tc.wantCommand)
			}
		})
	}
}

type nodeActionDockerFake struct {
	execCreateOpts client.ExecCreateOptions
}

func (f *nodeActionDockerFake) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	return client.ContainerListResult{Items: []container.Summary{{ID: "node-updater"}}}, nil
}

func (f *nodeActionDockerFake) ExecCreate(ctx context.Context, containerID string, options client.ExecCreateOptions) (client.ExecCreateResult, error) {
	f.execCreateOpts = options
	return client.ExecCreateResult{ID: "exec-1"}, nil
}

func (f *nodeActionDockerFake) ExecAttach(ctx context.Context, execID string, options client.ExecAttachOptions) (client.ExecAttachResult, error) {
	return client.ExecAttachResult{
		HijackedResponse: client.HijackedResponse{
			Reader: bufio.NewReader(bytes.NewReader(nil)),
		},
	}, nil
}

func (f *nodeActionDockerFake) ExecInspect(ctx context.Context, execID string, options client.ExecInspectOptions) (client.ExecInspectResult, error) {
	return client.ExecInspectResult{ExitCode: 0}, nil
}
