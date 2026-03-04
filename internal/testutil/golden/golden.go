package golden

import (
	"os"
	"path/filepath"
	"testing"
)

func LoadBytes(t testing.TB, parts ...string) []byte {
	t.Helper()
	p := filepath.Join(parts...)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read golden %s: %v", p, err)
	}
	return b
}

func LoadString(t testing.TB, parts ...string) string {
	t.Helper()
	return string(LoadBytes(t, parts...))
}
