package httpx

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		client  func(t *testing.T) (*http.Client, string)
		method  string
		body    any
		want    string
		wantErr string
	}{
		{
			name: "success preserves request",
			client: func(t *testing.T) (*http.Client, string) {
				t.Helper()
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodPost || r.URL.Path != "/items" || r.Header.Get("Content-Type") != "application/json" {
						t.Errorf("request = %s %s content-type=%q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
					}
					request, _ := io.ReadAll(r.Body)
					if string(request) != `{"name":"input"}` {
						t.Errorf("body = %s", request)
					}
					_, _ = io.WriteString(w, `{"name":"output"}`)
				}))
				t.Cleanup(ts.Close)
				return ts.Client(), ts.URL + "/items"
			},
			method: http.MethodPost,
			body:   map[string]string{"name": "input"},
			want:   "output",
		},
		{
			name: "request construction failure",
			client: func(t *testing.T) (*http.Client, string) {
				return http.DefaultClient, "://bad"
			},
			method:  http.MethodGet,
			wantErr: "test request: build request",
		},
		{
			name: "transport failure",
			client: func(t *testing.T) (*http.Client, string) {
				return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("offline")
				})}, "http://example.test"
			},
			method:  http.MethodGet,
			wantErr: "test request: http request: Get \"http://example.test\": offline",
		},
		{
			name:    "unexpected status",
			client:  responseClient(http.StatusConflict, `{"error":"conflict"}`),
			method:  http.MethodGet,
			wantErr: "test request: unexpected status 409: conflict",
		},
		{
			name:    "unknown field",
			client:  responseClient(http.StatusOK, `{"name":"ok","extra":true}`),
			method:  http.MethodGet,
			wantErr: `test request: decode response: json: unknown field "extra"`,
		},
		{
			name:    "malformed JSON",
			client:  responseClient(http.StatusOK, `{"name":`),
			method:  http.MethodGet,
			wantErr: "test request: decode response: unexpected EOF",
		},
		{
			name:    "response size limit",
			client:  responseClient(http.StatusOK, `{"name":"`+strings.Repeat("x", int(MaxJSONBodyBytes))+`"}`),
			method:  http.MethodGet,
			wantErr: "test request: decode response: response exceeds 1048576 bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			client, endpoint := tt.client(t)
			got, err := DoJSON[struct {
				Name string `json:"name"`
			}](context.Background(), client, tt.method, endpoint, tt.body, http.StatusOK, "test request")
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DoJSON() error = %v", err)
			}
			if got.Name != tt.want {
				t.Fatalf("name = %q, want %q", got.Name, tt.want)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestHeaderTransportOverlaysHeadersWithoutMutatingRequest(t *testing.T) {
	t.Parallel()

	configured := http.Header{
		"Authorization":  []string{"Bearer test-token"},
		"PLOY_NODE_UUID": []string{"local1"},
	}
	var gotReq *http.Request
	transport := NewHeaderTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotReq = req
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	}), configured)
	configured.Set("Authorization", "Bearer changed")

	req, err := http.NewRequest(http.MethodGet, "http://example.test/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer original")
	req.Header.Set("X-Test", "1")

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}
	_ = resp.Body.Close()

	if gotReq == nil {
		t.Fatal("base transport did not receive request")
	}
	if got := gotReq.Header.Get("Authorization"); got != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := gotReq.Header.Get("PLOY_NODE_UUID"); got != "local1" {
		t.Fatalf("PLOY_NODE_UUID = %q, want %q", got, "local1")
	}
	if got := gotReq.Header.Get("X-Test"); got != "1" {
		t.Fatalf("X-Test = %q, want %q", got, "1")
	}
	if got := req.Header.Get("Authorization"); got != "Bearer original" {
		t.Fatalf("original Authorization = %q, want %q", got, "Bearer original")
	}
	if got := req.Header.Get("PLOY_NODE_UUID"); got != "" {
		t.Fatalf("original PLOY_NODE_UUID = %q, want empty", got)
	}
}

func TestHeaderTransportHandlesNilInputState(t *testing.T) {
	t.Parallel()

	transport := NewHeaderTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer test-token")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	}), http.Header{"Authorization": []string{"Bearer test-token"}})

	req, err := http.NewRequest(http.MethodGet, "http://example.test/v1/health", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	req.Header = nil
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}
	_ = resp.Body.Close()

	if _, err := transport.RoundTrip(nil); err == nil || err.Error() != "request is nil" {
		t.Fatalf("RoundTrip(nil) error = %v, want request is nil", err)
	}
}

func TestHeaderTransportDefaultsBaseTransport(t *testing.T) {
	t.Parallel()

	gotAuth := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := &http.Client{Transport: NewHeaderTransport(nil, http.Header{
		"Authorization": []string{"Bearer test-token"},
	})}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Get() failed: %v", err)
	}
	_ = resp.Body.Close()
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
}

func responseClient(status int, body string) func(*testing.T) (*http.Client, string) {
	return func(t *testing.T) (*http.Client, string) {
		t.Helper()
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})}, "http://example.test"
	}
}

func TestGunzipToBytes(t *testing.T) {
	t.Parallel()
	want := []byte("diff --git a/a b/a\n+line\n")
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write(want)
	_ = gw.Close()

	got, err := GunzipToBytes(bytes.NewReader(gzBuf.Bytes()), MaxGunzipOutputBytes)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("GunzipToBytes() = %q, %v", got, err)
	}
	got, err = GunzipToBytes(bytes.NewReader(nil), MaxGunzipOutputBytes)
	if err != nil || len(got) != 0 {
		t.Fatalf("GunzipToBytes(empty) = %q, %v", got, err)
	}
	if _, err := GunzipToBytes(bytes.NewReader(gzBuf.Bytes()), 10); err == nil {
		t.Fatal("GunzipToBytes(too large) error = nil")
	}
}
