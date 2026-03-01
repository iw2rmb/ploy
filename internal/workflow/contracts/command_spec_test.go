package contracts

import (
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
