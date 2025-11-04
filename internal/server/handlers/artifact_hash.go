package handlers

import (
	"crypto/sha256"
	"encoding/hex"
)

// computeArtifactCIDAndDigest computes a content identifier and SHA256 digest for an artifact bundle.
// CID uses a simple "bafy" prefix with hex-encoded SHA256 for compatibility with existing test fixtures.
// Digest is the full SHA256 hex string with "sha256:" prefix.
func computeArtifactCIDAndDigest(bundle []byte) (cid, digest string) {
	hash := sha256.Sum256(bundle)
	hexHash := hex.EncodeToString(hash[:])
	// Use bafy prefix (like IPFS CIDv1) followed by first 32 chars of hash for readability
	cid = "bafy" + hexHash[:32]
	digest = "sha256:" + hexHash
	return cid, digest
}
