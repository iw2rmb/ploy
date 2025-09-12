package transflow

import (
	"log"
	"os/exec"
	"strings"
)

// previewTarEntries lists up to max entries from a tar archive.
// Best-effort: uses `tar -tf` and returns the first max lines.
func previewTarEntries(tarPath string, max int) ([]string, error) {
	if max <= 0 {
		max = 1
	}
	cmd := exec.Command("tar", "-tf", tarPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Return empty list with error; caller may ignore
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if max > len(lines) {
		max = len(lines)
	}
	return lines[:max], nil
}

// logPreviewTar logs a short preview of a tar archive's contents.
func logPreviewTar(tarPath string, max int) {
	entries, err := previewTarEntries(tarPath, max)
	if err != nil {
		log.Printf("[Transflow] input.tar preview failed: %v", err)
		return
	}
	log.Printf("[Transflow] input.tar preview (%d entries):\n%s", len(entries), strings.Join(entries, "\n"))
}
