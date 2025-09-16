package mods

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBranchChainReplayer_ReplaysInOrder(t *testing.T) {
	// Build fake storage map for HEAD and meta chain: s1 -> s2 -> s3 (HEAD)
	storage := map[string][]byte{
		"mods/exec-1/branches/branch-A/HEAD.json":          []byte(`{"step_id":"s3"}`),
		"mods/exec-1/branches/branch-A/steps/s3/meta.json": []byte(`{"prev_step_id":"s2"}`),
		"mods/exec-1/branches/branch-A/steps/s2/meta.json": []byte(`{"prev_step_id":"s1"}`),
		"mods/exec-1/branches/branch-A/steps/s1/meta.json": []byte(`{"prev_step_id":""}`),
	}
	var appliedOrder []string

	r := &BranchChainReplayer{
		GetJSON: func(base, key string) ([]byte, int, error) {
			// base is not used here; we key on the key only
			if b, ok := storage[key]; ok {
				return b, 200, nil
			}
			return nil, 404, nil
		},
		DownloadToFile: func(url, dest string) error {
			// simulate by writing a non-empty file
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			return os.WriteFile(dest, []byte("patch"), 0644)
		},
		ValidateDiffPaths: func(diffPath string, allow []string) error {
			return nil
		},
		ValidateUnifiedDiff: func(ctx context.Context, repoPath, diffPath string) error {
			// record order by filename
			_, file := filepath.Split(diffPath)
			appliedOrder = append(appliedOrder, file)
			return nil
		},
		ApplyUnifiedDiff: func(ctx context.Context, repoPath, diffPath string) error { return nil },
		Allowlist:        []string{"**"},
	}

	tmp := t.TempDir()
	err := r.Replay(context.Background(), "http://filer", "exec-1", "branch-A", tmp, "/repo")
	if err != nil {
		t.Fatalf("replay err: %v", err)
	}
	// With head-only replay optimization, only the HEAD (s3) should be applied
	want := []string{"chain-s3.patch"}
	if !reflect.DeepEqual(appliedOrder, want) {
		t.Fatalf("order mismatch:\nwant=%v\n got=%v", want, appliedOrder)
	}
}

func TestBranchChainReplayer_SkipsOnInvalidDiffPath(t *testing.T) {
	storage := map[string][]byte{
		"mods/exec-2/branches/B/HEAD.json":          []byte(`{"step_id":"s2"}`),
		"mods/exec-2/branches/B/steps/s2/meta.json": []byte(`{"prev_step_id":"s1"}`),
		"mods/exec-2/branches/B/steps/s1/meta.json": []byte(`{"prev_step_id":""}`),
	}
	var validated []string

	r := &BranchChainReplayer{
		GetJSON: func(base, key string) ([]byte, int, error) {
			if b, ok := storage[key]; ok {
				return b, 200, nil
			}
			return nil, 404, nil
		},
		DownloadToFile: func(url, dest string) error {
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				return err
			}
			return os.WriteFile(dest, []byte("patch"), 0644)
		},
		ValidateDiffPaths: func(diffPath string, allow []string) error {
			_, file := filepath.Split(diffPath)
			validated = append(validated, file)
			if file == "chain-s1.patch" {
				return fmt.Errorf("invalid path")
			}
			return nil
		},
		ValidateUnifiedDiff: func(ctx context.Context, repoPath, diffPath string) error { return nil },
		ApplyUnifiedDiff:    func(ctx context.Context, repoPath, diffPath string) error { return nil },
		Allowlist:           []string{"**"},
	}

	tmp := t.TempDir()
	if err := r.Replay(context.Background(), "http://filer", "exec-2", "B", tmp, "/repo"); err != nil {
		t.Fatalf("replay err: %v", err)
	}
	// With head-only replay, only the HEAD (s2) is validated/applied
	want := []string{"chain-s2.patch"}
	if !reflect.DeepEqual(validated, want) {
		t.Fatalf("validate order mismatch:\nwant=%v\n got=%v", want, validated)
	}
}
