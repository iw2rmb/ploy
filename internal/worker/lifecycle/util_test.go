package lifecycle

import "testing"

func TestExtractFirstLine(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"single line", "single line"},
		{"first\nsecond\nthird", "first"},
		{" first with space \n second ", "first with space"},
	}
	for _, c := range cases {
		if got := extractFirstLine(c.in); got != c.want {
			t.Fatalf("extractFirstLine(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
