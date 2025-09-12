package transflow

import "testing"

func TestNewBranchStep_GeneratesIDAndKey(t *testing.T) {
	bs := NewBranchStep("e-1", "b-1")
	if bs.ID == "" || bs.DiffKey == "" {
		t.Fatalf("expected id and key: %+v", bs)
	}
	if !strContains(bs.DiffKey, "transflow/e-1/branches/b-1/steps/") {
		t.Fatalf("unexpected diff key: %s", bs.DiffKey)
	}
	if !strContains(bs.DiffKey, "/"+bs.ID+"/") && !strContains(bs.DiffKey, "/"+bs.ID) {
		t.Fatalf("diff key doesn't include step id: %s", bs.DiffKey)
	}
}
