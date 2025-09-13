package nvd

import (
	"testing"

	"github.com/iw2rmb/ploy/api/arf"
)

func TestGenerateRemediationGuidance_CISAOverrides(t *testing.T) {
	db := NewNVDDatabase()
	cve := NVDCVEInfo{
		EvaluatorSolution:  "update to latest",
		CISARequiredAction: "Apply vendor patch immediately",
	}
	out := db.generateRemediationGuidance(cve, []arf.AffectedPackage{{Name: "acme/widget"}})
	if out.Instructions != "Apply vendor patch immediately" {
		t.Fatalf("expected CISA to override, got %q", out.Instructions)
	}
}
