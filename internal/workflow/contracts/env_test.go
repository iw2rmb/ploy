package contracts

import "testing"

func TestCopyEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   map[string]string
		want map[string]string
	}{
		{name: "nil input", in: nil, want: nil},
		{name: "empty input", in: map[string]string{}, want: nil},
		{name: "non-empty input", in: map[string]string{"A": "1", "B": "2"}, want: map[string]string{"A": "1", "B": "2"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := CopyEnv(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len(CopyEnv()) = %d, want %d", len(got), len(tc.want))
			}
			for key, wantVal := range tc.want {
				if got[key] != wantVal {
					t.Fatalf("CopyEnv()[%q] = %q, want %q", key, got[key], wantVal)
				}
			}
		})
	}
}

func TestCopyEnv_CopyIndependence(t *testing.T) {
	t.Parallel()

	in := map[string]string{"A": "1"}
	got := CopyEnv(in)
	got["A"] = "2"
	if in["A"] != "1" {
		t.Fatalf("input map mutated: got %q, want %q", in["A"], "1")
	}
}

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		base     map[string]string
		override map[string]string
		want     map[string]string
	}{
		{name: "both nil", base: nil, override: nil, want: nil},
		{name: "both empty", base: map[string]string{}, override: map[string]string{}, want: nil},
		{name: "base only", base: map[string]string{"A": "1"}, override: nil, want: map[string]string{"A": "1"}},
		{name: "override only", base: nil, override: map[string]string{"B": "2"}, want: map[string]string{"B": "2"}},
		{
			name:     "override wins conflicts",
			base:     map[string]string{"A": "1", "B": "2"},
			override: map[string]string{"B": "3", "C": "4"},
			want:     map[string]string{"A": "1", "B": "3", "C": "4"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := MergeEnv(tc.base, tc.override)
			if len(got) != len(tc.want) {
				t.Fatalf("len(MergeEnv()) = %d, want %d", len(got), len(tc.want))
			}
			for key, wantVal := range tc.want {
				if got[key] != wantVal {
					t.Fatalf("MergeEnv()[%q] = %q, want %q", key, got[key], wantVal)
				}
			}
		})
	}
}

func TestMergeEnv_DoesNotMutateInputs(t *testing.T) {
	t.Parallel()

	base := map[string]string{"A": "1"}
	override := map[string]string{"B": "2"}
	got := MergeEnv(base, override)
	got["A"] = "x"
	got["B"] = "y"

	if base["A"] != "1" {
		t.Fatalf("base mutated: got %q, want %q", base["A"], "1")
	}
	if override["B"] != "2" {
		t.Fatalf("override mutated: got %q, want %q", override["B"], "2")
	}
}
