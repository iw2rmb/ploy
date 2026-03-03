package sbom

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/iw2rmb/ploy/internal/domain/types"
)

func TestExtractRowsFromBundle_ParsesSPDXAndCycloneDX(t *testing.T) {
	t.Parallel()

	spdx := `{
	  "spdxVersion": "SPDX-2.3",
	  "packages": [
	    {"name":"org.springframework:spring-core","versionInfo":"6.1.8"},
	    {"name":"org.springframework:spring-core","versionInfo":"6.1.8"},
	    {"name":"skip-empty-version","versionInfo":""}
	  ]
	}`
	cdx := `{
	  "bomFormat":"CycloneDX",
	  "components":[
	    {"name":"com.fasterxml.jackson.core:jackson-databind","version":"2.18.2"},
	    {"name":"parent","version":"1.0.0","components":[
	      {"name":"nested-lib","version":"3.4.5"}
	    ]}
	  ]
	}`

	bundle := mustBundle(t, map[string]string{
		"out/build-gate.log":   "not json",
		"out/sbom.spdx.json":   spdx,
		"out/cyclonedx.json":   cdx,
		"out/other/report.txt": "skip me",
	})

	jobID := types.JobID("job-123")
	repoID := types.RepoID("repo-456")
	rows, err := ExtractRowsFromBundle(bundle, jobID, repoID)
	if err != nil {
		t.Fatalf("ExtractRowsFromBundle error: %v", err)
	}

	got := map[string]Row{}
	for _, row := range rows {
		got[row.Lib+"@"+row.Ver] = row
	}

	wantKeys := []string{
		"com.fasterxml.jackson.core:jackson-databind@2.18.2",
		"nested-lib@3.4.5",
		"org.springframework:spring-core@6.1.8",
		"parent@1.0.0",
	}
	if len(got) != len(wantKeys) {
		t.Fatalf("row count = %d, want %d; rows=%v", len(got), len(wantKeys), rows)
	}
	for _, key := range wantKeys {
		row, ok := got[key]
		if !ok {
			t.Fatalf("missing row %q", key)
		}
		if row.JobID != jobID {
			t.Fatalf("row %q job_id = %q, want %q", key, row.JobID, jobID)
		}
		if row.RepoID != repoID {
			t.Fatalf("row %q repo_id = %q, want %q", key, row.RepoID, repoID)
		}
	}
}

func TestExtractRowsFromBundle_InvalidSBOMDoesNotBlockRows(t *testing.T) {
	t.Parallel()

	valid := `{
	  "spdxVersion":"SPDX-2.3",
	  "packages":[{"name":"lib-a","versionInfo":"1.2.3"}]
	}`
	invalid := `{
	  "spdxVersion":"SPDX-2.3",
	  "packages":[{"name":"broken"`

	bundle := mustBundle(t, map[string]string{
		"out/good.spdx.json": valid,
		"out/bad.spdx.json":  invalid,
	})

	rows, err := ExtractRowsFromBundle(bundle, "job-a", "repo-a")
	if err != nil {
		t.Fatalf("expected no fatal error for invalid sbom file, got %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	if rows[0].Lib != "lib-a" || rows[0].Ver != "1.2.3" {
		t.Fatalf("row = %+v, want lib-a@1.2.3", rows[0])
	}
}

func mustBundle(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		payload := []byte(body)
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(payload)),
		}); err != nil {
			t.Fatalf("write tar header %q: %v", name, err)
		}
		if _, err := tw.Write(payload); err != nil {
			t.Fatalf("write tar payload %q: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
