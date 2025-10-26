package sshtransport_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/pkg/sshtransport"
)

func TestManagerDialUsesJobAssignment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()

	remoteA := startEchoServer(t, "node-a")
	defer remoteA.Close()
	remoteB := startEchoServer(t, "node-b")
	defer remoteB.Close()

	cache := newMemoryCache()
	factory := newStubFactory(map[string]string{
		"node-a": remoteA.Addr().String(),
		"node-b": remoteB.Addr().String(),
	})

	manager, err := sshtransport.NewManager(sshtransport.Config{
		ControlSocketDir: filepath.Join(dir, "sockets"),
		Cache:            cache,
		Factory:          factory,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
	})

	nodes := []sshtransport.Node{
		{ID: "node-a", Address: "127.0.0.1", SSHPort: 22, APIPort: remoteA.Port()},
		{ID: "node-b", Address: "127.0.0.1", SSHPort: 22, APIPort: remoteB.Port()},
	}
	if err := manager.SetNodes(nodes); err != nil {
		t.Fatalf("set nodes: %v", err)
	}
	if err := cache.RememberJob("job-1", "node-b", time.Now()); err != nil {
		t.Fatalf("remember job: %v", err)
	}

	conn, err := manager.DialContext(sshtransport.WithJob(ctx, "job-1"), "tcp", "control-plane:8443")
	if err != nil {
		t.Fatalf("dial via manager: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read from tunnel: %v", err)
	}
	if got := reply; got != "node-b\n" {
		t.Fatalf("expected response from node-b, got %q", got)
	}

	if count := factory.Activations("node-b"); count == 0 {
		t.Fatalf("expected tunnel activation for node-b")
	}
}

func TestManagerFallbackOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	remoteB := startEchoServer(t, "node-b")
	defer remoteB.Close()

	cache := newMemoryCache()
	factory := newStubFactory(map[string]string{
		"node-a": "invalid:1234",
		"node-b": remoteB.Addr().String(),
	})
	factory.Fail("node-a", fmt.Errorf("boom"))

	manager, err := sshtransport.NewManager(sshtransport.Config{
		ControlSocketDir: filepath.Join(dir, "sockets"),
		Cache:            cache,
		Factory:          factory,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
	})

	nodes := []sshtransport.Node{
		{ID: "node-a", Address: "127.0.0.1", SSHPort: 22, APIPort: 8443},
		{ID: "node-b", Address: "127.0.0.1", SSHPort: 22, APIPort: remoteB.Port()},
	}
	if err := manager.SetNodes(nodes); err != nil {
		t.Fatalf("set nodes: %v", err)
	}

	conn, err := manager.DialContext(ctx, "tcp", "control-plane:8443")
	if err != nil {
		t.Fatalf("dial via manager: %v", err)
	}
	defer conn.Close()

	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}
	if reply != "node-b\n" {
		t.Fatalf("expected fallback to node-b, got %q", reply)
	}

	if got := factory.Activations("node-a"); got == 0 {
		t.Fatalf("expected attempted activation for node-a")
	}
	if got := factory.Activations("node-b"); got == 0 {
		t.Fatalf("expected activation for node-b")
	}
}

func TestManagerReconnectsAfterDrop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()

	remote := startEchoServer(t, "node-a")
	defer remote.Close()

	cache := newMemoryCache()
	factory := newStubFactory(map[string]string{
		"node-a": remote.Addr().String(),
	})

	manager, err := sshtransport.NewManager(sshtransport.Config{
		ControlSocketDir: filepath.Join(dir, "sockets"),
		Cache:            cache,
		Factory:          factory,
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() {
		_ = manager.Close()
	})
	if err := manager.SetNodes([]sshtransport.Node{
		{ID: "node-a", Address: "127.0.0.1", SSHPort: 22, APIPort: remote.Port()},
	}); err != nil {
		t.Fatalf("set nodes: %v", err)
	}

	conn1, err := manager.DialContext(ctx, "tcp", "control-plane:8443")
	if err != nil {
		t.Fatalf("dial first time: %v", err)
	}
	if _, err := bufio.NewReader(conn1).ReadString('\n'); err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	conn1.Close()

	// Simulate tunnel drop.
	factory.Drop("node-a")
	time.Sleep(600 * time.Millisecond)

	conn2, err := manager.DialContext(ctx, "tcp", "control-plane:8443")
	if err != nil {
		t.Fatalf("dial after drop: %v", err)
	}
	defer conn2.Close()

	if _, err := bufio.NewReader(conn2).ReadString('\n'); err != nil {
		t.Fatalf("read after reconnect: %v", err)
	}

	if got := factory.Activations("node-a"); got < 2 {
		t.Fatalf("expected at least two activations, got %d", got)
	}
}

// startEchoServer returns listener that writes banner on new connections.
func startEchoServer(t *testing.T, banner string) *testListener {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start listener: %v", err)
	}
	t.Helper()
	tl := &testListener{Listener: ln, banner: banner}
	go tl.serve()
	return tl
}

type testListener struct {
	net.Listener
	banner string
}

func (l *testListener) serve() {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			_, _ = io.WriteString(c, l.banner+"\n")
		}(conn)
	}
}

func (l *testListener) Port() int {
	_, portStr, _ := net.SplitHostPort(l.Addr().String())
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

// memoryCache is an in-memory implementation of sshtransport.AssignmentCache.
type memoryCache struct {
	mu   sync.Mutex
	jobs map[string]string
}

func newMemoryCache() *memoryCache {
	return &memoryCache{jobs: make(map[string]string)}
}

func (c *memoryCache) RememberNodes(nodes []sshtransport.Node) error {
	return nil
}

func (c *memoryCache) RememberJob(jobID, nodeID string, at time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jobs[jobID] = nodeID
	return nil
}

func (c *memoryCache) LookupJob(jobID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.jobs[jobID]
	return value, ok
}

func (c *memoryCache) RemoveJob(jobID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.jobs, jobID)
	return nil
}

func (c *memoryCache) Snapshot() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]string, len(c.jobs))
	for k, v := range c.jobs {
		out[k] = v
	}
	return out
}

// stubFactory simulates SSH tunnels by bridging local listeners to remote addresses.
type stubFactory struct {
	mu          sync.Mutex
	remotes     map[string]string
	handles     map[string]*stubHandle
	failures    map[string]error
	activations map[string]int
}

func newStubFactory(remotes map[string]string) *stubFactory {
	return &stubFactory{
		remotes:     remotes,
		handles:     make(map[string]*stubHandle),
		failures:    make(map[string]error),
		activations: make(map[string]int),
	}
}

func (f *stubFactory) Activate(ctx context.Context, node sshtransport.Node, localAddr string) (sshtransport.TunnelHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.failures[node.ID]; err != nil {
		f.activations[node.ID]++
		return nil, err
	}
	f.activations[node.ID]++

	remote := f.remotes[node.ID]
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return nil, err
	}

	handle := newStubHandle(ln, remote)
	f.handles[node.ID] = handle
	return handle, nil
}

func (f *stubFactory) Fail(nodeID string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[nodeID] = err
}

func (f *stubFactory) Drop(nodeID string) {
	f.mu.Lock()
	handle := f.handles[nodeID]
	f.mu.Unlock()
	if handle != nil {
		_ = handle.Close()
	}
}

func (f *stubFactory) Activations(nodeID string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.activations[nodeID]
}

type stubHandle struct {
	listener net.Listener
	done     chan error
	once     sync.Once
}

func newStubHandle(ln net.Listener, remote string) *stubHandle {
	h := &stubHandle{
		listener: ln,
		done:     make(chan error, 1),
	}
	go h.serve(remote)
	return h
}

func (h *stubHandle) serve(remote string) {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			h.finish(err)
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			target, err := net.Dial("tcp", remote)
			if err != nil {
				return
			}
			defer target.Close()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, _ = io.Copy(target, c)
			}()
			go func() {
				defer wg.Done()
				_, _ = io.Copy(c, target)
			}()
			wg.Wait()
		}(conn)
	}
}

func (h *stubHandle) finish(err error) {
	h.once.Do(func() {
		h.done <- err
		close(h.done)
	})
}

func (h *stubHandle) LocalAddress() string {
	return h.listener.Addr().String()
}

func (h *stubHandle) ControlPath() string {
	return ""
}

func (h *stubHandle) Wait() <-chan error {
	return h.done
}

func (h *stubHandle) Close() error {
	err := h.listener.Close()
	h.finish(nil)
	return err
}
