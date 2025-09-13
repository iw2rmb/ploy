package mods

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// previewTarEntries lists up to max entries from a tar(.gz) archive using pure Go.
func previewTarEntries(tarPath string, max int) ([]string, error) {
	if max <= 0 {
		max = 1
	}
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var r io.Reader = f
	// Allow .tar.gz preview best-effort
	if strings.HasSuffix(strings.ToLower(tarPath), ".gz") {
		if gz, gErr := gzip.NewReader(f); gErr == nil {
			defer func() { _ = gz.Close() }()
			r = gz
		}
	}
	tr := tar.NewReader(r)
	entries := make([]string, 0, max)
	for len(entries) < max {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, hdr.Name)
	}
	return entries, nil
}

// logPreviewTar logs a short preview of a tar archive's contents.
func logPreviewTar(tarPath string, max int) {
	entries, err := previewTarEntries(tarPath, max)
	if err != nil {
		log.Printf("[Mods] input.tar preview failed: %v", err)
		return
	}
	log.Printf("[Mods] input.tar preview (%d entries):\n%s", len(entries), strings.Join(entries, "\n"))
}

// logPreviewTarWithReporter emits tar preview via EventReporter when available (fallback to log)
func logPreviewTarWithReporter(rep EventReporter, phase, step, tarPath string, max int) {
	entries, err := previewTarEntries(tarPath, max)
	if err != nil {
		if rep != nil {
			_ = rep.Report(context.Background(), Event{Phase: phase, Step: step, Level: "warn", Message: "input.tar preview failed: " + err.Error(), Time: time.Now()})
		} else {
			log.Printf("[Mods] input.tar preview failed: %v", err)
		}
		return
	}
	msg := strings.Join(entries, "\n")
	if rep != nil {
		_ = rep.Report(context.Background(), Event{Phase: phase, Step: step, Level: "info", Message: "input.tar preview:\n" + msg, Time: time.Now()})
	} else {
		log.Printf("[Mods] input.tar preview (%d entries):\n%s", len(entries), msg)
	}
}
