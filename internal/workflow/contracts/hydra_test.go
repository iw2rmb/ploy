package contracts

import (
	"strings"
	"testing"
)

func TestParseStoredInEntry(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHash string
		wantDst  string
		wantErr  string
	}{
		{
			name:     "valid entry",
			input:    "abcdef0:/in/config.json",
			wantHash: "abcdef0",
			wantDst:  "/in/config.json",
		},
		{
			name:     "valid with nested path",
			input:    "1234567890abcdef:/in/subdir/file.txt",
			wantHash: "1234567890abcdef",
			wantDst:  "/in/subdir/file.txt",
		},
		{
			name:     "double slash cleaned",
			input:    "abcdef0:/in//tmp/pwn",
			wantHash: "abcdef0",
			wantDst:  "/in/tmp/pwn",
		},
		{
			name:    "wrong domain",
			input:   "abcdef0:/out/file",
			wantErr: "destination must start with /in/",
		},
		{
			name:    "invalid hash",
			input:   "XYZ:/in/file",
			wantErr: "invalid short hash",
		},
		{
			name:    "path traversal",
			input:   "abcdef0:/in/../etc/passwd",
			wantErr: "destination must start with /in/",
		},
		{
			name:    "no colon",
			input:   "abcdef0",
			wantErr: "expected format shortHash:dst",
		},
		{
			name:    "hash too short",
			input:   "abc:/in/x",
			wantErr: "invalid short hash",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseStoredInEntry(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Hash != tc.wantHash {
				t.Errorf("hash = %q, want %q", parsed.Hash, tc.wantHash)
			}
			if parsed.Dst != tc.wantDst {
				t.Errorf("dst = %q, want %q", parsed.Dst, tc.wantDst)
			}
			if !parsed.ReadOnly {
				t.Errorf("in entries must be read-only")
			}
		})
	}
}

func TestParseStoredOutEntry(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHash string
		wantDst  string
		wantErr  string
	}{
		{
			name:     "valid entry",
			input:    "abcdef0:/out/results",
			wantHash: "abcdef0",
			wantDst:  "/out/results",
		},
		{
			name:     "double slash cleaned",
			input:    "abcdef0:/out//tmp/pwn",
			wantHash: "abcdef0",
			wantDst:  "/out/tmp/pwn",
		},
		{
			name:    "double slash escapes domain",
			input:   "abcdef0:/out/../../etc/shadow",
			wantErr: "destination must start with /out/",
		},
		{
			name:    "wrong domain",
			input:   "abcdef0:/in/file",
			wantErr: "destination must start with /out/",
		},
		{
			name:    "path traversal",
			input:   "abcdef0:/out/../../etc/passwd",
			wantErr: "destination must start with /out/",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseStoredOutEntry(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Hash != tc.wantHash {
				t.Errorf("hash = %q, want %q", parsed.Hash, tc.wantHash)
			}
			if parsed.Dst != tc.wantDst {
				t.Errorf("dst = %q, want %q", parsed.Dst, tc.wantDst)
			}
			if parsed.ReadOnly {
				t.Errorf("out entries must be read-write")
			}
		})
	}
}

func TestParseStoredHomeEntry(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHash string
		wantDst  string
		wantRO   bool
		wantErr  string
	}{
		{
			name:     "rw entry",
			input:    "abcdef0:.codex/auth.json",
			wantHash: "abcdef0",
			wantDst:  ".codex/auth.json",
			wantRO:   false,
		},
		{
			name:     "ro entry",
			input:    "abcdef0:.codex/config.toml:ro",
			wantHash: "abcdef0",
			wantDst:  ".codex/config.toml",
			wantRO:   true,
		},
		{
			name:     "double slash cleaned",
			input:    "abcdef0:.config//app",
			wantHash: "abcdef0",
			wantDst:  ".config/app",
			wantRO:   false,
		},
		{
			name:    "absolute path rejected",
			input:   "abcdef0:/etc/config",
			wantErr: "destination must be relative",
		},
		{
			name:    "traversal rejected",
			input:   "abcdef0:../../etc/passwd",
			wantErr: "path traversal not allowed",
		},
		{
			name:    "empty destination",
			input:   "abcdef0:",
			wantErr: "destination required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseStoredHomeEntry(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parsed.Hash != tc.wantHash {
				t.Errorf("hash = %q, want %q", parsed.Hash, tc.wantHash)
			}
			if parsed.Dst != tc.wantDst {
				t.Errorf("dst = %q, want %q", parsed.Dst, tc.wantDst)
			}
			if parsed.ReadOnly != tc.wantRO {
				t.Errorf("readOnly = %v, want %v", parsed.ReadOnly, tc.wantRO)
			}
		})
	}
}

func TestParseStoredCAEntry(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{
			name:  "valid hash",
			input: "abcdef0123456",
			want:  "abcdef0123456",
		},
		{
			name:  "valid 7-char hash",
			input: "abcdef0",
			want:  "abcdef0",
		},
		{
			name:    "invalid chars",
			input:   "NOT_HEX",
			wantErr: "must be a valid short hash",
		},
		{
			name:    "too short",
			input:   "abc",
			wantErr: "must be a valid short hash",
		},
		{
			name:    "empty",
			input:   "",
			wantErr: "must be a valid short hash",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStoredCAEntry(tc.input)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateHydraInEntries_DuplicateDst(t *testing.T) {
	err := ValidateHydraInEntries([]string{
		"abcdef0:/in/a",
		"bbbbbbb:/in/a",
	}, "test")
	if err == nil {
		t.Fatal("expected duplicate destination error")
	}
	if !strings.Contains(err.Error(), "duplicate destination") {
		t.Fatalf("error %q does not mention duplicate destination", err.Error())
	}
}

func TestValidateHydraHomeEntries_DuplicateDst(t *testing.T) {
	err := ValidateHydraHomeEntries([]string{
		"abcdef0:.config/a",
		"bbbbbbb:.config/a:ro",
	}, "test")
	if err == nil {
		t.Fatal("expected duplicate destination error")
	}
	if !strings.Contains(err.Error(), "duplicate destination") {
		t.Fatalf("error %q does not mention duplicate destination", err.Error())
	}
}

func TestValidateHydraHomeEntries_DuplicateEquivalentPath(t *testing.T) {
	err := ValidateHydraHomeEntries([]string{
		"abcdef0:.config//app",
		"bbbbbbb:.config/app",
	}, "test")
	if err == nil {
		t.Fatal("expected duplicate destination error for equivalent paths")
	}
	if !strings.Contains(err.Error(), "duplicate destination") {
		t.Fatalf("error %q does not mention duplicate destination", err.Error())
	}
}

func TestValidateHydraSection(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"pre_gate", "re_gate", "post_gate", "mig", "heal"} {
		if err := ValidateHydraSection(s); err != nil {
			t.Errorf("ValidateHydraSection(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range []string{"", "unknown", "mr", "server", "node"} {
		if err := ValidateHydraSection(s); err == nil {
			t.Errorf("ValidateHydraSection(%q) = nil, want error", s)
		}
	}
}

func TestValidateCAConfigSection(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"pre_gate", "re_gate", "post_gate", "mig", "heal"} {
		if err := ValidateCAConfigSection(s); err != nil {
			t.Errorf("ValidateCAConfigSection(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range []string{"", "unknown", "mr", "server", "node"} {
		if err := ValidateCAConfigSection(s); err == nil {
			t.Errorf("ValidateCAConfigSection(%q) = nil, want error", s)
		}
	}
}

func TestValidateHydraCAEntries_DuplicateHash(t *testing.T) {
	err := ValidateHydraCAEntries([]string{"abcdef0", "abcdef0"}, "test")
	if err == nil {
		t.Fatal("expected duplicate hash error")
	}
	if !strings.Contains(err.Error(), "duplicate hash") {
		t.Fatalf("error %q does not mention duplicate hash", err.Error())
	}
}
