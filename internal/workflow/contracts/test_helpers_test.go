package contracts

import (
	"strings"
	"testing"
)

// requireValidationErr asserts that err matches wantSubstr. If wantSubstr is
// empty the error must be nil; otherwise err must be non-nil and contain the
// substring.
func requireValidationErr(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	if wantSubstr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", wantSubstr)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("error = %q, want substring %q", err.Error(), wantSubstr)
	}
}
