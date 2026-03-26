package contracts

import (
	"strings"
	"testing"
)

func TestModsSpecValidate_TmpBundleStep(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "valid bundle reference",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": ["config.json"]}}]
			}`,
		},
		{
			name: "valid bundle multiple entries",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": ["a.txt", "b.txt"]}}]
			}`,
		},
		{
			name: "legacy tmp_dir rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_dir": [{"name": "config.json", "content": "e2Zve30="}]}]
			}`,
			wantErr: "steps[0].tmp_dir: not supported; use tmp_bundle",
		},
		{
			name: "bundle_id missing",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"cid": "cid1", "digest": "sha256:abc", "entries": ["f.txt"]}}]
			}`,
			wantErr: "steps[0].tmp_bundle.bundle_id: required",
		},
		{
			name: "cid missing",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "digest": "sha256:abc", "entries": ["f.txt"]}}]
			}`,
			wantErr: "steps[0].tmp_bundle.cid: required",
		},
		{
			name: "digest missing",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "entries": ["f.txt"]}}]
			}`,
			wantErr: "steps[0].tmp_bundle.digest: required",
		},
		{
			name: "entries empty",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": []}}]
			}`,
			wantErr: "steps[0].tmp_bundle.entries: required",
		},
		{
			name: "entry path separator rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": ["sub/file.txt"]}}]
			}`,
			wantErr: "steps[0].tmp_bundle.entries[0]: must be a plain filename with no path separators",
		},
		{
			name: "entry dotdot rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": [".."]}}]
			}`,
			wantErr: "steps[0].tmp_bundle.entries[0]: must be a plain filename with no path separators",
		},
		{
			name: "duplicate entries rejected",
			input: `{
				"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": ["x", "x"]}}]
			}`,
			wantErr: `steps[0].tmp_bundle.entries[1]: duplicate "x"`,
		},
		{
			name: "no tmp_bundle is valid",
			input: `{
				"steps": [{"image": "img:latest"}]
			}`,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModsSpecJSON([]byte(tc.input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestModsSpecValidate_TmpBundleHealing(t *testing.T) {
	base := `{
		"steps": [{"image": "img:latest"}],
		"build_gate": {
			"router": {"image": "router:latest"},
			"healing": {"by_error_kind": {"infra": {"retries": 1, "image": "healer:latest", %s}}}
		}
	}`

	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{
			name:  "valid bundle in healing action",
			field: `"tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": ["patch.diff"]}`,
		},
		{
			name:    "legacy tmp_dir in healing rejected",
			field:   `"tmp_dir": [{"name": "patch.diff", "content": "aGVsbG8="}]`,
			wantErr: "build_gate.healing.by_error_kind.infra.tmp_dir: not supported; use tmp_bundle",
		},
		{
			name:    "missing bundle_id in healing rejected",
			field:   `"tmp_bundle": {"cid": "cid1", "digest": "sha256:abc", "entries": ["f.txt"]}`,
			wantErr: "build_gate.healing.by_error_kind.infra.tmp_bundle.bundle_id: required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := strings.ReplaceAll(base, "%s", tc.field)
			_, err := ParseModsSpecJSON([]byte(input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestModsSpecValidate_TmpBundleRouter(t *testing.T) {
	base := `{
		"steps": [{"image": "img:latest"}],
		"build_gate": {"router": {"image": "router:latest", %s}}
	}`

	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{
			name:  "valid bundle in router",
			field: `"tmp_bundle": {"bundle_id": "b1", "cid": "cid1", "digest": "sha256:abc", "entries": ["rules.json"]}`,
		},
		{
			name:    "legacy tmp_dir in router rejected",
			field:   `"tmp_dir": [{"name": "rules.json", "content": "e30="}]`,
			wantErr: "build_gate.router.tmp_dir: not supported; use tmp_bundle",
		},
		{
			name:    "missing cid in router rejected",
			field:   `"tmp_bundle": {"bundle_id": "b1", "digest": "sha256:abc", "entries": ["f.txt"]}`,
			wantErr: "build_gate.router.tmp_bundle.cid: required",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			input := strings.ReplaceAll(base, "%s", tc.field)
			_, err := ParseModsSpecJSON([]byte(input))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestParseModsSpecJSON_TmpBundleEntriesCanonicalized(t *testing.T) {
	spec, err := ParseModsSpecJSON([]byte(`{
		"steps": [{"image": "img:latest", "tmp_bundle": {"bundle_id": "b1", "cid": "c1", "digest": "d1", "entries": [" config.json "]}}]
	}`))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON() unexpected error: %v", err)
	}
	if got, want := spec.Steps[0].TmpBundle.Entries[0], "config.json"; got != want {
		t.Fatalf("tmp_bundle.entries[0] got %q, want %q", got, want)
	}
}

