package recipes

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestMavenFetcher_Fetch(t *testing.T) {
	// Build fake jar
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, _ := zw.Create("META-INF/rewrite/test.yml")
	_, _ = f.Write([]byte("type: specs.openrewrite.org/v1beta/recipe\nname: test\n"))
	_ = zw.Close()
	jar := buf.Bytes()

	// Fake transport that returns jar for the expected path
	tr := rtFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(jar)), Header: make(http.Header)}, nil
	})
	fch := MavenFetcher{BaseURL: "https://repo.example/maven2", GroupID: "org.openrewrite", Client: &http.Client{Transport: tr}}
	b, err := fch.Fetch("rewrite-java", "8.1.0")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("empty body")
	}
}
