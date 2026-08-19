package contracts

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestCommandSpecBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   any
		direct  *CommandSpec
		want    []string
		wire    string
		wantErr string
	}{
		{name: "shell string", input: " echo hi ", want: []string{"/bin/sh", "-c", "echo hi"}, wire: `"echo hi"`},
		{name: "exec string slice", input: []string{"echo", "hi"}, want: []string{"echo", "hi"}, wire: `["echo","hi"]`},
		{name: "exec interface slice", input: []any{"echo", "hi"}, want: []string{"echo", "hi"}, wire: `["echo","hi"]`},
		{name: "empty", direct: &CommandSpec{}, wire: "null"},
		{name: "exec takes precedence", direct: &CommandSpec{Shell: "ignored", Exec: []string{"echo", "used"}}, want: []string{"echo", "used"}, wire: `["echo","used"]`},
		{name: "invalid interface element", input: []any{"echo", 1}, wantErr: "expected string array element, got int"},
		{name: "invalid type", input: 42, wantErr: "expected string or array, got int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var spec CommandSpec
			var err error
			if tt.direct != nil {
				spec = *tt.direct
			} else {
				spec, err = ParseCommandSpec(tt.input)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseCommandSpec() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCommandSpec() error = %v", err)
			}
			if got := spec.ToSlice(); !slices.Equal(got, tt.want) {
				t.Errorf("ToSlice() = %v, want %v", got, tt.want)
			}
			wire, err := json.Marshal(spec)
			if err != nil || string(wire) != tt.wire {
				t.Errorf("json.Marshal() = %s, %v; want %s, nil", wire, err, tt.wire)
			}
			var decoded CommandSpec
			if err := json.Unmarshal([]byte(tt.wire), &decoded); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got := decoded.ToSlice(); !slices.Equal(got, tt.want) {
				t.Errorf("decoded ToSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}
