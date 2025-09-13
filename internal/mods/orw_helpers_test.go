package mods

import "testing"

func TestComputeBranchDiffKey(t *testing.T) {
	key := computeBranchDiffKey("e-1", "b-2", "s-3")
	if key != "mods/e-1/branches/b-2/steps/s-3/diff.patch" {
		t.Fatalf("unexpected key: %s", key)
	}
}

func TestMakeORWVarsContainsExpected(t *testing.T) {
	vars := makeORWVars("/tmp/x", "e-9", "mods/e-9/branches/b/steps/s/diff.patch", "http://filer:8888")
	if vars["MODS_CONTEXT_DIR"] != "/tmp/x" {
		t.Fatal("context dir mismatch")
	}
	if vars["PLOY_MODS_EXECUTION_ID"] != "e-9" {
		t.Fatal("exec id mismatch")
	}
	if vars["MODS_DIFF_KEY"] == "" {
		t.Fatal("diff key missing")
	}
	if vars["PLOY_SEAWEEDFS_URL"] != "http://filer:8888" {
		t.Fatal("seaweed url mismatch")
	}
}
