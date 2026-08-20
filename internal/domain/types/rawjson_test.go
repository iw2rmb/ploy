package types

import (
	"bytes"
	"testing"
)

type rawJSONValue interface {
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}

func TestRawJSONDomainTypesCopyUnmarshalInput(t *testing.T) {
	tests := []struct {
		name     string
		newValue func() rawJSONValue
	}{
		{name: "diff summary", newValue: func() rawJSONValue { return new(DiffSummary) }},
		{name: "run stats", newValue: func() rawJSONValue { return new(RunStats) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := []byte(`{"value":"original"}`)
			value := tt.newValue()
			if err := value.UnmarshalJSON(input); err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}

			copy(input, []byte(`{"value":"modified"}`))
			got, err := value.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if want := []byte(`{"value":"original"}`); !bytes.Equal(got, want) {
				t.Fatalf("MarshalJSON() = %s, want %s", got, want)
			}
		})
	}
}
