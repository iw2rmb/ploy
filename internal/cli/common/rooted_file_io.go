package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func splitRootedPath(path string) (string, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return "", "", fmt.Errorf("path must not be empty")
	}
	dir := filepath.Dir(clean)
	name := filepath.Base(clean)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "", "", fmt.Errorf("path %q does not reference a file", path)
	}
	return dir, name, nil
}

func ReadFileRooted(path string) ([]byte, error) {
	dir, name, err := splitRootedPath(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func statFileRooted(path string) (os.FileInfo, error) {
	dir, name, err := splitRootedPath(path)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.Stat(name)
}

func openFileRooted(path string) (*os.File, *os.Root, error) {
	dir, name, err := splitRootedPath(path)
	if err != nil {
		return nil, nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	f, err := root.Open(name)
	if err != nil {
		_ = root.Close()
		return nil, nil, err
	}
	return f, root, nil
}
