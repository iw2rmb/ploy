package transflow

import (
    "archive/tar"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
)

// hasRepoChanges returns true if the working tree has any changes
func hasRepoChanges(repoPath string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// getHeadHash returns the current HEAD commit hash
func getHeadHash(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// createTarFromDir creates a tar archive of a directory using Go's archive/tar
func createTarFromDir(srcDir, dstTar string) error {
    // Remove existing tar if any
    _ = os.Remove(dstTar)

    f, err := os.Create(dstTar)
    if err != nil {
        return fmt.Errorf("create tar: %w", err)
    }
    defer func() {
        _ = f.Close()
    }()

    tw := tar.NewWriter(f)
    defer func() { _ = tw.Close() }()

    // Add an explicit root entry like GNU tar shows ("./") for better parity with previews
    if err := tw.WriteHeader(&tar.Header{
        Name:     "./",
        Mode:     0755,
        Typeflag: tar.TypeDir,
    }); err != nil {
        return fmt.Errorf("tar header root: %w", err)
    }

    walkFn := func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return err
        }
        rel, err := filepath.Rel(srcDir, path)
        if err != nil {
            return err
        }
        if rel == "." {
            return nil // already wrote root
        }
        name := "./" + filepath.ToSlash(rel)

        info, err := d.Info()
        if err != nil {
            return err
        }
        hdr, err := tar.FileInfoHeader(info, "")
        if err != nil {
            return err
        }
        hdr.Name = name

        if err := tw.WriteHeader(hdr); err != nil {
            return err
        }

        if info.Mode().IsRegular() {
            rf, err := os.Open(path)
            if err != nil {
                return err
            }
            if _, err := io.Copy(tw, rf); err != nil {
                _ = rf.Close()
                return err
            }
            _ = rf.Close()
        }
        return nil
    }

    if err := filepath.WalkDir(srcDir, walkFn); err != nil {
        return fmt.Errorf("walk dir: %w", err)
    }

    // Ensure tar not empty
    if st, err := f.Stat(); err == nil {
        if st.Size() == 0 {
            return fmt.Errorf("created empty tar archive")
        }
    }
    return nil
}

// test indirection for getHeadHash
var getHeadHashFn = getHeadHash
