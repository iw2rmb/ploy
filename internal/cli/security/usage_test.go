package security

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func captureUsageOutput(fn func()) string {
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = originalStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	_ = r.Close()
	return buf.String()
}

func TestUsageDoesNotMentionHealthOrCache(t *testing.T) {
	output := captureUsageOutput(printSecurityUsage)

	if !strings.Contains(output, "ploy security") {
		t.Fatalf("expected usage to mention 'ploy security', got: %s", output)
	}
	if strings.Contains(output, "health") {
		t.Fatalf("expected usage to omit health commands, got: %s", output)
	}
	if strings.Contains(output, "cache") {
		t.Fatalf("expected usage to omit cache commands, got: %s", output)
	}
}
