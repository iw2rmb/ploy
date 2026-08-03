package speccompiler

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RepositorySource confines every file operation to one repository checkout.
type RepositorySource struct {
	path string
	root *os.Root
}

func NewRepositorySource(path string) (*RepositorySource, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("repository root is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root: %w", err)
	}
	root, err := os.OpenRoot(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("open repository root: %w", err)
	}
	return &RepositorySource{path: resolvedPath, root: root}, nil
}

func (s *RepositorySource) Close() error {
	return s.root.Close()
}

func (s *RepositorySource) ResolveSpec(path string) (string, error) {
	return s.resolve(path, "")
}

func (s *RepositorySource) ResolveBaseDir(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return s.path, nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository base directory: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve repository base directory: %w", err)
	}
	if _, err := s.relative(resolvedPath); err != nil {
		return "", err
	}
	return resolvedPath, nil
}

func (s *RepositorySource) ResolveReference(path, baseDir string) (string, error) {
	return s.resolve(path, baseDir)
}

func (s *RepositorySource) ResolveFileRecord(path, baseDir string) (string, error) {
	return s.resolve(path, baseDir)
}

func (s *RepositorySource) resolve(path, baseDir string) (string, error) {
	raw := strings.TrimSpace(path)
	if raw == "" {
		return "", fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(raw) {
		return "", fmt.Errorf("absolute path is not allowed")
	}
	if raw == "~" || strings.HasPrefix(raw, "~/") {
		return "", fmt.Errorf("home-relative path is not allowed")
	}
	if strings.Contains(raw, "$") {
		return "", fmt.Errorf("environment-expanded path is not allowed")
	}

	base := s.path
	if strings.TrimSpace(baseDir) != "" {
		base = baseDir
	}
	candidate := filepath.Clean(filepath.Join(base, raw))
	if _, err := s.relative(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func (s *RepositorySource) ReadFile(path string) ([]byte, error) {
	rel, err := s.relative(path)
	if err != nil {
		return nil, err
	}
	return s.root.ReadFile(rel)
}

func (s *RepositorySource) Stat(path string) (fs.FileInfo, error) {
	rel, err := s.relative(path)
	if err != nil {
		return nil, err
	}
	return s.root.Stat(rel)
}

func (s *RepositorySource) ReadDir(path string) ([]fs.DirEntry, error) {
	rel, err := s.relative(path)
	if err != nil {
		return nil, err
	}
	return fs.ReadDir(s.root.FS(), rel)
}

func (s *RepositorySource) WorkingDir() (string, error) {
	return s.path, nil
}

func (s *RepositorySource) relative(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository path: %w", err)
	}
	rel, err := filepath.Rel(s.path, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve repository-relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository root: %s", path)
	}
	return rel, nil
}
