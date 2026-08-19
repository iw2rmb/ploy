package types

import (
	"encoding/json"
	"testing"
)

func TestEventIDWireContract(t *testing.T) {
	t.Parallel()

	values := []struct {
		name  string
		value EventID
		wire  string
		valid bool
	}{
		{name: "zero", value: 0, wire: "0", valid: true},
		{name: "positive", value: 42, wire: "42", valid: true},
		{name: "maximum", value: EventID(1<<63 - 1), wire: "9223372036854775807", valid: true},
		{name: "negative", value: -1},
		{name: "minimum", value: EventID(-1 << 63)},
	}
	for _, tt := range values {
		t.Run("encode "+tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.value.Valid(); got != tt.valid {
				t.Fatalf("Valid() = %v, want %v", got, tt.valid)
			}
			text, textErr := tt.value.MarshalText()
			jsonData, jsonErr := json.Marshal(tt.value)
			if !tt.valid {
				if textErr == nil || jsonErr == nil {
					t.Fatalf("negative EventID encoded with text error %v and JSON error %v", textErr, jsonErr)
				}
				return
			}
			if textErr != nil || string(text) != tt.wire {
				t.Errorf("MarshalText() = %q, %v; want %q, nil", text, textErr, tt.wire)
			}
			if jsonErr != nil || string(jsonData) != tt.wire {
				t.Errorf("json.Marshal() = %s, %v; want %s, nil", jsonData, jsonErr, tt.wire)
			}
		})
	}

	decodes := []struct {
		name    string
		input   string
		json    bool
		want    EventID
		wantErr bool
	}{
		{name: "text trims whitespace", input: " 42 ", want: 42},
		{name: "text rejects empty", input: "", wantErr: true},
		{name: "text rejects whitespace", input: "   ", wantErr: true},
		{name: "text rejects negative", input: "-1", wantErr: true},
		{name: "text rejects letters", input: "abc", wantErr: true},
		{name: "text rejects decimal", input: "12.5", wantErr: true},
		{name: "JSON number", input: "42", json: true, want: 42},
		{name: "JSON rejects negative", input: "-1", json: true, wantErr: true},
		{name: "JSON rejects null", input: "null", json: true, wantErr: true},
		{name: "JSON rejects string", input: `"42"`, json: true, wantErr: true},
	}
	for _, tt := range decodes {
		t.Run("decode "+tt.name, func(t *testing.T) {
			t.Parallel()
			var got EventID
			var err error
			if tt.json {
				err = json.Unmarshal([]byte(tt.input), &got)
			} else {
				err = got.UnmarshalText([]byte(tt.input))
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("decode %q error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("decode %q = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
