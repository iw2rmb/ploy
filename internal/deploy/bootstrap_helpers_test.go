package deploy

import (
	"regexp"
	"strings"
	"testing"
)

var clusterIDPattern = regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9]{4}$`)

func TestGenerateClusterID(t *testing.T) {
	t.Run("generates valid cluster ID", func(t *testing.T) {
		id, err := GenerateClusterID()
		if err != nil {
			t.Fatalf("GenerateClusterID() error = %v, want nil", err)
		}

		if strings.TrimSpace(id) == "" {
			t.Fatalf("GenerateClusterID() = %q, want non-empty", id)
		}

		if !clusterIDPattern.MatchString(id) {
			t.Errorf("GenerateClusterID() = %q, want haikunator pattern adjective-noun-4digit token", id)
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

// nanoIDAlphabet is the URL-safe alphabet used by NewNodeKey().
const nanoIDAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_-"

func TestGenerateNodeID(t *testing.T) {
	t.Run("generates valid node ID", func(t *testing.T) {
		id := GenerateNodeID()

		if len(id) != 6 {
			t.Errorf("GenerateNodeID() length = %d, want 6", len(id))
		}

		for _, c := range id {
			if !strings.ContainsRune(nanoIDAlphabet, c) {
				t.Errorf("invalid character %q in node ID %q; expected URL-safe alphabet", c, id)
			}
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		id1 := GenerateNodeID()
		id2 := GenerateNodeID()

		if id1 == id2 {
			t.Errorf("GenerateNodeID() generated duplicate IDs: %q", id1)
		}
	})
}

func TestRandomHexString(t *testing.T) {
	tests := []struct {
		name    string
		length  int
		wantErr bool
	}{
		{name: "valid length 8", length: 8},
		{name: "valid length 16", length: 16},
		{name: "valid length 32", length: 32},
		{name: "zero length", length: 0, wantErr: true},
		{name: "negative length", length: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RandomHexString(tt.length)
			if (err != nil) != tt.wantErr {
				t.Errorf("RandomHexString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(got) != tt.length {
				t.Errorf("RandomHexString(%d) length = %d, want %d", tt.length, len(got), tt.length)
			}

			for _, c := range got {
				if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
					t.Errorf("invalid hex character %q in result %q", c, got)
				}
			}
		})
	}
}
