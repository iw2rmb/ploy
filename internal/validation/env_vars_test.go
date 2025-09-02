package validation

import (
	"fmt"
	"strings"
	"testing"
)

// TestValidateEnvVarName tests environment variable name validation
func TestValidateEnvVarName(t *testing.T) {
	tests := []struct {
		name        string
		varName     string
		wantErr     bool
		errContains string
	}{
		// Valid names
		{"valid uppercase", "DATABASE_URL", false, ""},
		{"valid with numbers", "API_KEY_123", false, ""},
		{"valid lowercase", "debug_mode", false, ""},
		{"valid mixed case", "MyVariable", false, ""},
		{"valid single char", "X", false, ""},
		{"valid underscore start", "_PRIVATE", false, ""},
		
		// Invalid names
		{"empty name", "", true, "empty"},
		{"spaces in name", "MY VAR", true, "invalid character"},
		{"starts with number", "123_VAR", true, "must start with letter or underscore"},
		{"special characters", "VAR-NAME", true, "invalid character"},
		{"equals sign", "VAR=VALUE", true, "invalid character"},
		{"dollar sign", "$VAR", true, "invalid character"},
		{"brackets", "VAR[0]", true, "invalid character"},
		{"dots", "my.var", true, "invalid character"},
		{"command injection", "VAR;rm -rf", true, "invalid character"},
		{"null byte", "VAR\x00", true, "invalid character"},
		
		// Reserved names that should be rejected
		{"reserved PATH", "PATH", true, "reserved"},
		{"reserved HOME", "HOME", true, "reserved"},
		{"reserved USER", "USER", true, "reserved"},
		{"reserved SHELL", "SHELL", true, "reserved"},
		{"reserved PWD", "PWD", true, "reserved"},
		{"reserved LD_PRELOAD", "LD_PRELOAD", true, "reserved"},
		{"reserved LD_LIBRARY_PATH", "LD_LIBRARY_PATH", true, "reserved"},
		
		// Length limits
		{"too long", strings.Repeat("A", 256), true, "too long"},
		{"max length valid", strings.Repeat("A", 255), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvVarName(tt.varName)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateEnvVarName(%q) expected error but got none", tt.varName)
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateEnvVarName(%q) error = %v, want error containing %q", tt.varName, err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateEnvVarName(%q) unexpected error: %v", tt.varName, err)
				}
			}
		})
	}
}

// TestValidateEnvVarValue tests environment variable value validation
func TestValidateEnvVarValue(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantErr     bool
		errContains string
	}{
		// Valid values
		{"valid simple", "simple-value", false, ""},
		{"valid with spaces", "this is a value", false, ""},
		{"valid URL", "https://example.com/path?query=1", false, ""},
		{"valid path", "/usr/local/bin:/usr/bin", false, ""},
		{"valid JSON", `{"key": "value", "num": 123}`, false, ""},
		{"valid base64", "dGVzdCBkYXRhCg==", false, ""},
		{"valid empty", "", false, ""},
		{"valid special chars", "!@#$%^&*()_+-=[]{}|;:,.<>?", false, ""},
		
		// Invalid values
		{"null byte", "value\x00data", true, "null byte"},
		{"control char", "value\x01data", true, "control character"},
		{"too long", strings.Repeat("x", 65537), true, "too long"},
		
		// Values that look like injection but should be allowed (escaped/quoted properly)
		{"looks like command", "rm -rf /", false, ""},
		{"SQL-like", "'; DROP TABLE users; --", false, ""},
		{"script tags", "<script>alert('xss')</script>", false, ""},
		
		// Maximum valid length
		{"max length", strings.Repeat("x", 65536), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvVarValue(tt.value)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateEnvVarValue() expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateEnvVarValue() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateEnvVarValue() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestValidateEnvVars tests validation of multiple environment variables
func TestValidateEnvVars(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "all valid",
			envVars: map[string]string{
				"DATABASE_URL": "postgres://localhost/mydb",
				"API_KEY":      "secret-key-123",
				"DEBUG":        "true",
			},
			wantErr: false,
		},
		{
			name: "invalid name",
			envVars: map[string]string{
				"VALID_VAR":   "value",
				"INVALID VAR": "value",
			},
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name: "reserved name",
			envVars: map[string]string{
				"CUSTOM_VAR": "value",
				"PATH":       "/custom/path",
			},
			wantErr:     true,
			errContains: "reserved",
		},
		{
			name: "invalid value",
			envVars: map[string]string{
				"VAR1": "good value",
				"VAR2": "bad\x00value",
			},
			wantErr:     true,
			errContains: "null byte",
		},
		{
			name:    "empty map",
			envVars: map[string]string{},
			wantErr: false,
		},
		{
			name:    "nil map",
			envVars: nil,
			wantErr: false,
		},
		{
			name: "too many variables",
			envVars: func() map[string]string {
				vars := make(map[string]string)
				for i := 0; i < 1001; i++ {
					vars[fmt.Sprintf("VAR_%d", i)] = "value"
				}
				return vars
			}(),
			wantErr:     true,
			errContains: "too many",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvVars(tt.envVars)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateEnvVars() expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateEnvVars() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateEnvVars() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestIsReservedEnvVar tests the reserved environment variable check
func TestIsReservedEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		reserved bool
	}{
		{"PATH", "PATH", true},
		{"HOME", "HOME", true},
		{"USER", "USER", true},
		{"SHELL", "SHELL", true},
		{"PWD", "PWD", true},
		{"OLDPWD", "OLDPWD", true},
		{"LD_PRELOAD", "LD_PRELOAD", true},
		{"LD_LIBRARY_PATH", "LD_LIBRARY_PATH", true},
		{"DYLD_INSERT_LIBRARIES", "DYLD_INSERT_LIBRARIES", true},
		{"DYLD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", true},
		{"IFS", "IFS", true},
		{"CDPATH", "CDPATH", true},
		{"ENV", "ENV", true},
		{"BASH_ENV", "BASH_ENV", true},
		
		// Not reserved
		{"CUSTOM_PATH", "CUSTOM_PATH", false},
		{"MY_HOME", "MY_HOME", false},
		{"DATABASE_URL", "DATABASE_URL", false},
		{"API_KEY", "API_KEY", false},
		
		// Case sensitive check
		{"path lowercase", "path", false},
		{"Path mixed", "Path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsReservedEnvVar(tt.varName)
			if result != tt.reserved {
				t.Errorf("IsReservedEnvVar(%q) = %v, want %v", tt.varName, result, tt.reserved)
			}
		})
	}
}

// BenchmarkValidateEnvVarName benchmarks name validation
func BenchmarkValidateEnvVarName(b *testing.B) {
	testCases := []struct {
		name    string
		varName string
	}{
		{"short", "VAR"},
		{"medium", "DATABASE_CONNECTION_URL"},
		{"long", strings.Repeat("A", 200)},
		{"with_numbers", "VAR_123_456_789"},
		{"mixed_case", "MyDatabaseUrl"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = ValidateEnvVarName(tc.varName)
			}
		})
	}
}

// BenchmarkValidateEnvVarValue benchmarks value validation
func BenchmarkValidateEnvVarValue(b *testing.B) {
	testCases := []struct {
		name  string
		value string
	}{
		{"short", "value"},
		{"medium", "postgres://user:pass@localhost:5432/database?sslmode=disable"},
		{"long", strings.Repeat("x", 10000)},
		{"json", `{"key": "value", "nested": {"data": [1, 2, 3]}}`},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = ValidateEnvVarValue(tc.value)
			}
		})
	}
}