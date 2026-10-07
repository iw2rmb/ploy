package step

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var testDockerJobOwner = DockerJobOwner{RunID: "run-proxy", JobID: "job-proxy"}

func TestDockerJobRequestHandler_TranslatesJobSourcesAndPreservesRequest(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	staging := filepath.Join(root, "staging")
	for _, dir := range []string{filepath.Join(workspace, "docker"), staging} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{filepath.Join(workspace, "docker", "init.sql"), filepath.Join(staging, "content")} {
		if err := os.WriteFile(file, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mounts := []ContainerMount{
		{Source: workspace, Target: "/workspace"},
		{Source: root, Target: "/root"},
		{Source: filepath.Join(staging, "content"), Target: "/root/.codex/config.toml", ReadOnly: true},
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "Compose legacy bind uses the real SQL file and keeps read-only options",
			body: `{"Image":"postgres:13","HostConfig":{"Binds":["/workspace/docker/init.sql:/docker-entrypoint-initdb.d/init.sql:ro,z"]}}`,
			want: `{"Image":"postgres:13","HostConfig":{"Binds":["` + workspace + `/docker/init.sql:/docker-entrypoint-initdb.d/init.sql:ro,z"]}}`,
		},
		{
			name: "structured bind translates and disables source creation without losing options",
			body: `{"HostConfig":{"Mounts":[{"Type":"bind","Source":"/workspace/docker/init.sql","Target":"/init.sql","ReadOnly":true,"BindOptions":{"Propagation":"rprivate","CreateMountpoint":true}}]}}`,
			want: `{"HostConfig":{"Mounts":[{"Type":"bind","Source":"` + workspace + `/docker/init.sql","Target":"/init.sql","ReadOnly":true,"BindOptions":{"Propagation":"rprivate","CreateMountpoint":false}}]}}`,
		},
		{
			name: "nested file projection wins over the parent home projection",
			body: `{"HostConfig":{"Binds":["/root/.codex/config.toml:/config:ro","/root:/home"]}}`,
			want: `{"HostConfig":{"Binds":["` + staging + `/content:/config:ro","` + root + `:/home"]}}`,
		},
		{
			name: "named volumes and similarly prefixed host paths remain unchanged",
			body: `{"HostConfig":{"Binds":["db-data:/data","/workspace-other/file:/file"],"Mounts":[{"Type":"volume","Source":"db-data","Target":"/data"}]}}`,
			want: `{"HostConfig":{"Binds":["db-data:/data","/workspace-other/file:/file"],"Mounts":[{"Type":"volume","Source":"db-data","Target":"/data"}]}}`,
		},
		{
			name: "host paths and unknown API fields retain exact numeric values",
			body: `{"Extension":{"Counter":9007199254740993},"HostConfig":{"Memory":9007199254740993,"Binds":["` + workspace + `/docker/init.sql:/init.sql:ro"]}}`,
			want: `{"Extension":{"Counter":9007199254740993},"HostConfig":{"Memory":9007199254740993,"Binds":["` + workspace + `/docker/init.sql:/init.sql:ro"]}}`,
		},
		{
			name: "canonicalized paths resolve within the workspace projection",
			body: `{"HostConfig":{"Binds":["/workspace/docker/../docker/init.sql:/init.sql"]}}`,
			want: `{"HostConfig":{"Binds":["` + workspace + `/docker/init.sql:/init.sql"]}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			called := false
			handler := dockerJobRequestHandler(mounts, testDockerJobOwner, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				called = true
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				want := decodeDockerRequest(t, []byte(tt.want))
				wantLabels := make(map[string]any)
				for k, v := range testDockerJobOwner.labels() {
					wantLabels[k] = v
				}
				want["Labels"] = wantLabels
				if !reflect.DeepEqual(decodeDockerRequest(t, body), want) {
					t.Fatalf("forwarded body = %s, want %s", body, tt.want)
				}
				if req.ContentLength != int64(len(body)) || len(req.TransferEncoding) != 0 {
					t.Fatalf("invalid rewritten body framing: length=%d encoding=%v", req.ContentLength, req.TransferEncoding)
				}
				if req.URL.RawQuery != "name=process-db" || req.Header.Get("X-Registry-Auth") != "auth" {
					t.Fatalf("request metadata was lost: %v", req)
				}
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{"Id":"db"}`)
			}))
			req := httptest.NewRequest(http.MethodPost, "/v1.52/containers/create?name=process-db", strings.NewReader(tt.body))
			req.TransferEncoding = []string{"chunked"}
			req.Header.Set("X-Registry-Auth", "auth")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if !called || response.Code != http.StatusCreated || response.Body.String() != `{"Id":"db"}` {
				t.Fatalf("upstream response not preserved: called=%v response=%v", called, response)
			}
		})
	}
}

func TestDockerJobRequestHandler_RejectsMissingSourceBeforeDockerCreatesDirectory(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	called := false
	handler := dockerJobRequestHandler([]ContainerMount{{Source: workspace, Target: "/workspace"}}, testDockerJobOwner, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	for _, body := range []string{
		`{"HostConfig":{"Binds":["/workspace/init.sql:/init.sql:ro"]}}`,
		`{"HostConfig":{"Mounts":[{"Type":"bind","Source":"/workspace/init.sql","Target":"/init.sql","BindOptions":{"CreateMountpoint":true}}]}}`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/containers/create", strings.NewReader(body)))
		if called || response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "maps to unavailable host path") {
			t.Fatalf("missing source was not rejected: called=%v response=%v", called, response)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "init.sql")); !os.IsNotExist(err) {
		t.Fatalf("missing source must stay absent: %v", err)
	}
}

func TestDockerJobRequestHandler_PassesOtherDockerRequestsAndStreams(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"/v1.52/build", "/v1.52/containers/db/attach", "/v1.52/exec/id/start", "/vfoo/containers/create"} {
		t.Run(endpoint, func(t *testing.T) {
			body := []byte("arbitrary binary\x00payload")
			handler := dockerJobRequestHandler(nil, testDockerJobOwner, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				got, _ := io.ReadAll(req.Body)
				if !bytes.Equal(got, body) {
					t.Fatalf("body changed: %q", got)
				}
				_, _ = w.Write(got)
				w.(http.Flusher).Flush()
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body)))
			if !response.Flushed || !bytes.Equal(response.Body.Bytes(), body) {
				t.Fatal("streaming response was not preserved")
			}
		})
	}
}

func TestDockerJobRequestHandler_ConcurrentJobsUseSeparateMappings(t *testing.T) {
	t.Parallel()
	for _, job := range []string{"a", "b"} {
		t.Run(job, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			if err := os.WriteFile(filepath.Join(workspace, "init.sql"), []byte(job), 0o600); err != nil {
				t.Fatal(err)
			}
			handler := dockerJobRequestHandler([]ContainerMount{{Source: workspace, Target: "/workspace"}}, testDockerJobOwner, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				config := decodeDockerRequest(t, mustReadDockerBody(t, req.Body))
				bind := config["HostConfig"].(map[string]any)["Binds"].([]any)[0].(string)
				source, _, _ := strings.Cut(bind, ":")
				content, err := os.ReadFile(source)
				if err != nil || string(content) != job {
					t.Fatalf("job %s received another workspace: source=%s content=%q err=%v", job, source, content, err)
				}
			}))
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/containers/create", strings.NewReader(`{"HostConfig":{"Binds":["/workspace/init.sql:/init.sql"]}}`)))
		})
	}
}

func TestDockerJobRequestHandler_RejectsMalformedCreateRequests(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"null", "{}{}", `{`, `{"Labels":{},"labels":{"com.ploy.job_id":"forged"}}`, `{"Labels":[]}`, `{"HostConfig":[]}`, `{"HostConfig":{"Binds":[1]}}`, `{"HostConfig":{"Mounts":[null]}}`} {
		t.Run(body, func(t *testing.T) {
			handler := dockerJobRequestHandler(nil, testDockerJobOwner, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("invalid request reached Docker")
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/containers/create", strings.NewReader(body)))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}
		})
	}
}

func decodeDockerRequest(t *testing.T, body []byte) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var config map[string]any
	if err := decoder.Decode(&config); err != nil {
		t.Fatal(err)
	}
	return config
}

func mustReadDockerBody(t *testing.T, body io.Reader) []byte {
	t.Helper()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
