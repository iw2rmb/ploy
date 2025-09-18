package catalog

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"testing"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func buildTestJar() []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("META-INF/rewrite/test.yml")
	_, _ = f.Write([]byte("type: specs.openrewrite.org/v1beta/recipe\nname: test\n"))
	_ = zw.Close()
	return buf.Bytes()
}

func TestHTTPFetcher_Fetch(t *testing.T) {
	jar := buildTestJar()
	// Fake transport that returns the jar for any request
	tr := rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(jar)),
			Header:     make(http.Header),
		}, nil
	})
	f := HTTPFetcher{BaseURL: "http://registry.example", Client: &http.Client{Transport: tr}}
	b, err := f.Fetch("rewrite-java", "1.0.0")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected bytes")
	}
}
