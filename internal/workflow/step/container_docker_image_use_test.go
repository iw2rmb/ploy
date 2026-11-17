package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

type imageUseDocker struct {
	*fakeDockerClient
	ids             map[string]string
	inspectImageErr error
}

func (f *imageUseDocker) ImageInspect(_ context.Context, ref string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if f.inspectImageErr != nil {
		return client.ImageInspectResult{}, f.inspectImageErr
	}
	return client.ImageInspectResult{InspectResponse: image.InspectResponse{ID: f.ids[ref]}}, nil
}

type imageUseRecorder struct {
	mu     sync.Mutex
	uses   map[string]int
	resets int
	events []string
}

func (r *imageUseRecorder) record(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.uses == nil {
		r.uses = make(map[string]int)
	}
	r.uses[id]++
	r.events = append(r.events, id)
}
func (r *imageUseRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resets++
	r.events = append(r.events, "reset")
}
func (r *imageUseRecorder) counts(id string) (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.uses[id], r.resets
}

// Runtime close refreshes immutable identities even after containers have disappeared.
func TestContainerRuntimeImageUseRefreshesAtClose(t *testing.T) {
	for _, pull := range []bool{false, true} {
		t.Run(fmt.Sprint(pull), func(t *testing.T) {
			fake := &imageUseDocker{fakeDockerClient: &fakeDockerClient{createResult: client.ContainerCreateResult{ID: "job"}, inspectResult: client.ContainerInspectResult{Container: container.InspectResponse{Image: "platform-id"}}}, ids: map[string]string{"job:latest": "sha256:before", "platform-id": "sha256:actual"}}
			recorder := &imageUseRecorder{}
			runtime := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{PullImage: pull, RecordImageUse: recorder.record, InvalidateImageUse: recorder.reset})
			if _, err := runtime.Create(context.Background(), ContainerSpec{Image: "job:latest"}); err != nil {
				t.Fatal(err)
			}
			if fake.createOpts.Config.Image != "job:latest" {
				t.Fatal("image name changed")
			}
			if fake.pullCalled != pull {
				t.Fatalf("pull=%v", fake.pullCalled)
			}
			if got, resets := recorder.counts("sha256:actual"); got != 1 || resets != 0 {
				t.Fatalf("actual records=%d resets=%d", got, resets)
			}
			fake.inspectErr = errors.New("container already removed")
			if err := runtime.Close(); err != nil {
				t.Fatal(err)
			}
			if err := runtime.Close(); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"sha256:before", "sha256:actual"} {
				if got, resets := recorder.counts(id); got != 2 || resets != 0 {
					t.Fatalf("%s records=%d resets=%d", id, got, resets)
				}
			}
		})
	}
}

// A successful pull followed by missing identity invalidates history even if creation also fails.
func TestContainerRuntimeSuccessfulPullMissingIdentityInvalidatesUsage(t *testing.T) {
	fake := &imageUseDocker{
		fakeDockerClient: &fakeDockerClient{createErr: errors.New("image disappeared before create")},
		inspectImageErr:  errdefs.ErrNotFound.WithMessage("image disappeared after pull"),
	}
	recorder := &imageUseRecorder{}
	runtime := newContainerRuntimeWithClient(fake, ContainerRuntimeOptions{PullImage: true, RecordImageUse: recorder.record, InvalidateImageUse: recorder.reset})
	if _, err := runtime.Create(context.Background(), ContainerSpec{Image: "job:latest"}); err == nil {
		t.Fatal("create failure was lost")
	}
	if !fake.pullCalled || !fake.createCalled {
		t.Fatal("pull/create path did not execute")
	}
	if _, resets := recorder.counts(""); resets != 1 {
		t.Fatalf("failed identity resets=%d", resets)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, resets := recorder.counts(""); resets != 2 {
		t.Fatalf("completion resets=%d", resets)
	}
}

// An uncertain session resets all observations again before refreshing its known identities at close.
func TestDockerImageUseUncertaintySurvivesUntilClose(t *testing.T) {
	recorder := &imageUseRecorder{}
	session := newDockerImageUseSession(recorder.record, recorder.reset)
	session.observeImage(context.Background(), &imageUseDocker{fakeDockerClient: &fakeDockerClient{}, inspectImageErr: errors.New("Docker unavailable")}, "unknown:latest", false)
	session.note("sha256:known")
	session.close()
	session.close()
	got, resets := recorder.counts("sha256:known")
	if got != 2 || resets != 2 {
		t.Fatalf("records=%d resets=%d", got, resets)
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.events) != 4 || recorder.events[2] != "reset" || recorder.events[3] != "sha256:known" {
		t.Fatalf("close events=%v", recorder.events)
	}
}

func imageUseTestProxy(t *testing.T, handler http.HandlerFunc, session *dockerImageUseSession) (*dockerSocketProxy, *http.Client) {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "ploy-iu-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	socket := filepath.Join(root, "host.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	daemon := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodHead {
			w.Header().Set("API-Version", "1.52")
			return
		}
		switch dockerAPIPath(r.URL.Path) {
		case "containers/json", "networks":
			_, _ = io.WriteString(w, `[]`)
			return
		case "volumes":
			_, _ = io.WriteString(w, `{"Volumes":[]}`)
			return
		}
		handler(w, r)
	})}
	go func() { _ = daemon.Serve(listener) }()
	t.Cleanup(func() { _ = daemon.Close() })
	proxy, err := startDockerSocketProxy(context.Background(), nil, socket, root, testDockerJobOwner, session)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = proxy.Close() })
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", proxy.socketPath)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	return proxy, &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func imageUsePost(t *testing.T, httpClient *http.Client, path, body string) string {
	t.Helper()
	response, err := httpClient.Post("http://docker/v1.52/"+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s status=%d body=%s", path, response.StatusCode, data)
	}
	return string(data)
}

// The proxy resolves the actual image before returning create and retains it after AutoRemove start/restart.
func TestDockerProxyImageUseSurvivesAutoRemove(t *testing.T) {
	for _, action := range []string{"start", "restart"} {
		t.Run(action, func(t *testing.T) {
			recorder := &imageUseRecorder{}
			session := newDockerImageUseSession(recorder.record, recorder.reset)
			var mu sync.Mutex
			removed := false
			receivedImage := ""
			proxy, httpClient := imageUseTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch dockerAPIPath(r.URL.Path) {
				case "images/db:latest/json":
					_, _ = io.WriteString(w, `{"Id":"sha256:before"}`)
				case "images/platform-id/json":
					_, _ = io.WriteString(w, `{"Id":"sha256:actual"}`)
				case "containers/create":
					var config struct{ Image string }
					_ = json.NewDecoder(r.Body).Decode(&config)
					receivedImage = config.Image
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `{"Id":"nested","Warnings":[]}`)
				case "containers/nested/json":
					if removed {
						http.Error(w, `{"message":"removed"}`, http.StatusNotFound)
						return
					}
					_, _ = io.WriteString(w, `{"Id":"nested","Image":"platform-id"}`)
				case "containers/nested/start", "containers/nested/restart":
					removed = true
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}, session)
			body := imageUsePost(t, httpClient, "containers/create", `{"Image":"db:latest","HostConfig":{"AutoRemove":true}}`)
			if !strings.Contains(body, `"Id":"nested"`) {
				t.Fatalf("create response changed: %s", body)
			}
			if count, resets := recorder.counts("sha256:actual"); count != 1 || resets != 0 {
				t.Fatalf("create returned before identity: records=%d resets=%d", count, resets)
			}
			mu.Lock()
			if receivedImage != "db:latest" {
				t.Errorf("forwarded image=%q", receivedImage)
			}
			mu.Unlock()
			imageUsePost(t, httpClient, "containers/nested/"+action, "")
			if err := proxy.Close(); err != nil {
				t.Fatal(err)
			}
			if count, resets := recorder.counts("sha256:actual"); count != 2 || resets != 0 {
				t.Fatalf("proxy completion records=%d resets=%d", count, resets)
			}
			session.close()
			if count, resets := recorder.counts("sha256:actual"); count != 3 || resets != 0 {
				t.Fatalf("runtime completion records=%d resets=%d", count, resets)
			}
		})
	}
}

// Cached and newly downloaded pulls both refresh the resolved image without resetting unrelated images.
func TestDockerProxyImagePullRecordsResolvedImage(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			recorder := &imageUseRecorder{}
			session := newDockerImageUseSession(recorder.record, recorder.reset)
			var mu sync.Mutex
			pulled := false
			proxy, httpClient := imageUseTestProxy(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch dockerAPIPath(r.URL.Path) {
				case "images/db:latest/json":
					if !cached && !pulled {
						w.WriteHeader(http.StatusNotFound)
						_, _ = io.WriteString(w, `{"message":"not found"}`)
						return
					}
					_, _ = io.WriteString(w, `{"Id":"sha256:downloaded"}`)
				case "images/create":
					pulled = true
					_, _ = io.WriteString(w, "{\"status\":\"done\"}\n")
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}, session)
			imageUsePost(t, httpClient, "images/create?fromImage=db&tag=latest", "")
			if err := proxy.Close(); err != nil {
				t.Fatal(err)
			}
			want := 1
			if cached {
				want = 2
			}
			if count, resets := recorder.counts("sha256:downloaded"); count != want || resets != 0 {
				t.Fatalf("pull records=%d want=%d resets=%d", count, want, resets)
			}
		})
	}
}

// Opaque image operations invalidate at admission, completion, and session close.
func TestDockerProxyOpaqueImageOperationsInvalidateUsage(t *testing.T) {
	for _, path := range []string{"/v1.52/build", "/images/load", "/images/create?fromSrc=-", "/images/create?fromImage=db", "/commit?container=existing"} {
		t.Run(path, func(t *testing.T) {
			recorder := &imageUseRecorder{}
			session := newDockerImageUseSession(recorder.record, recorder.reset)
			handler := session.requestHandler(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, resets := recorder.counts(""); resets != 1 {
					t.Fatalf("admission resets=%d", resets)
				}
				w.WriteHeader(http.StatusOK)
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, nil))
			if _, resets := recorder.counts(""); resets != 2 {
				t.Fatalf("completion resets=%d", resets)
			}
			session.close()
			if _, resets := recorder.counts(""); resets != 3 {
				t.Fatalf("close resets=%d", resets)
			}
		})
	}
}
