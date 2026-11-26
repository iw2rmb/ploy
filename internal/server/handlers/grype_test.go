package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type grypeTransportFunc func(*http.Request) (*http.Response, error)

func (f grypeTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestGrypeProxyStreamsPartialArchive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/grype/databases/{path...}", grypeDatabaseHandler(grypeTransportFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.URL.String(); got != "https://grype.anchore.io/databases/v6/archive.tar.zst?checksum=sha256%3A123" {
			t.Errorf("upstream URL = %s", got)
		}
		if r.Host != "grype.anchore.io" || r.Header.Get("Range") != "bytes=1-3" || r.Header.Get("If-Range") != "\"release\"" {
			t.Error("upstream host or range headers differ")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("upstream received Ploy credentials")
		}
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Range": {"bytes 1-3/5"}, "Etag": {"\"release\""}},
			Body:       io.NopCloser(strings.NewReader("bcd")),
		}, nil
	})))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/grype/databases/v6/archive.tar.zst?auth_token=secret&checksum=sha256%3A123", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=1-3")
	req.Header.Set("If-Range", "\"release\"")
	req.Header.Set("Authorization", "Bearer secret")
	req.AddCookie(&http.Cookie{Name: "session", Value: "secret"})
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != "bytes 1-3/5" || resp.Header.Get("ETag") != "\"release\"" || string(body) != "bcd" {
		t.Errorf("proxy returned %d, %v, %q", resp.StatusCode, resp.Header, body)
	}
}
