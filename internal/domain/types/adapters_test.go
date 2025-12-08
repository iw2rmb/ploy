package types

import (
	"testing"
	"time"
)

func TestStringsAdapter(t *testing.T) {
	in := []Protocol{ProtocolTCP, ProtocolUDP}
	got := Strings(in)
	want := []string{"tcp", "udp"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Strings() = %#v, want %#v", got, want)
	}
}

func TestStringPtrAdapter(t *testing.T) {
	// Test with RunID (canonical run identifier; TicketID is a deprecated alias).
	if p := StringPtr(RunID("")); p != nil {
		t.Fatalf("StringPtr(empty) = %#v, want nil", *p)
	}
	id := RunID("abc")
	p := StringPtr(id)
	if p == nil || *p != "abc" {
		t.Fatalf("StringPtr(%q) = %#v, want %q", id, p, "abc")
	}
}

func TestDurationStdRoundtrip(t *testing.T) {
	d := Duration(90 * time.Second)
	if StdDuration(d) != 90*time.Second {
		t.Fatalf("StdDuration mismatch: got %v", StdDuration(d))
	}
	if FromStdDuration(2*time.Minute) != Duration(2*time.Minute) {
		t.Fatalf("FromStdDuration mismatch: got %v", FromStdDuration(2*time.Minute))
	}
}
