package transflow

import "testing"

func TestComputeBranchDiffKey(t *testing.T) {
	key := computeBranchDiffKey("e-1", "b-2", "s-3")
	if key != "transflow/e-1/branches/b-2/steps/s-3/diff.patch" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestMakeORWVarsContainsExpected(t *testing.T) {
	vars := makeORWVars("/tmp/x", "e-9", "transflow/e-9/branches/b/steps/s/diff.patch", "http://filer:8888")
	if vars["TRANSFLOW_CONTEXT_DIR"] != "/tmp/x" {
		t.Fatal("context dir mismatch")
	}
	if vars["PLOY_TRANSFLOW_EXECUTION_ID"] != "e-9" {
		t.Fatal("exec id mismatch")
	}
	if vars["TRANSFLOW_DIFF_KEY"] == "" {
		t.Fatal("diff key missing")
	}
	if vars["PLOY_SEAWEEDFS_URL"] != "http://filer:8888" {
		t.Fatal("seaweed url mismatch")
	}
}
