package recipes

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func captureOutput(fn func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String()
}

func TestUsageMentionsRootCommand(t *testing.T) {
	output := captureOutput(func() {
		printRecipesUsage()
	})

	if !strings.Contains(output, "ploy recipe") {
		t.Fatalf("expected usage to mention 'ploy recipe', got: %s", output)
	}
	if strings.Contains(output, "ploy arf recipe") {
		t.Fatalf("expected usage to drop 'arf' prefix, got: %s", output)
	}
}
