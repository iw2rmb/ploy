package step

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/moby/moby/client"
)

type dockerSocketProxy struct {
	socketPath string
	close      func() error
	err        error
	once       sync.Once
}

func (p *dockerSocketProxy) Close() error {
	p.once.Do(func() { p.err = p.close() })
	return p.err
}

func (r *containerRuntime) prepareDockerSocket(ctx context.Context, spec ContainerSpec) ([]ContainerMount, *dockerSocketProxy, error) {
	mounts := append([]ContainerMount(nil), r.withDockerAuthMount(spec.Mounts)...)
	socketTarget := dockerHostSocketPathFromEnv(spec.Env)
	if socketTarget == "" {
		socketTarget = "/var/run/docker.sock"
	}
	socketIndex := -1
	tmpDir := ""
	for i, m := range mounts {
		if m.Target == socketTarget {
			socketIndex = i
		}
		if m.Target == jobTmpContainerDir {
			tmpDir = m.Source
		}
	}
	if socketIndex < 0 {
		return mounts, nil, nil
	}
	if tmpDir == "" {
		return nil, nil, errors.New("Docker-socket jobs require job-owned /tmp storage")
	}
	start := r.startDockerProxy
	if start == nil {
		start = startDockerSocketProxy
	}
	owner, err := DockerJobOwnerFromLabels(spec.Labels)
	if err != nil {
		return nil, nil, err
	}
	proxy, err := start(ctx, mounts, mounts[socketIndex].Source, tmpDir, owner)
	if err != nil {
		return nil, nil, err
	}
	mounts[socketIndex].Source = proxy.socketPath
	return mounts, proxy, nil
}

func (r *containerRuntime) closeDockerProxy(handle ContainerHandle) error {
	if r == nil {
		return nil
	}
	if p, ok := r.dockerProxies.LoadAndDelete(handle); ok {
		return p.(*dockerSocketProxy).Close()
	}
	return nil
}

// Close also covers containers that never reach Wait, such as interrupted setup.
func (r *containerRuntime) Close() error {
	var errs []error
	r.dockerProxies.Range(func(key, _ any) bool {
		errs = append(errs, r.closeDockerProxy(key.(ContainerHandle)))
		return true
	})
	if closer, ok := r.client.(interface{ Close() error }); ok {
		errs = append(errs, closer.Close())
	}
	return errors.Join(errs...)
}

func startDockerSocketProxy(ctx context.Context, mounts []ContainerMount, upstream, tmpDir string, owner DockerJobOwner) (*dockerSocketProxy, error) {
	dir, err := os.MkdirTemp(tmpDir, "docker-")
	if err != nil {
		return nil, fmt.Errorf("create Docker proxy directory: %w", err)
	}
	ready := false
	defer func() {
		if !ready {
			_ = os.RemoveAll(dir)
		}
	}()
	socketPath := filepath.Join(dir, "s")
	listener, err := listenDockerProxySocket(dir, socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on job Docker socket: %w", err)
	}
	// The file mount delegates Docker access to the job, including images with
	// a non-root user. The host socket directory remains private to the node.
	if err := os.Chmod(socketPath, 0o666); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("protect job Docker socket: %w", err)
	}

	// Nested Docker clients must retain this job's ownership boundary. The node
	// performs final cleanup even if a helper such as Ryuk can no longer connect.
	projection := append([]ContainerMount(nil), mounts...)
	for i := range projection {
		if projection[i].Source == upstream {
			projection[i].Source = socketPath
		}
	}
	docker, err := client.New(client.WithHost("unix://" + upstream))
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	var admission sync.Mutex
	var pending sync.WaitGroup
	closing := false
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "unix", upstream)
		},
	}
	forward := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "docker"
		},
		Transport:     transport,
		FlushInterval: -1,
	}
	handler := dockerJobRequestHandler(projection, owner, forward)
	var mu sync.Mutex
	connections := make(map[net.Conn]struct{})
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			admission.Lock()
			if closing {
				admission.Unlock()
				http.Error(w, "job Docker access closed", http.StatusServiceUnavailable)
				return
			}
			pending.Add(1)
			admission.Unlock()
			defer pending.Done()
			if req.Method == http.MethodPost && dockerCreateResource(req.URL.Path) != "" {
				// A disconnected caller must not hide a creation still in flight at teardown.
				requestCtx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 30*time.Second)
				defer cancel()
				req = req.WithContext(requestCtx)
			}
			handler.ServeHTTP(w, req)
		}),
		ConnState: func(conn net.Conn, state http.ConnState) {
			mu.Lock()
			defer mu.Unlock()
			if state == http.StateClosed {
				delete(connections, conn)
			} else {
				connections[conn] = struct{}{}
			}
		},
	}
	p := &dockerSocketProxy{socketPath: socketPath}
	p.close = func() error {
		admission.Lock()
		closing = true
		admission.Unlock()
		_ = server.Close()
		// http.Server.Close leaves hijacked exec/attach connections open.
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		pending.Wait()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanupErr := RemoveDockerJobResources(cleanupCtx, docker, owner)
		transport.CloseIdleConnections()
		return errors.Join(cleanupErr, docker.Close(), os.RemoveAll(dir))
	}
	stopCancel := context.AfterFunc(ctx, func() { _ = p.Close() })
	go func() {
		_ = server.Serve(listener)
		stopCancel()
		p.Close()
	}()
	ready = true
	return p, nil
}

func listenDockerProxySocket(dir, socketPath string) (net.Listener, error) {
	listenPath := socketPath
	var directory *os.File
	if runtime.GOOS == "linux" {
		// Run/job IDs exceed Linux's Unix-socket pathname limit. The directory
		// descriptor binds the same inode through a short path without chdir.
		var err error
		directory, err = os.Open(dir)
		if err != nil {
			return nil, err
		}
		defer directory.Close()
		listenPath = fmt.Sprintf("/proc/self/fd/%d/s", directory.Fd())
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: listenPath, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// The descriptor path is no longer valid after listen returns. Cleanup
	// removes the socket using its full path instead of automatic unlink.
	listener.SetUnlinkOnClose(false)
	return listener, nil
}
