package validation

import (
	"strings"
	"testing"
)

// TestValidateCPULimit tests CPU limit validation
func TestValidateCPULimit(t *testing.T) {
	tests := []struct {
		name        string
		cpuLimit    string
		wantErr     bool
		errContains string
	}{
		// Valid CPU limits
		{"valid millicores", "500m", false, ""},
		{"valid cores", "2", false, ""},
		{"valid decimal cores", "1.5", false, ""},
		{"valid fractional", "0.5", false, ""},
		{"valid high millicores", "2000m", false, ""},
		{"valid single millicore", "1m", false, ""},
		{"valid max cores", "64", false, ""},

		// Invalid formats
		{"empty", "", true, "empty"},
		{"invalid suffix", "500x", true, "invalid CPU limit format"},
		{"negative", "-500m", true, "invalid CPU limit format"},
		{"negative cores", "-2", true, "invalid CPU limit format"},
		{"zero", "0", true, "must be greater than zero"},
		{"zero millicores", "0m", true, "must be greater than zero"},
		{"text", "unlimited", true, "invalid CPU limit format"},
		{"spaces", "500 m", true, "invalid CPU limit format"},
		{"multiple dots", "1.2.3", true, "invalid CPU limit format"},

		// Out of range
		{"too high", "100000", true, "exceeds maximum"},
		{"too high millicores", "100000000m", true, "exceeds maximum"},
		{"too low millicores", "0.1m", true, "invalid CPU limit format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCPULimit(tt.cpuLimit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCPULimit(%q) expected error but got none", tt.cpuLimit)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCPULimit(%q) error = %v, want error containing %q", tt.cpuLimit, err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCPULimit(%q) unexpected error: %v", tt.cpuLimit, err)
				}
			}
		})
	}
}

// TestValidateMemoryLimit tests memory limit validation
func TestValidateMemoryLimit(t *testing.T) {
	tests := []struct {
		name        string
		memLimit    string
		wantErr     bool
		errContains string
	}{
		// Valid memory limits
		{"valid bytes", "1073741824", false, ""},
		{"valid KB", "5120K", false, ""},
		{"valid KiB", "5120Ki", false, ""},
		{"valid MB", "512M", false, ""},
		{"valid MiB", "512Mi", false, ""},
		{"valid GB", "2G", false, ""},
		{"valid GiB", "2Gi", false, ""},
		{"valid TB", "1T", false, ""},
		{"valid TiB", "1Ti", false, ""},
		{"valid lowercase", "512m", false, ""},
		{"valid lowercase gi", "2gi", false, ""},

		// Invalid formats
		{"empty", "", true, "empty"},
		{"invalid suffix", "512X", true, "invalid memory limit format"},
		{"negative", "-512M", true, "negative"},
		{"zero", "0", true, "must be greater than zero"},
		{"zero MB", "0M", true, "must be greater than zero"},
		{"text", "unlimited", true, "invalid"},
		{"spaces", "512 Mi", true, "invalid"},
		{"decimal with bytes", "1.5", true, "invalid"},
		{"decimal MB", "1.5M", true, "invalid memory limit format"},

		// Out of range
		{"too low", "1", true, "below minimum"},
		{"too low KB", "1K", true, "is below minimum"},
		{"too high", "10000G", true, "exceeds maximum"},
		{"too high TB", "1000T", true, "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMemoryLimit(tt.memLimit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateMemoryLimit(%q) expected error but got none", tt.memLimit)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateMemoryLimit(%q) error = %v, want error containing %q", tt.memLimit, err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateMemoryLimit(%q) unexpected error: %v", tt.memLimit, err)
				}
			}
		})
	}
}

// TestValidateDiskLimit tests disk space limit validation
func TestValidateDiskLimit(t *testing.T) {
	tests := []struct {
		name        string
		diskLimit   string
		wantErr     bool
		errContains string
	}{
		// Valid disk limits
		{"valid MB", "100M", false, ""},
		{"valid MiB", "100Mi", false, ""},
		{"valid GB", "10G", false, ""},
		{"valid GiB", "10Gi", false, ""},
		{"valid TB", "1T", false, ""},
		{"valid TiB", "1Ti", false, ""},
		{"valid large", "500G", false, ""},

		// Invalid formats
		{"empty", "", true, "empty"},
		{"bytes only", "1073741824", true, "must specify unit"},
		{"invalid suffix", "10X", true, "invalid"},
		{"negative", "-10G", true, "negative"},
		{"zero", "0G", true, "must be greater than zero"},

		// Out of range
		{"too low", "1M", true, "below minimum"},
		{"too high", "10000T", true, "exceeds maximum"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDiskLimit(tt.diskLimit)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateDiskLimit(%q) expected error but got none", tt.diskLimit)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateDiskLimit(%q) error = %v, want error containing %q", tt.diskLimit, err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateDiskLimit(%q) unexpected error: %v", tt.diskLimit, err)
				}
			}
		})
	}
}

// TestValidateResourceConstraints tests validation of all resource constraints
func TestValidateResourceConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints ResourceConstraints
		wantErr     bool
		errContains string
	}{
		{
			name: "all valid",
			constraints: ResourceConstraints{
				CPU:    "2",
				Memory: "4Gi",
				Disk:   "100G",
			},
			wantErr: false,
		},
		{
			name: "valid millicores and MiB",
			constraints: ResourceConstraints{
				CPU:    "500m",
				Memory: "512Mi",
				Disk:   "10G",
			},
			wantErr: false,
		},
		{
			name:        "empty constraints allowed",
			constraints: ResourceConstraints{},
			wantErr:     false,
		},
		{
			name: "partial constraints allowed",
			constraints: ResourceConstraints{
				CPU: "1",
			},
			wantErr: false,
		},
		{
			name: "invalid CPU",
			constraints: ResourceConstraints{
				CPU:    "invalid",
				Memory: "1G",
			},
			wantErr:     true,
			errContains: "CPU",
		},
		{
			name: "invalid memory",
			constraints: ResourceConstraints{
				CPU:    "1",
				Memory: "invalid",
			},
			wantErr:     true,
			errContains: "memory",
		},
		{
			name: "invalid disk",
			constraints: ResourceConstraints{
				CPU:    "1",
				Memory: "1G",
				Disk:   "invalid",
			},
			wantErr:     true,
			errContains: "disk",
		},
		{
			name: "multiple invalid",
			constraints: ResourceConstraints{
				CPU:    "-1",
				Memory: "0M",
				Disk:   "wrong",
			},
			wantErr:     true,
			errContains: "CPU", // Should report first error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateResourceConstraints(tt.constraints)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateResourceConstraints() expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateResourceConstraints() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateResourceConstraints() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestParseCPUValue tests CPU value parsing
func TestParseCPUValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue float64
		wantErr   bool
	}{
		{"cores", "2", 2.0, false},
		{"decimal cores", "1.5", 1.5, false},
		{"millicores", "500m", 0.5, false},
		{"millicores 2000", "2000m", 2.0, false},
		{"millicores 100", "100m", 0.1, false},
		{"fractional", "0.25", 0.25, false},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := ParseCPUValue(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseCPUValue(%q) expected error but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParseCPUValue(%q) unexpected error: %v", tt.input, err)
				}
				if value != tt.wantValue {
					t.Errorf("ParseCPUValue(%q) = %v, want %v", tt.input, value, tt.wantValue)
				}
			}
		})
	}
}

// TestParseMemoryValue tests memory value parsing
func TestParseMemoryValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantBytes int64
		wantErr   bool
	}{
		{"bytes", "1073741824", 1073741824, false},
		{"KB", "1024K", 1024 * 1024, false},
		{"KiB", "1024Ki", 1024 * 1024, false},
		{"MB", "100M", 100 * 1024 * 1024, false},
		{"MiB", "100Mi", 100 * 1024 * 1024, false},
		{"GB", "1G", 1024 * 1024 * 1024, false},
		{"GiB", "1Gi", 1024 * 1024 * 1024, false},
		{"lowercase", "512m", 512 * 1024 * 1024, false},
		{"invalid", "abc", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bytes, err := ParseMemoryValue(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseMemoryValue(%q) expected error but got none", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("ParseMemoryValue(%q) unexpected error: %v", tt.input, err)
				}
				if bytes != tt.wantBytes {
					t.Errorf("ParseMemoryValue(%q) = %v, want %v", tt.input, bytes, tt.wantBytes)
				}
			}
		})
	}
}

// BenchmarkValidateCPULimit benchmarks CPU limit validation
func BenchmarkValidateCPULimit(b *testing.B) {
	testCases := []string{"500m", "2", "1.5", "2000m"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateCPULimit(testCases[i%len(testCases)])
	}
}

// BenchmarkValidateMemoryLimit benchmarks memory limit validation
func BenchmarkValidateMemoryLimit(b *testing.B) {
	testCases := []string{"512Mi", "1G", "2048M", "4Gi"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ValidateMemoryLimit(testCases[i%len(testCases)])
	}
}
