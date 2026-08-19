package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestProtocolCanonicalizationAndJSONRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input string
		want  Protocol
	}{
		{input: " tcp ", want: ProtocolTCP},
		{input: "UDP", want: ProtocolUDP},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var got Protocol
			if err := got.UnmarshalText([]byte(tc.input)); err != nil {
				t.Fatalf("UnmarshalText() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("UnmarshalText() = %q, want %q", got, tc.want)
			}
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			var roundTrip Protocol
			if err := json.Unmarshal(data, &roundTrip); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if roundTrip != tc.want {
				t.Fatalf("JSON round trip = %q, want %q", roundTrip, tc.want)
			}
		})
	}
}

func TestProtocolValidationErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value string
		want  error
	}{
		{name: "empty", value: " ", want: ErrEmpty},
		{name: "unknown", value: "icmp", want: ErrInvalidProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var protocol Protocol
			if err := protocol.UnmarshalText([]byte(tc.value)); !errors.Is(err, tc.want) {
				t.Fatalf("UnmarshalText() error = %v, want %v", err, tc.want)
			}
			if err := Protocol(tc.value).Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.want)
			}
		})
	}
}
