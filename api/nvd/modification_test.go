package nvd

import (
	"testing"

	"github.com/iw2rmb/ploy/api/security"
)

func TestGenerateModificationGuidance_CISAOverrides(t *testing.T) {
	db := NewNVDDatabase()
	cve := NVDCVEInfo{
		EvaluatorSolution:  "update to latest",
		CISARequiredAction: "Apply vendor patch immediately",
	}
	out := db.generateModificationGuidance(cve, []security.AffectedPackage{{Name: "acme/widget"}})
	if out.Instructions != "Apply vendor patch immediately" {
		t.Fatalf("expected CISA to override, got %q", out.Instructions)
	}
}
