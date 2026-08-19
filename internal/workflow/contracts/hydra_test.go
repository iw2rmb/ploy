package contracts

import (
	"slices"
	"strings"
	"testing"
)

func TestHydraFileKindsUseContractOrder(t *testing.T) {
	t.Parallel()

	want := []HydraFileKind{HydraFileIn, HydraFileOut, HydraFileHome, HydraFileTmp}
	if got := HydraFileKinds(); !slices.Equal(got, want) {
		t.Fatalf("HydraFileKinds() = %v, want %v", got, want)
	}
	manifest := StepManifest{
		In: []string{"in"}, Out: []string{"out"}, Home: []string{"home"}, Tmp: []string{"tmp"},
	}
	for _, kind := range want {
		if got := kind.Entries(manifest); len(got) != 1 || got[0] != kind.String() {
			t.Fatalf("%s entries = %v, want [%s]", kind, got, kind)
		}
	}
}

func TestIsHydraShortHash(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "minimum length", value: "abcdef0", want: true},
		{name: "maximum length", value: strings.Repeat("a", 64), want: true},
		{name: "too short", value: "abcdef"},
		{name: "too long", value: strings.Repeat("a", 65)},
		{name: "uppercase", value: "ABCDEF0"},
		{name: "invalid hexadecimal", value: "abcdefg"},
		{name: "whitespace", value: " abcdef0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHydraShortHash(tt.value); got != tt.want {
				t.Fatalf("IsHydraShortHash(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseStoredEntrySupportsAllHydraFileKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind     HydraFileKind
		entry    string
		wantDst  string
		wantRead bool
	}{
		{kind: HydraFileIn, entry: "abcdef0:/in/input", wantDst: "/in/input", wantRead: true},
		{kind: HydraFileOut, entry: "abcdef0:/out/output", wantDst: "/out/output"},
		{kind: HydraFileHome, entry: "abcdef0:.config/app:ro", wantDst: ".config/app", wantRead: true},
		{kind: HydraFileTmp, entry: "abcdef0:/tmp/cache", wantDst: "/tmp/cache"},
	}
	for _, tc := range tests {
		t.Run(tc.kind.String(), func(t *testing.T) {
			got, err := ParseStoredEntry(tc.kind, tc.entry)
			if err != nil {
				t.Fatalf("ParseStoredEntry() error = %v", err)
			}
			if got.Dst != tc.wantDst || got.ReadOnly != tc.wantRead {
				t.Fatalf("ParseStoredEntry() = %+v, want dst=%q readOnly=%v", got, tc.wantDst, tc.wantRead)
			}
		})
	}
}

func TestParseStoredEntry(t *testing.T) {
	tests := []struct {
		name     string
		kind     HydraFileKind
		input    string
		wantHash string
		wantDst  string
		wantRO   bool
		wantErr  string
	}{
		{
			name:     "in/valid entry",
			kind:     HydraFileIn,
			input:    "abcdef0:/in/config.json",
			wantHash: "abcdef0",
			wantDst:  "/in/config.json",
			wantRO:   true,
		},
		{
			name:     "in/valid with nested path",
			kind:     HydraFileIn,
			input:    "1234567890abcdef:/in/subdir/file.txt",
			wantHash: "1234567890abcdef",
			wantDst:  "/in/subdir/file.txt",
			wantRO:   true,
		},
		{
			name:     "in/double slash cleaned",
			kind:     HydraFileIn,
			input:    "abcdef0:/in//tmp/pwn",
			wantHash: "abcdef0",
			wantDst:  "/in/tmp/pwn",
			wantRO:   true,
		},
		{
			name:    "in/wrong domain",
			kind:    HydraFileIn,
			input:   "abcdef0:/out/file",
			wantErr: "destination must start with /in/",
		},
		{
			name:    "in/invalid hash",
			kind:    HydraFileIn,
			input:   "XYZ:/in/file",
			wantErr: "invalid short hash",
		},
		{
			name:    "in/path traversal",
			kind:    HydraFileIn,
			input:   "abcdef0:/in/../etc/passwd",
			wantErr: "destination must start with /in/",
		},
		{
			name:    "in/no colon",
			kind:    HydraFileIn,
			input:   "abcdef0",
			wantErr: "expected format shortHash:dst",
		},
		{
			name:    "in/hash too short",
			kind:    HydraFileIn,
			input:   "abc:/in/x",
			wantErr: "invalid short hash",
		},
		{
			name:     "out/valid entry",
			kind:     HydraFileOut,
			input:    "abcdef0:/out/results",
			wantHash: "abcdef0",
			wantDst:  "/out/results",
		},
		{
			name:     "out/double slash cleaned",
			kind:     HydraFileOut,
			input:    "abcdef0:/out//tmp/pwn",
			wantHash: "abcdef0",
			wantDst:  "/out/tmp/pwn",
		},
		{
			name:    "out/double slash escapes domain",
			kind:    HydraFileOut,
			input:   "abcdef0:/out/../../etc/shadow",
			wantErr: "destination must start with /out/",
		},
		{
			name:    "out/wrong domain",
			kind:    HydraFileOut,
			input:   "abcdef0:/in/file",
			wantErr: "destination must start with /out/",
		},
		{
			name:    "out/path traversal",
			kind:    HydraFileOut,
			input:   "abcdef0:/out/../../etc/passwd",
			wantErr: "destination must start with /out/",
		},
		{
			name:     "home/rw entry",
			kind:     HydraFileHome,
			input:    "abcdef0:.codex/auth.json",
			wantHash: "abcdef0",
			wantDst:  ".codex/auth.json",
		},
		{
			name:     "home/ro entry",
			kind:     HydraFileHome,
			input:    "abcdef0:.codex/config.toml:ro",
			wantHash: "abcdef0",
			wantDst:  ".codex/config.toml",
			wantRO:   true,
		},
		{
			name:     "home/double slash cleaned",
			kind:     HydraFileHome,
			input:    "abcdef0:.config//app",
			wantHash: "abcdef0",
			wantDst:  ".config/app",
		},
		{
			name:    "home/absolute path rejected",
			kind:    HydraFileHome,
			input:   "abcdef0:/etc/config",
			wantErr: "destination must be relative",
		},
		{
			name:    "home/traversal rejected",
			kind:    HydraFileHome,
			input:   "abcdef0:../../etc/passwd",
			wantErr: "path traversal not allowed",
		},
		{
			name:    "home/empty destination",
			kind:    HydraFileHome,
			input:   "abcdef0:",
			wantErr: "destination required",
		},
		{
			name:     "tmp/valid entry",
			kind:     HydraFileTmp,
			input:    "abcdef0:/tmp/ploy/lib.jar",
			wantHash: "abcdef0",
			wantDst:  "/tmp/ploy/lib.jar",
		},
		{
			name:     "tmp/double slash cleaned",
			kind:     HydraFileTmp,
			input:    "abcdef0:/tmp//ploy/tool",
			wantHash: "abcdef0",
			wantDst:  "/tmp/ploy/tool",
		},
		{
			name:    "tmp/outside tmp rejected",
			kind:    HydraFileTmp,
			input:   "abcdef0:/var/tmp/tool",
			wantErr: "destination must start with /tmp/",
		},
		{
			name:    "tmp/traversal rejected",
			kind:    HydraFileTmp,
			input:   "abcdef0:/tmp/../../etc/passwd",
			wantErr: "destination must start with /tmp/",
		},
		{
			name:    "tmp/empty destination",
			kind:    HydraFileTmp,
			input:   "abcdef0:",
			wantErr: "destination required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseStoredEntry(tc.kind, tc.input)
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

func TestValidateHydraEntriesRejectsDuplicateDestinations(t *testing.T) {
	tests := []struct {
		name    string
		kind    HydraFileKind
		entries []string
	}{
		{name: "in", kind: HydraFileIn, entries: []string{"abcdef0:/in/a", "bbbbbbb:/in/a"}},
		{name: "out", kind: HydraFileOut, entries: []string{"abcdef0:/out/a", "bbbbbbb:/out/a"}},
		{name: "tmp", kind: HydraFileTmp, entries: []string{"abcdef0:/tmp/ploy/tool.jar", "bbbbbbb:/tmp/ploy/tool.jar"}},
		{name: "home modes", kind: HydraFileHome, entries: []string{"abcdef0:.config/a", "bbbbbbb:.config/a:ro"}},
		{name: "equivalent home paths", kind: HydraFileHome, entries: []string{"abcdef0:.config//app", "bbbbbbb:.config/app"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHydraEntries(tt.kind, tt.entries, "test")
			if err == nil || !strings.Contains(err.Error(), "duplicate destination") {
				t.Fatalf("ValidateHydraEntries() error = %v, want duplicate destination", err)
			}
		})
	}
}

func TestValidateHydraSection(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"pre_gate", "post_gate", "mig"} {
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
