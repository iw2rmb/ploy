package nodeagent

import (
	"errors"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/step"
)

func TestFinalizeStandardJobOutputs(t *testing.T) {
	t.Parallel()

	existingErr := errors.New("existing failure")
	finalizeErr := errors.New("finalize failure")

	tests := []struct {
		name           string
		runErr         error
		exitCode       int
		finalizeErr    error
		wantNil        bool
		wantExisting   bool
		wantFinalizing bool
	}{
		{
			name:           "successful run returns finalizer failure",
			runErr:         nil,
			exitCode:       0,
			finalizeErr:    finalizeErr,
			wantFinalizing: true,
		},
		{
			name:         "existing runtime error is preserved",
			runErr:       existingErr,
			exitCode:     0,
			finalizeErr:  finalizeErr,
			wantExisting: true,
		},
		{
			name:        "non-zero exit keeps fail semantics",
			runErr:      nil,
			exitCode:    1,
			finalizeErr: finalizeErr,
			wantNil:     true,
		},
		{
			name:         "successful finalizer keeps prior run error",
			runErr:       existingErr,
			exitCode:     0,
			finalizeErr:  nil,
			wantExisting: true,
		},
		{
			name:        "successful finalizer with clean run stays nil",
			runErr:      nil,
			exitCode:    0,
			finalizeErr: nil,
			wantNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rc := &runController{}
			cfg := standardJobConfig{
				FinalizeOutputs: func(_, _ string) error {
					return tt.finalizeErr
				},
			}

			got := rc.finalizeStandardJobOutputs(
				StartRunRequest{},
				cfg,
				t.TempDir(),
				t.TempDir(),
				tt.runErr,
				step.Result{ExitCode: tt.exitCode},
			)

			if tt.wantNil {
				if got != nil {
					t.Fatalf("error = %v, want nil", got)
				}
				return
			}
			if tt.wantExisting {
				if !errors.Is(got, existingErr) {
					t.Fatalf("error = %v, want existing failure", got)
				}
				return
			}
			if tt.wantFinalizing {
				if got == nil {
					t.Fatal("error = nil, want finalize error")
				}
				if !strings.Contains(got.Error(), "finalize job outputs") {
					t.Fatalf("error = %v, want finalize wrapper", got)
				}
				if !errors.Is(got, finalizeErr) {
					t.Fatalf("error = %v, want wrapped finalize failure", got)
				}
				return
			}
			t.Fatal("invalid test case")
		})
	}
}
