package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func TestContainerRuntime_DockerProxyUsesJobMountsAndClosesWithContainer(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"wait", "wait failure", "wait cancelled", "wait error", "start error", "create error", "create panic", "runtime close"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			fake := &fakeDockerClient{createResult: client.ContainerCreateResult{ID: outcome}}
			switch outcome {
			case "wait failure":
				fake.waitStatusCode = 1
			case "wait cancelled":
				fake.waitErr = context.Canceled
			case "wait error":
				fake.waitErr = errors.New("wait failed")
			case "start error":
				fake.startErr = errors.New("start failed")
			case "create error":
				fake.createErr = errors.New("create failed")
			case "create panic":
				fake.createPanic = "create panic"
			}
			root := t.TempDir()
			mounts := []ContainerMount{
				{Source: filepath.Join(root, "workspace"), Target: "/workspace"},
				{Source: root, Target: "/tmp"},
				{Source: "/host/docker.sock", Target: "/var/run/docker.sock"},
			}
			rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{RegistryAuthConfigFile: "/host/auth/config.json"})
			closed := 0
			socketPath := filepath.Join(root, "socket")
			rt.startDockerProxy = func(_ context.Context, projection []ContainerMount, upstream, tmpDir string, owner DockerJobOwner) (*dockerSocketProxy, error) {
				if upstream != "/host/docker.sock" || tmpDir != root {
					t.Fatalf("proxy received wrong upstream/storage: %s %s", upstream, tmpDir)
				}
				if len(projection) != 4 || projection[0] != mounts[0] || projection[3].Target != "/root/.docker" {
					t.Fatalf("proxy must use existing mounts plus Docker credentials: %v", projection)
				}
				return &dockerSocketProxy{socketPath: socketPath, close: func() error { closed++; return nil }}, nil
			}
			handle, err := rt.Create(context.Background(), ContainerSpec{Image: "job", Labels: testDockerJobOwner.labels(), Mounts: mounts})
			if outcome == "create error" || outcome == "create panic" {
				if err == nil || closed != 1 {
					t.Fatalf("failed creation must close proxy: err=%v closed=%d", err, closed)
				}
				return
			}
			if err != nil || closed != 0 {
				t.Fatalf("successful creation must retain proxy: err=%v closed=%d", err, closed)
			}
			if fake.createOpts.HostConfig.Mounts[2].Source != socketPath || mounts[2].Source != "/host/docker.sock" {
				t.Fatal("job must get proxy socket without mutating the authoritative mount table")
			}
			switch outcome {
			case "wait", "wait failure", "wait cancelled", "wait error":
				_, _ = rt.Wait(context.Background(), handle)
			case "start error":
				_ = rt.Start(context.Background(), handle)
			case "runtime close":
				_ = rt.Close()
			}
			_ = rt.Close()
			if closed != 1 {
				t.Fatalf("proxy must close exactly once: %d", closed)
			}
		})
	}
}

func TestContainerRuntime_DockerProxyPreparationFailureStopsContainerCreation(t *testing.T) {
	t.Parallel()
	fake := &fakeDockerClient{}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{})
	rt.startDockerProxy = func(context.Context, []ContainerMount, string, string, DockerJobOwner) (*dockerSocketProxy, error) {
		return nil, errors.New("listen failed")
	}
	_, err := rt.Create(context.Background(), ContainerSpec{Image: "job", Labels: testDockerJobOwner.labels(), Mounts: []ContainerMount{
		{Source: t.TempDir(), Target: "/tmp"},
		{Source: "/host/docker.sock", Target: "/var/run/docker.sock"},
	}})
	if err == nil || !strings.Contains(err.Error(), "listen failed") || fake.createCalled {
		t.Fatalf("proxy failure must stop setup: err=%v created=%v", err, fake.createCalled)
	}
}

func TestContainerRuntime_DockerProxyHonorsCustomUnixEndpoint(t *testing.T) {
	t.Parallel()
	fake := &fakeDockerClient{createResult: client.ContainerCreateResult{ID: "custom"}}
	rt := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{})
	t.Cleanup(func() { _ = rt.Close() })
	rt.startDockerProxy = func(_ context.Context, _ []ContainerMount, upstream, _ string, _ DockerJobOwner) (*dockerSocketProxy, error) {
		if upstream != "/host/custom.sock" {
			t.Fatalf("upstream=%s", upstream)
		}
		return &dockerSocketProxy{socketPath: "/job/proxy.sock", close: func() error { return nil }}, nil
	}
	_, err := rt.Create(context.Background(), ContainerSpec{
		Image: "job", Labels: testDockerJobOwner.labels(),
		Env: map[string]string{"DOCKER_HOST": "unix:///custom/docker.sock"},
		Mounts: []ContainerMount{
			{Source: t.TempDir(), Target: "/tmp"},
			{Source: "/host/custom.sock", Target: "/custom/docker.sock"},
		},
	})
	if err != nil || fake.createOpts.HostConfig.Mounts[1].Source != "/job/proxy.sock" {
		t.Fatalf("custom Docker endpoint was not proxied: err=%v mounts=%v", err, fake.createOpts.HostConfig)
	}
}

func TestDockerSocketProxyUnix_TranslatesRealRequestsAndClosesOnCancellation(t *testing.T) {
	// A short root also works on macOS; production Linux job paths are longer.
	root, err := os.MkdirTemp("/tmp", "ploy-dp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	upstreamPath := filepath.Join(root, "host.sock")
	listener, err := net.Listen("unix", upstreamPath)
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 1)
	daemon := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodHead {
			w.Header().Set("API-Version", "1.52")
			return
		}
		if req.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			if strings.HasSuffix(req.URL.Path, "/volumes") {
				_, _ = io.WriteString(w, `{"Volumes":[]}`)
			} else {
				_, _ = io.WriteString(w, `[]`)
			}
			return
		}
		body, _ := io.ReadAll(req.Body)
		requests <- string(body)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"Id":"db"}`)
	})}
	go func() { _ = daemon.Serve(listener) }()
	t.Cleanup(func() { _ = daemon.Close() })
	if err := os.WriteFile(filepath.Join(root, "init.sql"), []byte("select 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxy, err := startDockerSocketProxy(ctx, []ContainerMount{
		{Source: root, Target: "/workspace"},
		{Source: upstreamPath, Target: "/var/run/docker.sock"},
	}, upstreamPath, root, testDockerJobOwner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", proxy.socketPath)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := httpClient.Post("http://docker/v1.52/containers/create", "application/json", strings.NewReader(`{"HostConfig":{"Binds":["/workspace/init.sql:/init.sql:ro","/var/run/docker.sock:/var/run/docker.sock"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d", response.StatusCode)
	}
	select {
	case body := <-requests:
		if !strings.Contains(body, root+"/init.sql:/init.sql:ro") || !strings.Contains(body, proxy.socketPath+":/var/run/docker.sock") {
			t.Fatalf("daemon received wrong bind paths: %s", body)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not reach daemon")
	}
	cancel()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(proxy.socketPath); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancellation did not remove proxy socket")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := os.Stat(upstreamPath); err != nil {
		t.Fatalf("proxy cleanup removed the host socket: %v", err)
	}
}

func TestDockerSocketProxyUnix_LongLinuxJobPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux descriptor paths are unavailable")
	}
	dir := filepath.Join(t.TempDir(), strings.Repeat("job-", 40))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := listenDockerProxySocket(dir, filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	info, err := os.Stat(filepath.Join(dir, "s"))
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("long job path must contain the listening socket: info=%v err=%v", info, err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	conn, err := net.Dial("unix", fmt.Sprintf("/proc/self/fd/%d/s", directory.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
}
