package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseCommandSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     any
		wantShell string
		wantExec  []string
		wantErr   string
	}{
		{
			name:      "shell string",
			input:     " echo hi ",
			wantShell: "echo hi",
		},
		{
			name:     "exec from []string",
			input:    []string{"echo", "hi"},
			wantExec: []string{"echo", "hi"},
		},
		{
			name:     "exec from []any",
			input:    []any{"echo", "hi"},
			wantExec: []string{"echo", "hi"},
		},
		{
			name:    "invalid []any element type",
			input:   []any{"echo", 1},
			wantErr: "expected string array element, got int",
		},
		{
			name:    "invalid type",
			input:   42,
			wantErr: "expected string or array, got int",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseCommandSpec(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Shell != tt.wantShell {
				t.Fatalf("Shell = %q, want %q", got.Shell, tt.wantShell)
			}
			if len(got.Exec) != len(tt.wantExec) {
				t.Fatalf("Exec = %v, want %v", got.Exec, tt.wantExec)
			}
			for i := range got.Exec {
				if got.Exec[i] != tt.wantExec[i] {
					t.Fatalf("Exec[%d] = %q, want %q", i, got.Exec[i], tt.wantExec[i])
				}
			}
		})
	}
}

func TestCommandSpec_ToSlice(t *testing.T) {
	tests := []struct {
		name string
		cmd  CommandSpec
		want []string
	}{
		{
			name: "shell string",
			cmd:  CommandSpec{Shell: "echo hello"},
			want: []string{"/bin/sh", "-c", "echo hello"},
		},
		{
			name: "exec array",
			cmd:  CommandSpec{Exec: []string{"echo", "hello"}},
			want: []string{"echo", "hello"},
		},
		{
			name: "empty",
			cmd:  CommandSpec{},
			want: nil,
		},
		{
			name: "exec takes precedence",
			cmd:  CommandSpec{Shell: "ignored", Exec: []string{"echo", "used"}},
			want: []string{"echo", "used"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cmd.ToSlice()
			if len(got) != len(tt.want) {
				t.Errorf("ToSlice() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ToSlice()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCommandSpec_JSONMarshal tests JSON marshaling of CommandSpec.
func TestCommandSpec_JSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		cmd  CommandSpec
		want string
	}{
		{
			name: "shell string",
			cmd:  CommandSpec{Shell: "echo hello"},
			want: `"echo hello"`,
		},
		{
			name: "exec array",
			cmd:  CommandSpec{Exec: []string{"echo", "hello"}},
			want: `["echo","hello"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("json.Marshal() = %s, want %s", data, tt.want)
			}
		})
	}
}

// TestCommandSpec_JSONUnmarshal tests JSON unmarshaling of CommandSpec.
func TestCommandSpec_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantShell string
		wantExec  []string
	}{
		{
			name:      "shell string",
			input:     `"echo hello"`,
			wantShell: "echo hello",
		},
		{
			name:     "exec array",
			input:    `["echo", "hello"]`,
			wantExec: []string{"echo", "hello"},
		},
		{
			name:  "null",
			input: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd CommandSpec
			if err := json.Unmarshal([]byte(tt.input), &cmd); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if cmd.Shell != tt.wantShell {
				t.Errorf("Shell = %q, want %q", cmd.Shell, tt.wantShell)
			}
			if len(cmd.Exec) != len(tt.wantExec) {
				t.Errorf("Exec = %v, want %v", cmd.Exec, tt.wantExec)
			}
		})
	}
}
