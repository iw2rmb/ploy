package arf

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// createTarFromDirectory creates a tar archive from a directory
func (d *DeploymentSandboxManager) createTarFromDirectory(sourceDir string) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()

	// Walk through the directory and add files to tar
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if strings.Contains(path, ".git") {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Update the name to be relative to sourceDir
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		// Preserve executable permissions for scripts
		if !info.IsDir() && (strings.HasSuffix(path, "gradlew") ||
			strings.HasSuffix(path, "mvnw") ||
			strings.HasSuffix(path, ".sh")) {
			header.Mode = 0755
		}

		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tw, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create tar archive: %w", err)
	}

	return buf.Bytes(), nil
}
