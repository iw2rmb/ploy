package common

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	utils "github.com/iw2rmb/ploy/internal/cli/utils"
)

func TestSharedPush(t *testing.T) {
	tests := []struct {
		name       string
		config     DeployConfig
		wantErr    bool
		wantDomain string
	}{
		{
			name: "user app deployment",
			config: DeployConfig{
				App:           "test-app",
				IsPlatform:    false,
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "test-app.ployd.app",
		},
		{
			name: "platform service deployment",
			config: DeployConfig{
				App:           "ploy-api",
				IsPlatform:    true,
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "ploy-api.ployman.app",
		},
		{
			name: "user app dev environment",
			config: DeployConfig{
				App:           "test-app",
				IsPlatform:    false,
				Environment:   "dev",
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "test-app.dev.ployd.app",
		},
		{
			name: "platform service dev environment",
			config: DeployConfig{
				App:           "ploy-api",
				IsPlatform:    true,
				Environment:   "dev",
				ControllerURL: "http://localhost:8081",
			},
			wantErr:    false,
			wantDomain: "ploy-api.dev.ployman.app",
		},
		{
			name: "missing app name",
			config: DeployConfig{
				App:           "",
				ControllerURL: "http://localhost:8081",
			},
			wantErr: true,
		},
		{
			name: "missing controller URL",
			config: DeployConfig{
				App:           "test-app",
				ControllerURL: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify headers
				if tt.config.IsPlatform {
					if r.Header.Get("X-Platform-Service") != "true" {
						t.Errorf("Expected X-Platform-Service header for platform deployment")
					}
					if r.Header.Get("X-Target-Domain") != "ployman.app" {
						t.Errorf("Expected X-Target-Domain to be ployman.app")
					}
				} else {
					if r.Header.Get("X-Target-Domain") != "ployd.app" {
						t.Errorf("Expected X-Target-Domain to be ployd.app")
					}
				}

				// Send success response
				w.Header().Set("X-Deployment-ID", "test-deployment-123")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"status": "success"}`))
			}))
			defer server.Close()

			// Update config with test server URL
			if tt.config.ControllerURL != "" {
				tt.config.ControllerURL = server.URL
			}

			result, err := SharedPush(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("SharedPush() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && result != nil {
				if result.URL != "https://"+tt.wantDomain {
					t.Errorf("SharedPush() URL = %v, want %v", result.URL, "https://"+tt.wantDomain)
				}
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  DeployConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: DeployConfig{
				App:           "test-app",
				ControllerURL: "http://localhost:8081",
			},
			wantErr: false,
		},
		{
			name: "missing app name",
			config: DeployConfig{
				App:           "",
				ControllerURL: "http://localhost:8081",
			},
			wantErr: true,
		},
		{
			name: "missing controller URL",
			config: DeployConfig{
				App:           "test-app",
				ControllerURL: "",
			},
			wantErr: true,
		},
		{
			name: "both missing",
			config: DeployConfig{
				App:           "",
				ControllerURL: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateConfig(tt.config); (err != nil) != tt.wantErr {
				t.Errorf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBuildDeployURL(t *testing.T) {
	tests := []struct {
		name   string
		config DeployConfig
		want   string
	}{
		{
			name: "basic URL",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123",
		},
		{
			name: "with lane",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				Lane:          "C",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&lane=C",
		},
		{
			name: "with main class",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				MainClass:     "com.example.Main",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&main=com.example.Main",
		},
		{
			name: "platform service",
			config: DeployConfig{
				App:           "ploy-api",
				SHA:           "abc123",
				IsPlatform:    true,
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/ploy-api/builds?sha=abc123&platform=true",
		},
		{
			name: "blue-green deployment",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				BlueGreen:     true,
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&blue_green=true",
		},
		{
			name: "with environment",
			config: DeployConfig{
				App:           "test-app",
				SHA:           "abc123",
				Environment:   "staging",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/test-app/builds?sha=abc123&env=staging",
		},
		{
			name: "all parameters",
			config: DeployConfig{
				App:           "ploy-api",
				SHA:           "abc123",
				Lane:          "E",
				MainClass:     "com.example.Main",
				IsPlatform:    true,
				BlueGreen:     true,
				Environment:   "prod",
				ControllerURL: "http://localhost:8081",
			},
			want: "http://localhost:8081/apps/ploy-api/builds?sha=abc123&main=com.example.Main&lane=E&platform=true&blue_green=true&env=prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildDeployURL(tt.config); got != tt.want {
				t.Errorf("buildDeployURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTargetDomain(t *testing.T) {
	tests := []struct {
		name   string
		config DeployConfig
		want   string
	}{
		{
			name: "user app prod",
			config: DeployConfig{
				IsPlatform:  false,
				Environment: "prod",
			},
			want: "ployd.app",
		},
		{
			name: "user app dev",
			config: DeployConfig{
				IsPlatform:  false,
				Environment: "dev",
			},
			want: "dev.ployd.app",
		},
		{
			name: "platform service prod",
			config: DeployConfig{
				IsPlatform:  true,
				Environment: "prod",
			},
			want: "ployman.app",
		},
		{
			name: "platform service dev",
			config: DeployConfig{
				IsPlatform:  true,
				Environment: "dev",
			},
			want: "dev.ployman.app",
		},
		{
			name: "user app no env (defaults to prod)",
			config: DeployConfig{
				IsPlatform:  false,
				Environment: "",
			},
			want: "ployd.app",
		},
		{
			name: "platform service no env (defaults to prod)",
			config: DeployConfig{
				IsPlatform:  true,
				Environment: "",
			},
			want: "ployman.app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTargetDomain(tt.config); got != tt.want {
				t.Errorf("getTargetDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSharedPushSupportsInjectedDependencies(t *testing.T) {
	t.Setenv("PLOY_AUTOGEN_DOCKERFILE", "1")
	deps := &SharedPushDeps{
		HTTPClient: &stubHTTPClient{},
		TarBuilder: (&stubTarBuilder{}).Build,
		ReadGitignore: func(string) (utils.Ignore, error) {
			return utils.Ignore{}, nil
		},
		AutogenDockerfile: func(string) error { return nil },
		Stdout:            io.Discard,
	}

	config := DeployConfig{
		App:           "demo",
		ControllerURL: "https://controller.example",
		SHA:           "sha-demo",
		Deps:          deps,
		WorkingDir:    t.TempDir(),
	}

	result, err := SharedPush(config)
	if err != nil {
		t.Fatalf("SharedPush returned error: %v", err)
	}

	client := deps.HTTPClient.(*stubHTTPClient)
	if client.request == nil {
		t.Fatalf("expected HTTP client to capture request")
	}
	if client.request.URL.String() != "https://controller.example/apps/demo/builds?sha=sha-demo" {
		t.Fatalf("unexpected request URL: %s", client.request.URL.String())
	}
	if got := client.request.Header.Get("X-Target-Domain"); got != "ployd.app" {
		t.Fatalf("missing target domain header, got %q", got)
	}
	if !bytes.Equal(client.body, []byte("stub-tar")) {
		t.Fatalf("unexpected tar payload: %q", string(client.body))
	}

	if result == nil || !result.Success {
		t.Fatalf("expected success result, got %#v", result)
	}
	if result.DeploymentID != "dep-stub" {
		t.Fatalf("deployment id mismatch: %q", result.DeploymentID)
	}
	if result.URL != "https://demo.ployd.app" {
		t.Fatalf("unexpected result URL: %s", result.URL)
	}
}

func TestSharedPushMultipartUploads(t *testing.T) {
	t.Setenv("PLOY_AUTOGEN_DOCKERFILE", "0")
	reqCh := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- r
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("parse media type: %v", err)
		}
		if mediaType != "multipart/form-data" {
			t.Fatalf("content-type = %s, want multipart/form-data", mediaType)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("read part: %v", err)
			}
			_, _ = io.Copy(io.Discard, part)
		}
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	config := DeployConfig{
		App:           "demo",
		ControllerURL: server.URL,
		SHA:           "sha-demo",
		UseMultipart:  true,
		TarExtras: map[string][]byte{
			"extra.txt": []byte("hello"),
		},
		Metadata: map[string]string{
			"autogen_dockerfile": "true",
		},
		Deps: &SharedPushDeps{Stdout: io.Discard},
	}

	result, err := SharedPush(config)
	if err != nil {
		t.Fatalf("SharedPush returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success result, got %#v", result)
	}

	select {
	case req := <-reqCh:
		if req.URL.Query().Get("autogen_dockerfile") != "true" {
			t.Fatalf("metadata missing: %s", req.URL.RawQuery)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected multipart request")
	}
}

func TestSharedPushErrorResponseParsing(t *testing.T) {
	reqCh := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCh <- struct{}{}
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":"build_failed","message":"builder failed","details":"missing base image"},"builder":{"job":"builder-123","logs":"compile error","logs_key":"logs-key","logs_url":"https://logs.example"}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	config := DeployConfig{
		App:           "broken",
		ControllerURL: server.URL,
		SHA:           "sha-fail",
		Deps:          &SharedPushDeps{Stdout: &stdout},
	}

	result, err := SharedPush(config)
	if err != nil {
		t.Fatalf("SharedPush returned error: %v", err)
	}
	if result.Success {
		t.Fatalf("expected failure result")
	}
	if result.ErrorCode != "build_failed" {
		t.Fatalf("unexpected error code: %#v", result)
	}
	if result.Message != "builder failed" {
		t.Fatalf("unexpected message: %#v", result)
	}
	if result.BuilderJob != "builder-123" || result.BuilderLogsKey != "logs-key" || result.BuilderLogsURL != "https://logs.example" {
		t.Fatalf("builder metadata not parsed: %#v", result)
	}
	if result.BuilderLogs != "compile error" {
		t.Fatalf("expected builder logs, got %q", result.BuilderLogs)
	}

	if stdout.Len() == 0 {
		t.Fatalf("expected response body to be written to stdout buffer")
	}

	select {
	case <-reqCh:
	case <-time.After(time.Second):
		t.Fatalf("expected request to server")
	}
}

type stubHTTPClient struct {
	request *http.Request
	body    []byte
}

func (c *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.request = req
	data, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	_ = req.Body.Close()
	c.body = data
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewBufferString(`{"status":"ok"}`)),
	}
	resp.Header.Set("X-Deployment-ID", "dep-stub")
	return resp, nil
}

type stubTarBuilder struct{}

func (stubTarBuilder) Build(dir string, w io.Writer, _ utils.Ignore, _ utils.TarOptions) error {
	_, err := w.Write([]byte("stub-tar"))
	return err
}
