package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestLogLevelCanonicalizationAndJSONRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input string
		want  LogLevel
	}{
		{input: "debug", want: LogLevelDebug},
		{input: " INFO ", want: LogLevelInfo},
		{input: "Warn", want: LogLevelWarn},
		{input: "ERROR", want: LogLevelError},
	} {
		t.Run(tc.input, func(t *testing.T) {
			var got LogLevel
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
			var roundTrip LogLevel
			if err := json.Unmarshal(data, &roundTrip); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if roundTrip != tc.want {
				t.Fatalf("JSON round trip = %q, want %q", roundTrip, tc.want)
			}
		})
	}
}

func TestLogLevelValidationErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		value string
		want  error
	}{
		{name: "empty", value: " ", want: ErrEmpty},
		{name: "unknown", value: "trace", want: ErrInvalidLogLevel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var level LogLevel
			if err := level.UnmarshalText([]byte(tc.value)); !errors.Is(err, tc.want) {
				t.Fatalf("UnmarshalText() error = %v, want %v", err, tc.want)
			}
			if err := LogLevel(tc.value).Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tc.want)
			}
		})
	}
}
