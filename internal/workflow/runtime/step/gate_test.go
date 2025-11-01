package step

import (
	"context"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func TestGateExecutor_Execute_Disabled(t *testing.T) {
	executor := NewGateExecutor()
	ctx := context.Background()

	tests := []struct {
		name string
		spec *contracts.StepGateSpec
	}{
		{
			name: "nil spec",
			spec: nil,
		},
		{
			name: "disabled gate",
			spec: &contracts.StepGateSpec{
				Enabled: false,
				Profile: "java",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tt.spec, "/tmp/workspace")
			if err != nil {
				t.Errorf("Execute() unexpected error: %v", err)
			}
			if result != nil {
				t.Errorf("Execute() expected nil result for disabled gate, got %+v", result)
			}
		})
	}
}

func TestGateExecutor_Execute_Enabled(t *testing.T) {
	executor := NewGateExecutor()
	ctx := context.Background()

	tests := []struct {
		name    string
		spec    *contracts.StepGateSpec
		wantErr bool
	}{
		{
			name: "enabled gate with java profile",
			spec: &contracts.StepGateSpec{
				Enabled: true,
				Profile: "java",
				Env:     map[string]string{"GATE_OPTION": "value"},
			},
			wantErr: false,
		},
		{
			name: "enabled gate with go profile",
			spec: &contracts.StepGateSpec{
				Enabled: true,
				Profile: "go",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.Execute(ctx, tt.spec, "/tmp/workspace")
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if result == nil {
					t.Errorf("Execute() expected non-nil result for enabled gate")
					return
				}
				if result.StaticChecks == nil {
					t.Errorf("Execute() result.StaticChecks should be initialized")
				}
				if result.LogFindings == nil {
					t.Errorf("Execute() result.LogFindings should be initialized")
				}
			}
		})
	}
}

func TestGateExecutor_Execute_CancelledContext(t *testing.T) {
	executor := NewGateExecutor()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	spec := &contracts.StepGateSpec{
		Enabled: true,
		Profile: "java",
	}

	// Current implementation doesn't check context, but test for future robustness
	_, err := executor.Execute(ctx, spec, "/tmp/workspace")
	// We don't expect error in current stub implementation
	if err != nil {
		t.Logf("Execute() with cancelled context returned error: %v", err)
	}
}
