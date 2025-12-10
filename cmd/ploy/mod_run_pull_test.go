package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestModRunPullRouting validates that `ploy mod run pull` routes to handleModRunPull.
// Uses t.Parallel since it does not use t.Setenv.
func TestModRunPullRouting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantErr    string
		wantOutput string
	}{
		{
			name:    "pull without run-name",
			args:    []string{"mod", "run", "pull"},
			wantErr: "run-name or run-id required",
		},
		{
			name:    "pull with empty run-name",
			args:    []string{"mod", "run", "pull", "   "},
			wantErr: "run-name or run-id required",
		},
		{
			name:       "pull with run-name routes correctly",
			args:       []string{"mod", "run", "pull", "my-run"},
			wantOutput: "mod run pull: would pull run",
		},
		{
			name:       "pull with dry-run flag",
			args:       []string{"mod", "run", "pull", "--dry-run", "my-run"},
			wantOutput: "dry-run: true",
		},
		{
			name:       "pull with origin flag",
			args:       []string{"mod", "run", "pull", "--origin", "upstream", "my-run"},
			wantOutput: `origin "upstream"`,
		},
		{
			name:       "pull with both flags",
			args:       []string{"mod", "run", "pull", "--origin", "upstream", "--dry-run", "my-run"},
			wantOutput: "dry-run: true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			if tc.wantOutput != "" && !strings.Contains(output, tc.wantOutput) {
				t.Errorf("output %q should contain %q", output, tc.wantOutput)
			}
		})
	}
}

// TestModRunPullUsageErrors validates that invalid flag combinations return appropriate errors.
// Uses t.Parallel since it does not use t.Setenv.
func TestModRunPullUsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		wantErr   string
		wantUsage bool // expect usage text in output
	}{
		{
			name:      "unknown flag",
			args:      []string{"mod", "run", "pull", "--unknown", "my-run"},
			wantErr:   "flag provided but not defined",
			wantUsage: true,
		},
		{
			name:      "origin flag without value",
			args:      []string{"mod", "run", "pull", "--origin"},
			wantErr:   "flag needs an argument",
			wantUsage: true,
		},
		{
			name:    "extra positional argument",
			args:    []string{"mod", "run", "pull", "my-run", "extra-arg"},
			wantErr: "unexpected argument: extra-arg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := executeCmd(tc.args, &buf)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantErr)
			}

			if tc.wantUsage {
				output := buf.String()
				if !strings.Contains(output, "Usage: ploy mod run pull") {
					t.Errorf("expected usage output, got %q", output)
				}
			}
		})
	}
}

// TestModRunPullUsageHelp validates that the usage text contains expected content.
func TestModRunPullUsageHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printModRunPullUsage(&buf)

	output := buf.String()

	// Verify usage line is present.
	if !strings.Contains(output, "Usage: ploy mod run pull") {
		t.Errorf("usage should contain command line, got %q", output)
	}

	// Verify flags are documented.
	if !strings.Contains(output, "--origin") {
		t.Errorf("usage should document --origin flag, got %q", output)
	}
	if !strings.Contains(output, "--dry-run") {
		t.Errorf("usage should document --dry-run flag, got %q", output)
	}

	// Verify argument is documented.
	if !strings.Contains(output, "<run-name|run-id>") {
		t.Errorf("usage should document run-name|run-id argument, got %q", output)
	}

	// Verify examples are present.
	if !strings.Contains(output, "Examples:") {
		t.Errorf("usage should contain examples section, got %q", output)
	}
}

// TestModRunPullDefaultOrigin validates that origin defaults to "origin".
func TestModRunPullDefaultOrigin(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := executeCmd([]string{"mod", "run", "pull", "my-run"}, &buf)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Default origin should be "origin".
	if !strings.Contains(output, `origin "origin"`) {
		t.Errorf("output should show default origin, got %q", output)
	}
}
