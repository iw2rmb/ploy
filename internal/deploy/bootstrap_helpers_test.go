package deploy

import (
	"strings"
	"testing"
)

func TestGenerateClusterID(t *testing.T) {
	t.Run("generates valid cluster ID", func(t *testing.T) {
		id, err := GenerateClusterID()
		if err != nil {
			t.Fatalf("GenerateClusterID() error = %v, want nil", err)
		}

		if !strings.HasPrefix(id, "cluster-") {
			t.Errorf("GenerateClusterID() = %q, want prefix 'cluster-'", id)
		}

		// Expect format: cluster-<16 hex chars>
		// Total length should be 8 ("cluster-") + 16 = 24
		if len(id) != 24 {
			t.Errorf("GenerateClusterID() length = %d, want 24", len(id))
		}

		// Extract hex part and validate it's hex
		hexPart := strings.TrimPrefix(id, "cluster-")
		if len(hexPart) != 16 {
			t.Errorf("hex part length = %d, want 16", len(hexPart))
		}

		for _, c := range hexPart {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("invalid hex character %q in cluster ID %q", c, id)
			}
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		id1, err := GenerateClusterID()
		if err != nil {
			t.Fatalf("GenerateClusterID() error = %v", err)
		}

		id2, err := GenerateClusterID()
		if err != nil {
			t.Fatalf("GenerateClusterID() error = %v", err)
		}

		if id1 == id2 {
			t.Errorf("GenerateClusterID() generated duplicate IDs: %q", id1)
		}
	})
}

func TestRandomHexString(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{
			name:    "valid length 8",
			length:  8,
			wantErr: false,
		},
		{
			name:    "valid length 16",
			length:  16,
			wantErr: false,
		},
		{
			name:    "valid length 32",
			length:  32,
			wantErr: false,
		},
		{
			name:    "zero length",
			length:  0,
			wantErr: true,
		},
		{
			name:    "negative length",
			length:  -1,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := randomHexString(tt.length)
			if (err != nil) != tt.wantErr {
				t.Errorf("randomHexString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(got) != tt.length {
				t.Errorf("randomHexString(%d) length = %d, want %d", tt.length, len(got), tt.length)
			}

			// Validate all characters are hex
			for _, c := range got {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("invalid hex character %q in result %q", c, got)
				}
			}
		})
	}
}
