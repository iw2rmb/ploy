package mods

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// NormalizePatch normalizes a patch and returns normalized content and fingerprint
func (sg *DefaultSignatureGenerator) NormalizePatch(patch []byte) ([]byte, string) {
	patchText := string(patch)
	lines := strings.Split(patchText, "\n")
	var normalizedLines []string
	for _, line := range lines {
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			if strings.HasPrefix(line, "---") {
				normalizedLines = append(normalizedLines, "--- [FILE_A]")
			} else {
				normalizedLines = append(normalizedLines, "+++ [FILE_B]")
			}
			continue
		}
		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "diff --git") {
			continue
		}
		normalizedLines = append(normalizedLines, line)
	}
	normalizedPatch := strings.Join(normalizedLines, "\n")
	normalizedPatch = strings.TrimSpace(normalizedPatch)
	lines = strings.Split(normalizedPatch, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			lines[i] = strings.TrimRight(line, " \t")
		}
	}
	normalizedPatch = strings.Join(lines, "\n")
	fingerprint := sg.generatePatchFingerprint([]byte(normalizedPatch))
	return []byte(normalizedPatch), fingerprint
}

func (sg *DefaultSignatureGenerator) generatePatchFingerprint(normalizedPatch []byte) string {
	hash := sha256.Sum256(normalizedPatch)
	return fmt.Sprintf("%x", hash)
}
