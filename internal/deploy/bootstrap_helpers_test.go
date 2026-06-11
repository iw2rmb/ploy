package deploy

import (
	"strings"
	"testing"
)

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
				t.Fatalf("RandomHexString() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != tt.length {
				t.Fatalf("RandomHexString() length = %d, want %d", len(got), tt.length)
			}
			for _, c := range got {
				if !strings.ContainsRune("0123456789abcdef", c) {
					t.Fatalf("RandomHexString() contains non-hex char %q in %q", c, got)
				}
			}
		})
	}
}
