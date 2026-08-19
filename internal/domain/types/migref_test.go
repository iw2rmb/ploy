package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestMigRefWireContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    MigRef
		wantErr error
	}{
		{name: "NanoID", input: "abc123", want: "abc123"},
		{name: "name", input: "my-mig", want: "my-mig"},
		{name: "underscore", input: "MigName_v2", want: "MigName_v2"},
		{name: "UUID-like", input: "12345678-1234-1234-1234-123456789012", want: "12345678-1234-1234-1234-123456789012"},
		{name: "trims whitespace", input: "  my-mig  ", want: "my-mig"},
		{name: "empty", input: "", wantErr: ErrEmpty},
		{name: "whitespace only", input: "   ", wantErr: ErrEmpty},
		{name: "slash", input: "my/mig", wantErr: ErrInvalidMigRef},
		{name: "question mark", input: "mig?name", wantErr: ErrInvalidMigRef},
		{name: "space", input: "my mig", wantErr: ErrInvalidMigRef},
		{name: "tab", input: "my\tmod", wantErr: ErrInvalidMigRef},
		{name: "newline", input: "my\nmod", wantErr: ErrInvalidMigRef},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := MigRef(tt.input).Validate(); !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}

			text, marshalErr := MigRef(tt.input).MarshalText()
			jsonData, jsonMarshalErr := json.Marshal(MigRef(tt.input))
			var fromText, fromJSON MigRef
			textErr := fromText.UnmarshalText([]byte(tt.input))
			quotedInput, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("json.Marshal(input) error = %v", err)
			}
			jsonErr := json.Unmarshal(quotedInput, &fromJSON)

			if tt.wantErr != nil {
				for name, err := range map[string]error{
					"MarshalText": marshalErr, "json.Marshal": jsonMarshalErr,
					"UnmarshalText": textErr, "json.Unmarshal": jsonErr,
				} {
					if !errors.Is(err, tt.wantErr) {
						t.Errorf("%s error = %v, want %v", name, err, tt.wantErr)
					}
				}
				return
			}

			if marshalErr != nil || string(text) != string(tt.want) {
				t.Errorf("MarshalText() = %q, %v; want %q, nil", text, marshalErr, tt.want)
			}
			wantJSON, _ := json.Marshal(string(tt.want))
			if jsonMarshalErr != nil || string(jsonData) != string(wantJSON) {
				t.Errorf("json.Marshal() = %s, %v; want %s, nil", jsonData, jsonMarshalErr, wantJSON)
			}
			if textErr != nil || fromText != tt.want {
				t.Errorf("UnmarshalText() = %q, %v; want %q, nil", fromText, textErr, tt.want)
			}
			if jsonErr != nil || fromJSON != tt.want {
				t.Errorf("json.Unmarshal() = %q, %v; want %q, nil", fromJSON, jsonErr, tt.want)
			}
		})
	}
}
