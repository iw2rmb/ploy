package contracts

import "testing"

func TestStackFieldsMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                            string
		lang, tool, release             string
		wantLang, wantTool, wantRelease string
		want                            bool
	}{
		{
			name:        "exact match",
			lang:        "java",
			tool:        "maven",
			release:     "17",
			wantLang:    "java",
			wantTool:    "maven",
			wantRelease: "17",
			want:        true,
		},
		{
			name:        "language and tool are case-insensitive",
			lang:        " Java ",
			tool:        " MAVEN ",
			release:     "17",
			wantLang:    "java",
			wantTool:    "maven",
			wantRelease: "17",
			want:        true,
		},
		{
			name:        "release ignores surrounding whitespace",
			lang:        "java",
			tool:        "maven",
			release:     " 17 ",
			wantLang:    "java",
			wantTool:    "maven",
			wantRelease: "17",
			want:        true,
		},
		{
			name:        "release remains case-sensitive",
			lang:        "java",
			tool:        "maven",
			release:     "RC1",
			wantLang:    "java",
			wantTool:    "maven",
			wantRelease: "rc1",
			want:        false,
		},
		{
			name:        "language wildcard",
			lang:        "go",
			tool:        "go",
			release:     "1.25",
			wantLang:    "",
			wantTool:    "go",
			wantRelease: "1.25",
			want:        true,
		},
		{
			name:        "tool wildcard",
			lang:        "java",
			tool:        "gradle",
			release:     "17",
			wantLang:    "java",
			wantTool:    "",
			wantRelease: "17",
			want:        true,
		},
		{
			name:        "release wildcard",
			lang:        "java",
			tool:        "maven",
			release:     "21",
			wantLang:    "java",
			wantTool:    "maven",
			wantRelease: "",
			want:        true,
		},
		{
			name:        "tool mismatch",
			lang:        "java",
			tool:        "gradle",
			release:     "17",
			wantLang:    "java",
			wantTool:    "maven",
			wantRelease: "17",
			want:        false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := StackFieldsMatch(tc.lang, tc.tool, tc.release, tc.wantLang, tc.wantTool, tc.wantRelease)
			if got != tc.want {
				t.Fatalf("StackFieldsMatch() = %v, want %v", got, tc.want)
			}
		})
	}
}
