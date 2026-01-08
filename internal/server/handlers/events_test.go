package handlers

import (
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestParseLastEventID(t *testing.T) {
	cases := []struct {
		in   string
		want domaintypes.EventID
	}{
		{"", 0},
		{"42", 42},
		{"  123  ", 123},
		{"0", 0},
		{"abc", 0},   // invalid: not a number
		{"12abc", 0}, // invalid: not a valid integer
		{"-5", 0},    // invalid: negative values are rejected
		{"-1", 0},    // invalid: negative values are rejected
	}
	for _, c := range cases {
		if got := parseLastEventID(c.in); got != c.want {
			t.Fatalf("parseLastEventID(%q)=%d want %d", c.in, got, c.want)
		}
	}
}
