package handlers

import "testing"

func TestParseLastEventID(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"42", 42},
		{"  123  ", 123},
		{"0", 0},
		{"abc", 0},
		{"12abc", 0},
		{"-5", -5},
	}
	for _, c := range cases {
		if got := parseLastEventID(c.in); got != c.want {
			t.Fatalf("parseLastEventID(%q)=%d want %d", c.in, got, c.want)
		}
	}
}
