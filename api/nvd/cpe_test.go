package nvd

import (
	"testing"
)

func TestParseCPEToPackage_Basic(t *testing.T) {
	db := NewNVDDatabase()
	got := db.parseCPEToPackage("cpe:2.3:a:acme:widget:1.2.3:*:*:*:*:*:*:*")
	if got == nil {
		t.Fatalf("expected non-nil package")
	}
	if got.Name != "acme/widget" || len(got.AffectedVersions) == 0 || got.AffectedVersions[0] != "1.2.3" {
		t.Fatalf("unexpected package mapping: %#v", got)
	}
}
