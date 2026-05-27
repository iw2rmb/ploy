package nodeagent

import (
	"context"
	"errors"
	"testing"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
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
