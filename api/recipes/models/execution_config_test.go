package models

import (
	"testing"
	"time"
)

func validExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		Parallelism: 2,
		MaxDuration: Duration{Duration: 5 * time.Minute},
		RetryPolicy: RetryPolicy{
			MaxAttempts: 2,
			Backoff:     Duration{Duration: time.Second},
			MaxBackoff:  Duration{Duration: 5 * time.Second},
		},
		Sandbox: SandboxConfig{
			Enabled:        true,
			MaxMemory:      "1024B",
			MaxCPU:         2,
			MaxDiskUsage:   "2048B",
			IsolationLevel: "medium",
			DockerImage:    "repo/image:tag",
		},
		Environment: map[string]string{"CUSTOM_VAR": "1"},
	}
}

func TestExecutionConfigValidateSuccess(t *testing.T) {
	cfg := validExecutionConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestExecutionConfigValidateFailures(t *testing.T) {
	cases := []struct {
		name string
		cfg  ExecutionConfig
	}{
		{
			name: "negative parallelism",
			cfg: func() ExecutionConfig {
				cfg := validExecutionConfig()
				cfg.Parallelism = -1
				return cfg
			}(),
		},
		{
			name: "reserved env var",
			cfg: func() ExecutionConfig {
				cfg := validExecutionConfig()
				cfg.Environment = map[string]string{"PATH": "/tmp"}
				return cfg
			}(),
		},
		{
			name: "invalid sandbox memory",
			cfg: func() ExecutionConfig {
				cfg := validExecutionConfig()
				cfg.Sandbox.MaxMemory = "invalid"
				return cfg
			}(),
		},
		{
			name: "retry backoff greater than max",
			cfg: func() ExecutionConfig {
				cfg := validExecutionConfig()
				cfg.RetryPolicy.Backoff = Duration{Duration: 10 * time.Second}
				cfg.RetryPolicy.MaxBackoff = Duration{Duration: 5 * time.Second}
				return cfg
			}(),
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatalf("expected validation failure for %s", tt.name)
			}
		})
	}
}

func TestExecutionConfigSetDefaults(t *testing.T) {
	cfg := ExecutionConfig{}
	cfg.SetDefaults()

	if cfg.Parallelism != 1 {
		t.Fatalf("Parallelism default = %d, want 1", cfg.Parallelism)
	}
	if cfg.MaxDuration.Duration != 15*time.Minute {
		t.Fatalf("MaxDuration default = %v, want 15m", cfg.MaxDuration)
	}
	if cfg.RetryPolicy.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts default = %d, want 1", cfg.RetryPolicy.MaxAttempts)
	}
	if cfg.RetryPolicy.Backoff.Duration != 5*time.Second {
		t.Fatalf("Backoff default = %v, want 5s", cfg.RetryPolicy.Backoff)
	}
	// Sandbox defaults apply only when enabled
	cfg.Sandbox.Enabled = true
	cfg.Sandbox.SetDefaults()
	if cfg.Sandbox.MaxMemory == "" || cfg.Sandbox.IsolationLevel == "" {
		t.Fatalf("Sandbox defaults not applied when enabled: %+v", cfg.Sandbox)
	}
}

func TestSandboxValidateFailures(t *testing.T) {
	sandbox := SandboxConfig{Enabled: true, MaxMemory: "bad-size"}
	if err := sandbox.Validate(); err == nil {
		t.Fatalf("expected error for invalid memory size")
	}

	sandbox = SandboxConfig{Enabled: true, MaxMemory: "512MB", MaxCPU: -1}
	if err := sandbox.Validate(); err == nil {
		t.Fatalf("expected error for negative CPU")
	}

	sandbox = SandboxConfig{Enabled: true, MaxMemory: "512MB", MaxCPU: 2, MaxDiskUsage: "bad"}
	if err := sandbox.Validate(); err == nil {
		t.Fatalf("expected error for invalid disk size")
	}

	sandbox = SandboxConfig{Enabled: true, MaxMemory: "512MB", MaxCPU: 2, MaxDiskUsage: "1GB", IsolationLevel: "invalid"}
	if err := sandbox.Validate(); err == nil {
		t.Fatalf("expected error for invalid isolation level")
	}

	sandbox = SandboxConfig{Enabled: true, MaxMemory: "512MB", MaxCPU: 2, MaxDiskUsage: "1GB", IsolationLevel: "medium", DockerImage: "bad image"}
	if err := sandbox.Validate(); err == nil {
		t.Fatalf("expected error for invalid docker image")
	}
}

func TestRetryPolicyValidate(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 3, Backoff: Duration{Duration: time.Second}, MaxBackoff: Duration{Duration: 5 * time.Second}}
	if err := policy.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	bad := RetryPolicy{MaxAttempts: 10}
	if err := bad.Validate(); err == nil {
		t.Fatalf("expected error for excessive max attempts")
	}
}
