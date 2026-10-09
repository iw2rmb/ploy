package step

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/moby/moby/client"
)

func TestDockerJobRequestStampsTrustedOwnerAndPreservesApplicationLabels(t *testing.T) {
	// Every creation API preserves application labels and replaces caller ownership.
	for _, endpoint := range []string{"/containers/create", "/v1.52/containers/create", "/networks/create", "/volumes/create", "/v1.52/volumes/create"} {
		t.Run(endpoint, func(t *testing.T) {
			owner := DockerJobOwner{RunID: "run", JobID: "job", ResumeCount: 2}
			handler := dockerJobRequestHandler(nil, owner, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Labels map[string]string }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				want := owner.labels()
				want["com.docker.compose.service"] = "db"
				if !reflect.DeepEqual(body.Labels, want) {
					t.Errorf("labels=%v want=%v", body.Labels, want)
				}
				w.WriteHeader(http.StatusCreated)
			}))
			r, _ := http.NewRequest(http.MethodPost, "http://docker"+endpoint, strings.NewReader(`{"labels":{"com.ploy.job_id":"forged","com.ploy.run_id":"other","com.ploy.resume_count":"99","com.ploy.job_resource":"false","com.docker.compose.service":"db"}}`))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				t.Fatalf("status=%d", w.Code)
			}
		})
	}
}

type resourceDaemon struct {
	mu            sync.Mutex
	resources     map[string]dockerJobResource
	removed       []string
	createEntered chan struct{}
	releaseCreate chan struct{}
	failRemove    bool
}

func (d *resourceDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/v1.52")
	w.Header().Set("Content-Type", "application/json")
	if p == "/_ping" {
		w.Header().Set("API-Version", "1.52")
		return
	}
	if r.Method == http.MethodPost && dockerCreateResource(p) != "" {
		var body struct{ Labels map[string]string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if d.createEntered != nil {
			close(d.createEntered)
			<-d.releaseCreate
		}
		d.mu.Lock()
		d.resources["created"] = dockerJobResource{dockerCreateResource(p), "created", body.Labels}
		d.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"Id":"created"}`)
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if r.Method == http.MethodDelete {
		id := filepath.Base(p)
		if d.failRemove {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"message":"daemon unavailable"}`)
			return
		}
		if strings.HasPrefix(p, "/volumes/") && r.URL.Query().Get("force") == "1" {
			http.Error(w, "volume must not be forced", 500)
			return
		}
		if strings.HasPrefix(p, "/containers/") && (r.URL.Query().Get("force") != "1" || r.URL.Query().Get("v") != "1") {
			http.Error(w, "container removal must include anonymous volumes", 500)
			return
		}
		delete(d.resources, id)
		d.removed = append(d.removed, id)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var result []map[string]any
	switch p {
	case "/containers/json", "/networks", "/volumes":
		kind := map[string]string{"/containers/json": "container", "/networks": "network", "/volumes": "volume"}[p]
		for _, resource := range d.resources {
			if resource.kind == kind {
				result = append(result, map[string]any{"Id": resource.id, "Name": resource.id, "Labels": resource.labels})
			}
		}
		if p == "/volumes" {
			_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": result})
		} else {
			_ = json.NewEncoder(w).Encode(result)
		}
	default:
		if strings.HasPrefix(p, "/volumes/") {
			v, ok := d.resources[filepath.Base(p)]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Name": v.id, "Labels": v.labels})
		} else {
			http.NotFound(w, r)
		}
	}
}

func startResourceDaemon(t *testing.T, daemon *resourceDaemon) (string, string, *client.Client) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "ploy-owner-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	socket := filepath.Join(root, "host.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: daemon}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	docker, err := client.New(client.WithHost("unix://"+socket), client.WithAPIVersion("1.52"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = docker.Close() })
	return root, socket, docker
}

func TestDockerJobCleanupRemovesOnlyOwnedExecutionInDependencyOrder(t *testing.T) {
	// Cleanup keeps the parent, other jobs, retries, and external resources intact.
	owner := testDockerJobOwner
	other := owner
	other.JobID = "other"
	retry := owner
	retry.ResumeCount = 1
	parent := owner.labels()
	delete(parent, types.LabelJobResource)
	daemon := &resourceDaemon{resources: map[string]dockerJobResource{
		"child":    {"container", "child", owner.labels()},
		"net":      {"network", "net", owner.labels()},
		"vol":      {"volume", "vol", owner.labels()},
		"parent":   {"container", "parent", parent},
		"other":    {"container", "other", other.labels()},
		"retry":    {"container", "retry", retry.labels()},
		"external": {"volume", "external", nil},
	}}
	_, _, docker := startResourceDaemon(t, daemon)
	if err := RemoveDockerJobResources(context.Background(), docker, owner); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(daemon.removed, []string{"child", "net", "vol"}) {
		t.Fatalf("removed=%v", daemon.removed)
	}
	if len(daemon.resources) != 4 {
		t.Fatalf("remaining=%v", daemon.resources)
	}
	if err := RemoveDockerJobResources(context.Background(), docker, owner); err != nil {
		t.Fatalf("repeated cleanup: %v", err)
	}
}

func TestDockerProxyCancellationWaitsForCreationThenRemovesChild(t *testing.T) {
	// A create response arriving after cancellation must not leave a running child.
	daemon := &resourceDaemon{resources: map[string]dockerJobResource{}, createEntered: make(chan struct{}), releaseCreate: make(chan struct{})}
	root, socket, _ := startResourceDaemon(t, daemon)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxy, err := startDockerSocketProxy(ctx, nil, socket, root, testDockerJobOwner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", proxy.socketPath)
	}}
	defer tr.CloseIdleConnections()
	httpClient := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		r, e := httpClient.Post("http://docker/containers/create", "application/json", strings.NewReader(`{"Image":"postgres"}`))
		if e == nil {
			_ = r.Body.Close()
		}
	}()
	select {
	case <-daemon.createEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("create did not reach daemon")
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- proxy.Close() }()
	select {
	case err := <-done:
		t.Fatalf("cleanup passed in-flight creation: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(daemon.releaseCreate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not complete")
	}
	<-requestDone
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if len(daemon.resources) != 0 || !reflect.DeepEqual(daemon.removed, []string{"created"}) {
		t.Fatalf("leaked resources: %v removed=%v", daemon.resources, daemon.removed)
	}
}

func TestDockerProxyReturnsCleanupFailureForRetry(t *testing.T) {
	// Failed removal is visible to the caller and leaves ownership for recovery.
	daemon := &resourceDaemon{resources: map[string]dockerJobResource{"child": {"container", "child", testDockerJobOwner.labels()}}, failRemove: true}
	root, socket, _ := startResourceDaemon(t, daemon)
	proxy, err := startDockerSocketProxy(context.Background(), nil, socket, root, testDockerJobOwner)
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.Close(); err == nil {
		t.Fatal("cleanup failure was swallowed")
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	if _, ok := daemon.resources["child"]; !ok {
		t.Fatal("failed cleanup lost recovery identity")
	}
}

func TestDockerSocketRejectsMissingOwnerBeforeStartingProxy(t *testing.T) {
	// Unattributed Docker access cannot create resources that cleanup cannot own.
	rt := newContainerRuntimeWithClient(&fakeDockerClient{}, ContainerRuntimeOptions{})
	rt.startDockerProxy = func(context.Context, []ContainerMount, string, string, DockerJobOwner) (*dockerSocketProxy, error) {
		t.Fatal("unowned proxy started")
		return nil, errors.New("unexpected")
	}
	_, err := rt.Create(context.Background(), ContainerSpec{Image: "job", Mounts: []ContainerMount{{Source: t.TempDir(), Target: "/tmp"}, {Source: "/host/docker.sock", Target: "/var/run/docker.sock"}}})
	if err == nil {
		t.Fatal("unowned Docker socket accepted")
	}
}
