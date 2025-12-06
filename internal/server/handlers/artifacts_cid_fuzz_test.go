package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// FuzzComputeArtifactCIDAndDigest validates determinism and format of CID/digest.
func FuzzComputeArtifactCIDAndDigest(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("a"))
	f.Add([]byte("some test bytes"))
	f.Fuzz(func(t *testing.T, b []byte) {
		cid, digest := computeArtifactCIDAndDigest(b)
		// digest must match sha256: + 64 hex
		sum := sha256.Sum256(b)
		hexSum := hex.EncodeToString(sum[:])
		wantDigest := "sha256:" + hexSum
		if digest != wantDigest {
			t.Fatalf("digest mismatch: got %q want %q", digest, wantDigest)
		}
		// CID is bafy + first 32 hex chars
		wantCID := "bafy" + hexSum[:32]
		if cid != wantCID {
			t.Fatalf("cid mismatch: got %q want %q", cid, wantCID)
		}
		if len(digest) != len("sha256:")+64 {
			t.Fatalf("unexpected digest length: %d", len(digest))
		}
		if len(cid) != len("bafy")+32 {
			t.Fatalf("unexpected cid length: %d", len(cid))
		}
	})
}
