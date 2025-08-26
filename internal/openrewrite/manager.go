package openrewrite

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitManagerImpl implements the GitManager interface
type GitManagerImpl struct {
	config *Config
}

// NewGitManager creates a new GitManager instance
func NewGitManager(config *Config) GitManager {
	return &GitManagerImpl{
		config: config,
	}
}

// InitializeRepo creates a Git repository from a tar archive
func (g *GitManagerImpl) InitializeRepo(ctx context.Context, jobID string, tarData []byte) (string, error) {
	// Validate tar data
	if len(tarData) == 0 {
		return "", fmt.Errorf("invalid tar archive: empty data")
	}
	
	// Create repository directory
	repoPath := filepath.Join(g.config.WorkDir, jobID)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create repository directory: %w", err)
	}
	
	// Extract tar archive
	if err := g.extractTar(repoPath, tarData); err != nil {
		_ = os.RemoveAll(repoPath)
		return "", fmt.Errorf("tar extraction failed: %w", err)
	}
	
	// Initialize git repository
	if err := g.runGitCommand(ctx, repoPath, "init"); err != nil {
		_ = os.RemoveAll(repoPath)
		return "", fmt.Errorf("git init failed: %w", err)
	}
	
	// Configure git user
	commands := [][]string{
		{"config", "user.email", "openrewrite@ploy.local"},
		{"config", "user.name", "OpenRewrite Service"},
	}
	
	for _, cmd := range commands {
		if err := g.runGitCommand(ctx, repoPath, cmd...); err != nil {
			_ = os.RemoveAll(repoPath)
			return "", fmt.Errorf("git config failed: %w", err)
		}
	}
	
	// Add all files
	if err := g.runGitCommand(ctx, repoPath, "add", "."); err != nil {
		_ = os.RemoveAll(repoPath)
		return "", fmt.Errorf("git add failed: %w", err)
	}
	
	// Create initial commit
	if err := g.runGitCommand(ctx, repoPath, "commit", "-m", "Initial state before transformation"); err != nil {
		// Check if there are files to commit
		status, _ := g.runGitCommandOutput(ctx, repoPath, "status", "--porcelain")
		if len(status) == 0 {
			// No files to commit, create an empty commit
			if err := g.runGitCommand(ctx, repoPath, "commit", "--allow-empty", "-m", "Initial state before transformation"); err != nil {
				_ = os.RemoveAll(repoPath)
				return "", fmt.Errorf("git commit failed: %w", err)
			}
		} else {
			_ = os.RemoveAll(repoPath)
			return "", fmt.Errorf("git commit failed: %w", err)
		}
	}
	
	// Tag the initial state
	if err := g.runGitCommand(ctx, repoPath, "tag", "before-transform"); err != nil {
		_ = os.RemoveAll(repoPath)
		return "", fmt.Errorf("git tag failed: %w", err)
	}
	
	return repoPath, nil
}

// GenerateDiff creates a unified diff after transformation
func (g *GitManagerImpl) GenerateDiff(ctx context.Context, repoPath string) ([]byte, error) {
	// Check for any changes (staged or unstaged)
	statusOutput, err := g.runGitCommandOutput(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to check git status: %w", err)
	}
	
	// If there are changes, stage them
	if len(statusOutput) > 0 {
		// Add all changes
		if err := g.runGitCommand(ctx, repoPath, "add", "."); err != nil {
			return nil, fmt.Errorf("failed to stage changes: %w", err)
		}
		
		// Commit the changes
		if err := g.runGitCommand(ctx, repoPath, "commit", "-m", "After OpenRewrite transformation"); err != nil {
			// Check if the error is due to no changes
			if !strings.Contains(err.Error(), "nothing to commit") {
				return nil, fmt.Errorf("failed to commit changes: %w", err)
			}
			// No actual changes, return empty diff
			return []byte{}, nil
		}
	} else {
		// No changes at all
		return []byte{}, nil
	}
	
	// Generate diff between the before-transform tag and HEAD
	diff, err := g.runGitCommandOutput(ctx, repoPath, "diff", "before-transform", "HEAD", "--unified=3")
	if err != nil {
		return nil, fmt.Errorf("failed to generate diff: %w", err)
	}
	
	return diff, nil
}

// Cleanup removes the temporary repository directory
func (g *GitManagerImpl) Cleanup(repoPath string) error {
	// Check if path exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil // Nothing to cleanup
	}
	
	// Remove the directory and all its contents
	if err := os.RemoveAll(repoPath); err != nil {
		// Log the error but don't fail
		fmt.Printf("Warning: failed to cleanup repository %s: %v\n", repoPath, err)
	}
	
	return nil
}

// extractTar extracts a tar.gz archive to the specified directory
func (g *GitManagerImpl) extractTar(destPath string, tarData []byte) error {
	// Create a gzip reader
	gr, err := gzip.NewReader(bytes.NewReader(tarData))
	if err != nil {
		// Try without gzip (plain tar)
		return g.extractPlainTar(destPath, tarData)
	}
	defer gr.Close()
	
	// Create tar reader
	tr := tar.NewReader(gr)
	
	return g.extractTarReader(tr, destPath)
}

// extractPlainTar extracts a plain tar archive (no gzip)
func (g *GitManagerImpl) extractPlainTar(destPath string, tarData []byte) error {
	tr := tar.NewReader(bytes.NewReader(tarData))
	return g.extractTarReader(tr, destPath)
}

// extractTarReader extracts files from a tar reader
func (g *GitManagerImpl) extractTarReader(tr *tar.Reader, destPath string) error {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}
		
		// Construct the file path
		target := filepath.Join(destPath, header.Name)
		
		// Ensure the path is still within destPath (security check)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destPath)) {
			return fmt.Errorf("tar entry %s attempts to write outside destination", header.Name)
		}
		
		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", target, err)
			}
			
		case tar.TypeReg:
			// Create directory for file if needed
			dir := filepath.Dir(target)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory for file %s: %w", target, err)
			}
			
			// Create file
			file, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", target, err)
			}
			
			// Copy file content
			if _, err := io.Copy(file, tr); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file %s: %w", target, err)
			}
			
			file.Close()
			
		case tar.TypeSymlink:
			// Create symlink
			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", target, err)
			}
		}
	}
	
	return nil
}

// runGitCommand executes a git command in the specified directory
func (g *GitManagerImpl) runGitCommand(ctx context.Context, repoPath string, args ...string) error {
	cmd := exec.CommandContext(ctx, g.config.GitPath, args...)
	cmd.Dir = repoPath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("git %s failed: %w\nOutput: %s", strings.Join(args, " "), err, string(output))
	}
	
	return nil
}

// runGitCommandOutput executes a git command and returns the output
func (g *GitManagerImpl) runGitCommandOutput(ctx context.Context, repoPath string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, g.config.GitPath, args...)
	cmd.Dir = repoPath
	
	output, err := cmd.Output()
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		}
		// For some commands like diff with no changes, exit code 1 is normal
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Check if this is a diff command with no changes
			if len(args) > 0 && args[0] == "diff" && exitErr.ExitCode() == 1 {
				return output, nil // Return output even with exit code 1
			}
		}
		return nil, fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
	}
	
	return output, nil
}