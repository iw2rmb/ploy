package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestRunController_uploadGateErrorStatus_UploadsProcessError(t *testing.T) {
	t.Parallel()

	req := newStartRunRequest()
	server, cap := newStatusCaptureServer(t, req.JobID.String())
	controller := newTestController(t, newAgentConfig(server.URL))

	err := errors.New("BUILD_GATE_STACK_DETECT_FAILED: no supported Java version configuration found in build.gradle")
	controller.uploadGateErrorStatus(context.Background(), req, err, 125*time.Millisecond)

	if cap.Status != types.JobStatusError.String() {
		t.Fatalf("status = %q, want %q", cap.Status, types.JobStatusError.String())
	}
	if cap.ExitCode == nil || *cap.ExitCode != -1 {
		t.Fatalf("exit_code = %v, want -1", cap.ExitCode)
	}
	if got := cap.Stats["error"]; got != err.Error() {
		t.Fatalf("stats.error = %v, want %q", got, err.Error())
	}
	if got := cap.Stats["duration_ms"]; got != float64(125) {
		t.Fatalf("stats.duration_ms = %v, want 125", got)
	}
}

func TestRunController_buildGateStats_GateFailureError(t *testing.T) {
	t.Parallel()

	gradleError := "Non-executable gradlew: got 644, need 7XX (chmod +x gradlew). Provide commit or branch with the correct gradlew permission."
	longLogLine := "generic\t" + strings.Repeat("x", maxDerivedJobErrorText)
	tests := []struct {
		name      string
		metadata  *contracts.BuildGateStageMetadata
		wantError string
	}{
		{
			name: "failed gradle gate prefers remediation line",
			metadata: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Passed: false, Tool: "gradle"}},
				LogFindings: []contracts.BuildGateLogFinding{{
					Severity: "error",
					Message:  "Gradle wrapper validation failed",
				}},
				LogsText: "preparing gate\n" + gradleError + "\nBUILD FAILED\n",
			},
			wantError: gradleError,
		},
		{
			name: "failed gate normalizes and bounds first log line",
			metadata: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Passed: false, Tool: "gradle"}},
				LogsText:     "\n  " + longLogLine + "  \nsecond failure\n",
			},
			wantError: "generic " + strings.Repeat("x", maxDerivedJobErrorText-len("generic ")),
		},
		{
			name: "successful gate omits error",
			metadata: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Passed: true, Tool: "gradle"}},
				LogsText:     gradleError,
			},
		},
		{
			name: "failed gate with empty metadata omits error",
			metadata: &contracts.BuildGateStageMetadata{
				StaticChecks: []contracts.BuildGateStaticCheckReport{{Passed: false, Tool: "gradle"}},
			},
		},
	}

	controller := &runController{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stats := controller.buildGateStats(tc.metadata, 125*time.Millisecond)
			var payload map[string]any
			if err := json.Unmarshal(stats, &payload); err != nil {
				t.Fatalf("json.Unmarshal(stats) error = %v", err)
			}
			got, exists := payload["error"]
			if tc.wantError == "" {
				if exists {
					t.Fatalf("stats.error = %#v, want absent", got)
				}
				return
			}
			if got != tc.wantError {
				t.Fatalf("stats.error = %#v, want %q", got, tc.wantError)
			}
		})
	}
}
