package mods

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
)

// randomStepID returns s-<12 hex chars>
func randomStepID() string {
	var buf [6]byte
	_, _ = crand.Read(buf[:])
	return "s-" + hex.EncodeToString(buf[:])
}

// uploadInputTar uploads input.tar to artifacts/mods/<modID>/input.tar (best-effort)
func uploadInputTar(seaweedBase, modID, inputTarPath string) error {
	key := fmt.Sprintf("mods/%s/input.tar", modID)
	return putFileFn(seaweedBase, key, inputTarPath, "application/octet-stream")
}
