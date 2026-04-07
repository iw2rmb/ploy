package nodeagent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

func preserveFailureArtifacts(runID types.RunID, jobID types.JobID, workspace, outDir, inDir string) (string, error) {
	root := filepath.Join(os.TempDir(), "ploy-preserved", runID.String(), jobID.String(), time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create preserve root: %w", err)
	}

	copyOne := func(src, name string) error {
		src = strings.TrimSpace(src)
		if src == "" {
			return nil
		}
		info, err := os.Stat(src)
		if err != nil {
			return nil
		}

		dst := filepath.Join(root, name)
		if info.IsDir() {
			return copyDirRsync(src, dst)
		}
		return copyFileBytes(src, dst)
	}

	if err := copyOne(inDir, "in"); err != nil {
		return root, fmt.Errorf("copy in dir: %w", err)
	}
	if err := copyOne(outDir, "out"); err != nil {
		return root, fmt.Errorf("copy out dir: %w", err)
	}
	if err := copyOne(workspace, "workspace"); err != nil {
		return root, fmt.Errorf("copy workspace: %w", err)
	}

	return root, nil
}

func copyDirRsync(srcDir, dstDir string) error {
	if _, err := exec.LookPath("rsync"); err != nil {
		return fmt.Errorf("rsync unavailable: %w", err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	cmd := exec.Command("rsync", "-a", srcDir+"/", dstDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync failed: %w (output: %s)", err, string(output))
	}
	return nil
}

func copyFileBytes(srcFile, dstFile string) error {
	data, err := os.ReadFile(srcFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dstFile, data, 0o644)
}
