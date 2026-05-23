package step

import (
	"context"
	"errors"
	"strings"
	"testing"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestDockerContainerRuntimeCreate_DelegatedAuthPull(t *testing.T) {
	t.Parallel()

	const imageRef = "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy/gate-maven:jdk17"
	updaterList := client.ContainerListResult{
		Items: []container.Summary{{ID: "updater-container"}},
	}

	testCases := []struct {
		name                 string
		image                string
		registry             string
		pullErr              error
		inspectErrs          []error
		containerListResults []client.ContainerListResult
		execInspectResult    client.ExecInspectResult
		execOutput           string
		wantErrContains      string
		wantExec             bool
		wantCreate           bool
		wantInspectCalls     int
		wantListCalls        int
	}{
		{
			name:             "normal pull succeeds without updater exec",
			image:            imageRef,
			registry:         "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy",
			inspectErrs:      []error{cerrdefs.ErrNotFound},
			wantCreate:       true,
			wantInspectCalls: 1,
		},
		{
			name:                 "auth error matching registry delegates pull and creates container",
			image:                imageRef,
			registry:             "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy",
			pullErr:              errors.New("unauthorized: authorization header required"),
			inspectErrs:          []error{cerrdefs.ErrNotFound, nil},
			containerListResults: []client.ContainerListResult{updaterList},
			execInspectResult:    client.ExecInspectResult{ExitCode: 0},
			execOutput:           "pull complete",
			wantExec:             true,
			wantCreate:           true,
			wantInspectCalls:     2,
			wantListCalls:        1,
		},
		{
			name:                 "auth error without updater returns original pull failure with fallback context",
			image:                imageRef,
			registry:             "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy",
			pullErr:              errors.New("authentication required"),
			inspectErrs:          []error{cerrdefs.ErrNotFound},
			containerListResults: []client.ContainerListResult{{}, {}},
			wantErrContains:      "delegated pull fallback failed",
			wantInspectCalls:     1,
			wantListCalls:        2,
		},
		{
			name:             "non auth pull error does not delegate",
			image:            imageRef,
			registry:         "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy",
			pullErr:          errors.New("net/http: TLS handshake timeout"),
			inspectErrs:      []error{cerrdefs.ErrNotFound},
			wantErrContains:  "pull image",
			wantInspectCalls: 1,
		},
		{
			name:             "auth error for different registry does not delegate",
			image:            "ghcr.io/iw2rmb/ploy/gate-maven:jdk17",
			registry:         "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy",
			pullErr:          errors.New("no basic auth credentials"),
			inspectErrs:      []error{cerrdefs.ErrNotFound},
			wantErrContains:  "pull image",
			wantInspectCalls: 1,
		},
		{
			name:                 "updater exec nonzero returns delegated failure with exit code",
			image:                imageRef,
			registry:             "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy",
			pullErr:              errors.New("denied requested access"),
			inspectErrs:          []error{cerrdefs.ErrNotFound},
			containerListResults: []client.ContainerListResult{updaterList},
			execInspectResult:    client.ExecInspectResult{ExitCode: 7},
			execOutput:           "pull denied",
			wantErrContains:      "exited with code 7",
			wantExec:             true,
			wantInspectCalls:     1,
			wantListCalls:        1,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeDockerClient{
				createResult:         client.ContainerCreateResult{ID: "job-container"},
				imageInspectErrs:     tc.inspectErrs,
				pullErr:              tc.pullErr,
				containerListResults: tc.containerListResults,
				execCreateResult:     client.ExecCreateResult{ID: "exec-1"},
				execAttachOutput:     tc.execOutput,
				execInspectResult:    tc.execInspectResult,
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
				PullImage:                 true,
				DelegatedAuthPullRegistry: tc.registry,
			})

			handle, err := rt.Create(context.Background(), ContainerSpec{Image: tc.image})

			if tc.wantErrContains != "" {
				requireErrContains(t, err, tc.wantErrContains)
			} else if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if tc.wantCreate {
				if string(handle) != "job-container" {
					t.Fatalf("handle = %q, want job-container", handle)
				}
				if !fake.createCalled {
					t.Fatal("ContainerCreate should have been called")
				}
			} else if fake.createCalled {
				t.Fatal("ContainerCreate should not have been called")
			}
			if fake.execCreateCalled != tc.wantExec {
				t.Fatalf("ExecCreate called = %v, want %v", fake.execCreateCalled, tc.wantExec)
			}
			if got := len(fake.imageInspectRefs); got != tc.wantInspectCalls {
				t.Fatalf("ImageInspect calls = %d, want %d", got, tc.wantInspectCalls)
			}
			if got := len(fake.containerListCalls); got != tc.wantListCalls {
				t.Fatalf("ContainerList calls = %d, want %d", got, tc.wantListCalls)
			}
			if !tc.wantExec {
				return
			}

			if fake.execContainer != "updater-container" {
				t.Fatalf("ExecCreate container = %q, want updater-container", fake.execContainer)
			}
			wantCmd := []string{
				"/usr/bin/bash",
				"-lc",
				`dp auth service-acc --key-file "${PLOY_DP_SERVICE_ACCOUNT_KEY_FILE:-/etc/ploy/dp.sa.json}" >/dev/null && docker pull "$1"`,
				"ploy-node-auth-pull",
				tc.image,
			}
			if got := fake.execCreateOpts.Cmd; strings.Join(got, "\x00") != strings.Join(wantCmd, "\x00") {
				t.Fatalf("ExecCreate Cmd = %#v, want %#v", got, wantCmd)
			}
			if strings.Contains(fake.execCreateOpts.Cmd[2], tc.image) {
				t.Fatalf("shell command must not embed image ref: %q", fake.execCreateOpts.Cmd[2])
			}
		})
	}
}
