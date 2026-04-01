package deploy

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDetectExisting(t *testing.T) {
	ctx := context.Background()

	sampleServerCert := `-----BEGIN CERTIFICATE-----
MIIBkTCCATegAwIBAgIQYoYxqvs=
-----END CERTIFICATE-----`

	tests := []struct {
		name           string
		opts           ProvisionOptions
		runnerBehavior func(command string, args []string) error
		runnerOutput   func(command string, args []string) string
		wantFound      bool
		wantClusterID  string
		wantErr        bool
	}{
		{
			name: "cluster exists with valid cluster ID",
			opts: ProvisionOptions{
				Address:      "192.168.1.10",
				User:         "root",
				Port:         22,
				IdentityFile: "/home/user/.ssh/id_rsa",
			},
			runnerBehavior: func(command string, args []string) error {
				// All checks pass
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				// Return cluster ID in CN format when extracting certificate
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
					return "ployd-abc123def456"
				}
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "cat /etc/ploy/pki/server.crt") {
					return sampleServerCert
				}
				return ""
			},
			wantFound:     true,
			wantClusterID: "abc123def456",
			wantErr:       false,
		},
		{
			name: "no cluster present - CA cert missing",
			opts: ProvisionOptions{
				Address: "192.168.1.11",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				// Fail on CA cert check
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "test -f /etc/ploy/pki/ca.crt") {
					return errors.New("file not found")
				}
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				return ""
			},
			wantFound:     false,
			wantClusterID: "",
			wantErr:       false,
		},
		{
			name: "no cluster present - config missing",
			opts: ProvisionOptions{
				Address: "192.168.1.12",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				// CA cert exists but config does not
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "test -f /etc/ploy/ployd.yaml") {
					return errors.New("file not found")
				}
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				return ""
			},
			wantFound:     false,
			wantClusterID: "",
			wantErr:       false,
		},
		{
			name: "cluster exists but server cert missing (node installation)",
			opts: ProvisionOptions{
				Address: "192.168.1.13",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				// CA cert and config exist but server cert does not
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "test -f /etc/ploy/pki/server.crt") {
					return errors.New("file not found")
				}
				return nil
			},
			runnerOutput:  func(command string, args []string) string { return "" },
			wantFound:     true,
			wantClusterID: "",
			wantErr:       false,
		},
		{
			name: "uses openssl -in with server.crt",
			opts: ProvisionOptions{
				Address: "192.168.1.19",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				joined := strings.Join(args, " ")
				if command == "ssh" && strings.Contains(joined, "openssl x509") && strings.Contains(joined, "commonName") {
					if !strings.Contains(joined, "-in /etc/ploy/pki/server.crt") {
						return errors.New("missing -in server.crt")
					}
				}
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
					return "ployd-checkin123"
				}
				return ""
			},
			wantFound:     true,
			wantClusterID: "checkin123",
			wantErr:       false,
		},
		{
			name: "cluster exists but CN extraction fails",
			opts: ProvisionOptions{
				Address: "192.168.1.14",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				// All files exist but CN extraction fails
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
					return errors.New("openssl command failed")
				}
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "cat /etc/ploy/pki/server.crt") {
					return sampleServerCert
				}
				return ""
			},
			wantFound:     true,
			wantClusterID: "",
			wantErr:       false,
		},
		{
			name: "cluster exists with uppercase cluster ID",
			opts: ProvisionOptions{
				Address: "192.168.1.15",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
					return "ployd-ABC123"
				}
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "cat /etc/ploy/pki/server.crt") {
					return sampleServerCert
				}
				return ""
			},
			wantFound:     true,
			wantClusterID: "ABC123",
			wantErr:       false,
		},
		{
			name: "invalid CN format (no prefix)",
			opts: ProvisionOptions{
				Address: "192.168.1.16",
				User:    "root",
			},
			runnerBehavior: func(command string, args []string) error {
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
					return "somethingelse-abc123"
				}
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "cat /etc/ploy/pki/server.crt") {
					return sampleServerCert
				}
				return ""
			},
			wantFound:     true,
			wantClusterID: "",
			wantErr:       false,
		},
		{
			name: "nil runner uses systemRunner (no cluster)",
			opts: ProvisionOptions{
				Address: "192.168.1.17",
			},
			runnerBehavior: nil,
			runnerOutput:   nil,
			wantFound:      false,
			wantClusterID:  "",
			wantErr:        false,
		},
		{
			name: "empty address returns error",
			opts: ProvisionOptions{
				User: "root",
			},
			runnerBehavior: func(command string, args []string) error {
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				return ""
			},
			wantFound:     false,
			wantClusterID: "",
			wantErr:       true,
		},
		{
			name: "default user and port applied",
			opts: ProvisionOptions{
				Address: "192.168.1.18",
				// No User or Port specified
			},
			runnerBehavior: func(command string, args []string) error {
				// Verify default user root@ is in target
				if command == "ssh" && len(args) > 0 {
					target := args[len(args)-2] // target is second-to-last in most commands
					if !strings.HasPrefix(target, "root@") {
						return errors.New("expected default user root@")
					}
				}
				return nil
			},
			runnerOutput: func(command string, args []string) string {
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "commonName") {
					return "ployd-xyz789"
				}
				if len(args) > 0 && strings.Contains(strings.Join(args, " "), "cat /etc/ploy/pki/server.crt") {
					return sampleServerCert
				}
				return ""
			},
			wantFound:     true,
			wantClusterID: "xyz789",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create stub runner
			var runner Runner
			if tt.runnerBehavior != nil {
				runner = &stubRunner{
					behavior: tt.runnerBehavior,
					output:   tt.runnerOutput,
				}
			}
			tt.opts.Runner = runner

			got, err := DetectExisting(ctx, runner, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectExisting() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Found != tt.wantFound {
				t.Errorf("DetectExisting() Found = %v, want %v", got.Found, tt.wantFound)
			}
			if got.ClusterID.String() != tt.wantClusterID {
				t.Errorf("DetectExisting() ClusterID = %q, want %q", got.ClusterID.String(), tt.wantClusterID)
			}
		})
	}
}

func TestExtractClusterIDFromCN(t *testing.T) {
	tests := []struct {
		name   string
		cn     string
		wantID string
	}{
		{
			name:   "valid format with lowercase hex",
			cn:     "ployd-abc123def456",
			wantID: "abc123def456",
		},
		{
			name:   "valid format with uppercase hex",
			cn:     "ployd-ABC123DEF456",
			wantID: "ABC123DEF456",
		},
		{
			name:   "valid format with mixed case",
			cn:     "ployd-AbC123DeF",
			wantID: "AbC123DeF",
		},
		{
			name:   "valid format with hyphens",
			cn:     "ployd-wispy-dust-1337",
			wantID: "wispy-dust-1337",
		},
		{
			name:   "invalid format - missing prefix",
			cn:     "abc123def456",
			wantID: "",
		},
		{
			name:   "invalid format - wrong prefix",
			cn:     "ploy-abc123",
			wantID: "",
		},
		{
			name:   "invalid format - unsupported separator in ID",
			cn:     "ployd-abc_123",
			wantID: "",
		},
		{
			name:   "invalid format - spaces",
			cn:     "ployd- abc123",
			wantID: "",
		},
		{
			name:   "empty string",
			cn:     "",
			wantID: "",
		},
		{
			name:   "just the prefix",
			cn:     "ployd-",
			wantID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractClusterIDFromCN(tt.cn)
			if got != tt.wantID {
				t.Errorf("extractClusterIDFromCN(%q) = %q, want %q", tt.cn, got, tt.wantID)
			}
		})
	}
}

// stubRunner is a test double for the Runner interface.
type stubRunner struct {
	behavior func(command string, args []string) error
	output   func(command string, args []string) string
}

func (s *stubRunner) Run(ctx context.Context, command string, args []string, stdin io.Reader, streams IOStreams) error {
	// Write output if defined
	if s.output != nil && streams.Stdout != nil {
		output := s.output(command, args)
		if output != "" {
			_, _ = io.WriteString(streams.Stdout, output)
		}
	}

	// Return error based on behavior
	if s.behavior != nil {
		return s.behavior(command, args)
	}
	return nil
}
