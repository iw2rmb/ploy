package ploy_test

import (
	"os"
	"strings"
	"testing"
)

func TestDocumentationCleanupMarkedComplete(t *testing.T) {
	roadmap, err := os.ReadFile("roadmap/shift/08-documentation-cleanup.md")
	if err != nil {
		t.Fatalf("read roadmap slice: %v", err)
	}
	if !strings.Contains(string(roadmap), "- [x]") {
		t.Fatalf("roadmap/shift/08-documentation-cleanup.md must be marked complete with - [x]")
	}

	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if !strings.Contains(string(readme), "08-documentation-cleanup") {
		t.Fatalf("README.md must mention roadmap slice 08-documentation-cleanup in the completed list")
	}
}
