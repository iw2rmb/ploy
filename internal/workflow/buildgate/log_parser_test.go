package buildgate

import "testing"

func TestDefaultLogParserDetectsKnownPatterns(t *testing.T) {
	parser := NewDefaultLogParser()
	log := "fatal error: unable to access 'https://example.com/repo.git': Permission denied\n" +
		"go: module github.com/acme/project found in multiple modules:\n" +
		"undefined reference to `SomeSymbol'\n" +
		"no space left on device"

	findings := parser.Parse(log)
	if len(findings) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(findings))
	}

	expectCodes := map[string]bool{
		"kb.git.auth":                   false,
		"kb.go.module_conflict":         false,
		"kb.linker.undefined_reference": false,
		"kb.infrastructure.disk_full":   false,
	}
	for _, finding := range findings {
		if _, ok := expectCodes[finding.Code]; !ok {
			t.Fatalf("unexpected finding code %q", finding.Code)
		}
		expectCodes[finding.Code] = true
		if finding.Message == "" {
			t.Fatalf("expected message for finding %q", finding.Code)
		}
		if finding.Severity == "" {
			t.Fatalf("expected severity for finding %q", finding.Code)
		}
		if finding.Evidence == "" {
			t.Fatalf("expected evidence for finding %q", finding.Code)
		}
	}
	for code, seen := range expectCodes {
		if !seen {
			t.Fatalf("expected finding for code %q", code)
		}
	}
}

func TestDefaultLogParserDeduplicatesMatches(t *testing.T) {
	parser := NewDefaultLogParser()
	log := "undefined reference to `SomeSymbol'\nundefined reference to `SomeSymbol'"

	findings := parser.Parse(log)
	if len(findings) != 1 {
		t.Fatalf("expected single deduplicated finding, got %d", len(findings))
	}
}
