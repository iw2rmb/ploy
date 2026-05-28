package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func TestDockerContainerRuntimeCreate_ImagePullAuthRefreshRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		pullErrs         []error
		pullWaitErrs     []error
		refreshResponse  registryAuthRefreshResponse
		wantRefresh      bool
		wantPulls        int
		wantErrContains  string
		wantCreateCalled bool
	}{
		{
			name: "unauthorized_pull_error_refreshes_and_retries",
			pullErrs: []error{
				errors.New("error from registry: unauthorized unauthorized"),
				nil,
			},
			refreshResponse:  registryAuthRefreshResponse{OK: true},
			wantRefresh:      true,
			wantPulls:        2,
			wantCreateCalled: true,
		},
		{
			name: "unauthorized_wait_error_refreshes_and_retries",
			pullWaitErrs: []error{
				errors.New("no basic auth credentials"),
				nil,
			},
			refreshResponse:  registryAuthRefreshResponse{OK: true},
			wantRefresh:      true,
			wantPulls:        2,
			wantCreateCalled: true,
		},
		{
			name:            "non_auth_pull_error_does_not_refresh",
			pullErrs:        []error{errors.New("manifest unknown")},
			wantPulls:       1,
			wantErrContains: "pull image",
		},
		{
			name:             "refresh_failure_stops_before_retry",
			pullErrs:         []error{errors.New("authentication required")},
			refreshResponse:  registryAuthRefreshResponse{OK: false, Error: "dp unavailable"},
			wantRefresh:      true,
			wantPulls:        1,
			wantErrContains:  "refresh registry auth",
			wantCreateCalled: false,
		},
		{
			name: "retry_failure_is_reported",
			pullErrs: []error{
				errors.New("authentication required"),
				errors.New("still unauthorized"),
			},
			refreshResponse: registryAuthRefreshResponse{OK: true},
			wantRefresh:     true,
			wantPulls:       2,
			wantErrContains: "after registry auth refresh",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var refreshRequests chan registryAuthRefreshRequest
			socketPath := ""
			if tt.wantRefresh {
				refreshRequests = make(chan registryAuthRefreshRequest, 1)
				socketPath = startRegistryAuthRefreshTestServer(t, tt.refreshResponse, refreshRequests)
			}

			fake := &fakeDockerClient{
				createResult: client.ContainerCreateResult{ID: "container-with-image"},
				pullErrs:     tt.pullErrs,
				pullWaitErrs: tt.pullWaitErrs,
			}
			rt := newDockerContainerRuntimeWithClient(fake, DockerContainerRuntimeOptions{
				PullImage:                 true,
				RegistryAuthRefreshSocket: socketPath,
			})

			_, err := rt.Create(context.Background(), ContainerSpec{
				Image: "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy/gate-gradle-jdk17:latest",
			})

			if tt.wantErrContains != "" {
				requireErrContains(t, err, tt.wantErrContains)
			} else if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if fake.pullCalls != tt.wantPulls {
				t.Fatalf("image pull calls = %d, want %d", fake.pullCalls, tt.wantPulls)
			}
			if fake.createCalled != tt.wantCreateCalled {
				t.Fatalf("create called = %v, want %v", fake.createCalled, tt.wantCreateCalled)
			}
			if tt.wantRefresh {
				req := <-refreshRequests
				if req.RegistryHost != "docker-hosted.artifactory.tcsbank.ru" {
					t.Fatalf("refresh registry host = %q", req.RegistryHost)
				}
				if req.ValidationImage != "docker-hosted.artifactory.tcsbank.ru/at-scale/ploy/gate-gradle-jdk17:latest" {
					t.Fatalf("refresh validation image = %q", req.ValidationImage)
				}
			}
		})
	}
}

func startRegistryAuthRefreshTestServer(
	t *testing.T,
	response registryAuthRefreshResponse,
	requests chan<- registryAuthRefreshRequest,
) string {
	t.Helper()

	socketPath := fmt.Sprintf("%s/ploy-auth-%d.sock", os.TempDir(), time.Now().UnixNano())
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var req registryAuthRefreshRequest
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			return
		}
		requests <- req
		_ = json.NewEncoder(conn).Encode(response)
	}()

	return socketPath
}
