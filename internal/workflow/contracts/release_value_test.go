package contracts

import "testing"

func TestParseReleaseValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		want    string
		wantErr bool
	}{
		{name: "string", input: " 17 ", want: "17"},
		{name: "int", input: 17, want: "17"},
		{name: "int64", input: int64(17), want: "17"},
		{name: "whole float64", input: 17.0, want: "17"},
		{name: "decimal float64", input: 3.9, want: "3.9"},
		{name: "invalid bool", input: true, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseReleaseValue(tt.input, "field.release")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseReleaseValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

