package analysis

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func extractTarEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	entries := make(map[string]string)

	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tarReader.Next: %v", err)
		}
		content, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatalf("io.ReadAll: %v", err)
		}
		entries[header.Name] = string(content)
	}
	return entries
}

func TestNomadPylintAnalyzerMetadataAndAutoFix(t *testing.T) {
	analyzer := NewNomadPylintAnalyzer(nil)

	info := analyzer.GetAnalyzerInfo()
	if info.Language != "python" {
		t.Fatalf("unexpected language: %s", info.Language)
	}
	if len(analyzer.GetSupportedFileTypes()) == 0 {
		t.Fatalf("expected supported file types")
	}
	if analyzer.ValidateConfiguration(nil) != nil {
		t.Fatalf("ValidateConfiguration should not error")
	}
	if analyzer.Configure(nil) != nil {
		t.Fatalf("Configure should not error")
	}
	if analyzer.CanAutoFix(Issue{RuleName: "unused-import"}) != true {
		t.Fatalf("expected unused-import to be auto-fixable")
	}
	if analyzer.CanAutoFix(Issue{RuleName: "not-fixable"}) {
		t.Fatalf("unexpected auto-fixable rule")
	}
	fixes, err := analyzer.GenerateFixSuggestions(Issue{})
	if err != nil {
		t.Fatalf("GenerateFixSuggestions: %v", err)
	}
	if len(fixes) != 0 {
		t.Fatalf("expected empty fixes slice")
	}
}

func TestNomadPylintCreateCodebaseArchive(t *testing.T) {
	analyzer := NewNomadPylintAnalyzer(nil)

	_, err := analyzer.createCodebaseArchive(Codebase{Files: []string{"main.go"}})
	if err == nil {
		t.Fatalf("expected error when no python files present")
	}

	data, err := analyzer.createCodebaseArchive(Codebase{Files: []string{"pkg/main.py", "README.md"}})
	if err != nil {
		t.Fatalf("createCodebaseArchive: %v", err)
	}
	entries := extractTarEntries(t, data)
	if _, ok := entries["pkg/main.py"]; !ok {
		t.Fatalf("expected python file in archive, got %#v", entries)
	}
}

func TestNomadESLintAnalyzerUtilities(t *testing.T) {
	analyzer := NewNomadESLintAnalyzer(nil)
	if analyzer.GetAnalyzerInfo().Language != "javascript" {
		t.Fatalf("unexpected analyzer language")
	}
	if analyzer.CanAutoFix(Issue{RuleName: "semi"}) != true {
		t.Fatalf("expected ESLint rule to be auto-fixable")
	}
	if analyzer.CanAutoFix(Issue{RuleName: "nope"}) {
		t.Fatalf("unexpected ESLint rule marked auto-fixable")
	}

	data, err := analyzer.createCodebaseArchive(Codebase{Files: []string{"web/app.tsx", "docs/readme.md"}})
	if err != nil {
		t.Fatalf("createCodebaseArchive: %v", err)
	}
	entries := extractTarEntries(t, data)
	if _, ok := entries["web/app.tsx"]; !ok {
		t.Fatalf("expected TSX file in archive")
	}

	_, err = analyzer.createCodebaseArchive(Codebase{Files: []string{"docs/readme.md"}})
	if err == nil {
		t.Fatalf("expected error when no js/ts files present")
	}
}

func TestNomadGolangCIAnalyzerUtilities(t *testing.T) {
	analyzer := NewNomadGolangCIAnalyzer(nil)
	if analyzer.GetAnalyzerInfo().Language != "go" {
		t.Fatalf("unexpected analyzer language")
	}
	if !analyzer.CanAutoFix(Issue{RuleName: "gofmt"}) {
		t.Fatalf("expected gofmt to be auto-fixable")
	}
	if analyzer.CanAutoFix(Issue{RuleName: "staticcheck"}) {
		t.Fatalf("unexpected rule marked auto-fixable")
	}

	fixes, err := analyzer.GenerateFixSuggestions(Issue{RuleName: "gofmt", File: "main.go"})
	if err != nil {
		t.Fatalf("GenerateFixSuggestions: %v", err)
	}
	if len(fixes) == 0 {
		t.Fatalf("expected fix suggestion for gofmt rule")
	}

	_, err = analyzer.createCodebaseArchive(Codebase{Files: []string{"README.md"}})
	if err == nil {
		t.Fatalf("expected error when no go files present")
	}

	data, err := analyzer.createCodebaseArchive(Codebase{Files: []string{"main.go", "go.mod"}})
	if err != nil {
		t.Fatalf("createCodebaseArchive: %v", err)
	}
	entries := extractTarEntries(t, data)
	if _, ok := entries["main.go"]; !ok {
		t.Fatalf("expected go file in archive")
	}
	if content, ok := entries["go.mod"]; !ok || content == "" {
		t.Fatalf("expected go.mod content in archive")
	}
}
