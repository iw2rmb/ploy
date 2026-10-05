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
)

type dockerSocketProxy struct {
	socketPath string
	close      func()
	once       sync.Once
}

func (p *dockerSocketProxy) Close() {
	p.once.Do(p.close)
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
	proxy, err := start(ctx, mounts, mounts[socketIndex].Source, tmpDir)
	if err != nil {
		return nil, nil, err
	}
	mounts[socketIndex].Source = proxy.socketPath
	return mounts, proxy, nil
}

func (r *containerRuntime) closeDockerProxy(handle ContainerHandle) {
	if r == nil {
		return
	}
	if p, ok := r.dockerProxies.LoadAndDelete(handle); ok {
		p.(*dockerSocketProxy).Close()
	}
}

// Close also covers containers that never reach Wait, such as interrupted setup.
func (r *containerRuntime) Close() error {
	r.dockerProxies.Range(func(key, _ any) bool {
		r.closeDockerProxy(key.(ContainerHandle))
		return true
	})
	if closer, ok := r.client.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func startDockerSocketProxy(ctx context.Context, mounts []ContainerMount, upstream, tmpDir string) (*dockerSocketProxy, error) {
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

	// Helper containers such as Ryuk must retain the host socket so their
	// cleanup can continue after the parent job and its proxy have exited.
	projection := append([]ContainerMount(nil), mounts...)
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
	var mu sync.Mutex
	connections := make(map[net.Conn]struct{})
	server := &http.Server{
		Handler: dockerBindMountHandler(projection, forward),
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
	p.close = func() {
		_ = server.Close()
		// http.Server.Close leaves hijacked exec/attach connections open.
		mu.Lock()
		for conn := range connections {
			_ = conn.Close()
		}
		mu.Unlock()
		transport.CloseIdleConnections()
		_ = os.RemoveAll(dir)
	}
	stopCancel := context.AfterFunc(ctx, p.Close)
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
