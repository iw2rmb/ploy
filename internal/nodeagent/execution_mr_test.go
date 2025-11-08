package nodeagent

import "testing"

func TestShouldCreateMR(t *testing.T) {
	tests := []struct {
		name           string
		terminalStatus string
		options        map[string]any
		want           bool
	}{
		{name: "success_flag_true", terminalStatus: "succeeded", options: map[string]any{"mr_on_success": true}, want: true},
		{name: "success_flag_false", terminalStatus: "succeeded", options: map[string]any{"mr_on_success": false}, want: false},
		{name: "success_flag_missing", terminalStatus: "succeeded", options: map[string]any{}, want: false},
		{name: "fail_flag_true", terminalStatus: "failed", options: map[string]any{"mr_on_fail": true}, want: true},
		{name: "fail_flag_false", terminalStatus: "failed", options: map[string]any{"mr_on_fail": false}, want: false},
		{name: "fail_flag_missing", terminalStatus: "failed", options: map[string]any{}, want: false},
		{name: "non_bool_values_ignored_success", terminalStatus: "succeeded", options: map[string]any{"mr_on_success": "true"}, want: false},
		{name: "non_bool_values_ignored_fail", terminalStatus: "failed", options: map[string]any{"mr_on_fail": "true"}, want: false},
		{name: "other_status_never_triggers", terminalStatus: "cancelled", options: map[string]any{"mr_on_success": true, "mr_on_fail": true}, want: false},
		{name: "gate_failure_with_mr_on_fail", terminalStatus: "failed", options: map[string]any{"mr_on_fail": true}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldCreateMR(tt.terminalStatus, tt.options)
			if got != tt.want {
				t.Fatalf("shouldCreateMR(%q, %v) = %v, want %v", tt.terminalStatus, tt.options, got, tt.want)
			}
		})
	}
}
