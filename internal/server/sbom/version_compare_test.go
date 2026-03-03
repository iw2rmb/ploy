package sbom

import "testing"

func TestCompareVersions_JavaMavenQualifierAware(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a    string
		b    string
		want int
	}{
		{a: "1.10.0", b: "1.2.0", want: 1},
		{a: "1.0.0", b: "1.0.0-rc1", want: 1},
		{a: "1.0.0-rc2", b: "1.0.0-rc1", want: 1},
		{a: "1.0.0-beta1", b: "1.0.0-rc1", want: -1},
		{a: "1.0.1", b: "1.0.0", want: 1},
		{a: "1.0", b: "1.0.0", want: 0},
	}

	for _, tc := range cases {
		got := CompareVersions("java", "maven", tc.a, tc.b)
		if got < 0 {
			got = -1
		} else if got > 0 {
			got = 1
		}
		if got != tc.want {
			t.Fatalf("CompareVersions(java,maven,%q,%q)=%d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCompareVersions_DefaultLoose(t *testing.T) {
	t.Parallel()
	if got := CompareVersions("go", "", "2.10.0", "2.9.0"); got <= 0 {
		t.Fatalf("expected 2.10.0 > 2.9.0 for default comparator, got %d", got)
	}
}
