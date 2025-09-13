package sbom

import "testing"

func TestEvaluateCompliance(t *testing.T) {
	c := EvaluateCompliance(SBOMSecurityMetrics{})
	if c != "compliant" {
		t.Fatalf("expected compliant, got %s", c)
	}

	c = EvaluateCompliance(SBOMSecurityMetrics{KnownVulns: 1})
	if c != "partial_compliance" {
		t.Fatalf("expected partial_compliance for known vulns, got %s", c)
	}

	c = EvaluateCompliance(SBOMSecurityMetrics{High: 1})
	if c != "partial_compliance" {
		t.Fatalf("expected partial_compliance for high, got %s", c)
	}
}
