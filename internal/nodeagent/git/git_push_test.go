package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPushOptions_Validation(t *testing.T) {
	tests := []struct {
		name    string
		opts    PushOptions
		wantErr string
	}{
		{
			name: "valid options",
			opts: PushOptions{
				RepoDir:   "/tmp/repo",
				TargetRef: "workflow/abc/123",
				PAT:       "token123",
				UserName:  "Test User",
				UserEmail: "test@example.com",
			},
			wantErr: "",
		},
		{
			name: "missing repo_dir",
			opts: PushOptions{
				TargetRef: "workflow/abc/123",
				PAT:       "token123",
				UserName:  "Test User",
				UserEmail: "test@example.com",
			},
			wantErr: "repo_dir is required",
		},
		{
			name: "missing target_ref",
			opts: PushOptions{
				RepoDir:   "/tmp/repo",
				PAT:       "token123",
				UserName:  "Test User",
				UserEmail: "test@example.com",
			},
			wantErr: "target_ref is required",
		},
		{
			name: "missing pat",
			opts: PushOptions{
				RepoDir:   "/tmp/repo",
				TargetRef: "workflow/abc/123",
				UserName:  "Test User",
				UserEmail: "test@example.com",
			},
			wantErr: "pat is required",
		},
		{
			name: "missing user_name",
			opts: PushOptions{
				RepoDir:   "/tmp/repo",
				TargetRef: "workflow/abc/123",
				PAT:       "token123",
				UserEmail: "test@example.com",
			},
			wantErr: "user_name is required",
		},
		{
			name: "missing user_email",
			opts: PushOptions{
				RepoDir:   "/tmp/repo",
				TargetRef: "workflow/abc/123",
				PAT:       "token123",
				UserName:  "Test User",
			},
			wantErr: "user_email is required",
		},
		{
			name: "whitespace only fields",
			opts: PushOptions{
				RepoDir:   "   ",
				TargetRef: "  ",
				PAT:       "  ",
				UserName:  "  ",
				UserEmail: "  ",
			},
			wantErr: "repo_dir is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePushOptions(tt.opts)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

// Note: askpass helper removed in favor of GIT_HTTP_EXTRAHEADER for
// non-persistent Authorization. Redaction and validation tests remain below.

func TestRedactError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		pat     string
		wantMsg string
	}{
		{
			name:    "nil error",
			err:     nil,
			pat:     "token123",
			wantMsg: "",
		},
		{
			name:    "error without pat",
			err:     &execError{msg: "git push failed: network error"},
			pat:     "token123",
			wantMsg: "git push failed: network error",
		},
		{
			name:    "error with pat",
			err:     &execError{msg: "git push failed: authentication failed for https://token123@gitlab.com/repo.git"},
			pat:     "token123",
			wantMsg: "git push failed: authentication failed for https://[REDACTED]@gitlab.com/repo.git",
		},
		{
			name:    "error with multiple pat occurrences",
			err:     &execError{msg: "token123 was used with token123 in request"},
			pat:     "token123",
			wantMsg: "[REDACTED] was used with [REDACTED] in request",
		},
		{
			name:    "empty pat",
			err:     &execError{msg: "git push failed"},
			pat:     "",
			wantMsg: "git push failed",
		},
		{
			name:    "error with url-encoded pat",
			err:     &execError{msg: "git push failed: url contains token%40special"},
			pat:     "token@special",
			wantMsg: "git push failed: url contains [REDACTED]",
		},
		{
			name:    "error with space-encoded pat",
			err:     &execError{msg: "authentication failed with token%20value"},
			pat:     "token value",
			wantMsg: "authentication failed with [REDACTED]",
		},
		{
			name:    "error with literal and encoded pat",
			err:     &execError{msg: "failed with token@special and token%40special"},
			pat:     "token@special",
			wantMsg: "failed with [REDACTED] and [REDACTED]",
		},
		{
			name:    "error with slash-encoded pat",
			err:     &execError{msg: "failed: token%2Fvalue not allowed"},
			pat:     "token/value",
			wantMsg: "failed: [REDACTED] not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactError(tt.err, tt.pat)
			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil, got: %v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected error, got nil")
				return
			}

			if result.Error() != tt.wantMsg {
				t.Errorf("RedactError() = %q, want %q", result.Error(), tt.wantMsg)
			}
		})
	}
}

// execError is a simple error type for testing.
type execError struct {
	msg string
}

func (e *execError) Error() string {
	return e.msg
}

func TestPush_ValidationRedaction(t *testing.T) {
	// Test that validation errors don't leak PAT.
	p := NewPusher()
	ctx := context.Background()

	opts := PushOptions{
		RepoDir:   "",
		TargetRef: "test-branch",
		PAT:       "glpat-secret-validation-pat",
		UserName:  "Test User",
		UserEmail: "test@example.com",
	}

	err := p.Push(ctx, opts)

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	errMsg := err.Error()

	// Verify that the PAT is not in the error message.
	if strings.Contains(errMsg, "glpat-secret-validation-pat") {
		t.Errorf("PAT leaked in validation error: %s", errMsg)
	}
}

func TestPush_Integration(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping integration test")
	}

	t.Run("configure git user", func(t *testing.T) {
		// Create a temporary git repository.
		tmpDir := t.TempDir()
		repoDir := filepath.Join(tmpDir, "test-repo")

		// Initialize git repo.
		cmd := exec.Command("git", "init", repoDir)
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to init git repo: %v", err)
		}

		// Create a pusher and configure user.
		p := &pusher{}
		ctx := context.Background()
		userName := "Test User"
		userEmail := "test@example.com"

		if err := p.configureGitUser(ctx, repoDir, userName, userEmail); err != nil {
			t.Fatalf("configureGitUser failed: %v", err)
		}

		// Verify user.name was set.
		cmd = exec.Command("git", "config", "user.name")
		cmd.Dir = repoDir
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("failed to read git config user.name: %v", err)
		}
		if got := strings.TrimSpace(string(output)); got != userName {
			t.Errorf("user.name = %q, want %q", got, userName)
		}

		// Verify user.email was set.
		cmd = exec.Command("git", "config", "user.email")
		cmd.Dir = repoDir
		output, err = cmd.Output()
		if err != nil {
			t.Fatalf("failed to read git config user.email: %v", err)
		}
		if got := strings.TrimSpace(string(output)); got != userEmail {
			t.Errorf("user.email = %q, want %q", got, userEmail)
		}
	})

	t.Run("push with missing repo", func(t *testing.T) {
		p := NewPusher()
		ctx := context.Background()

		opts := PushOptions{
			RepoDir:   "/nonexistent/repo",
			TargetRef: "test-branch",
			PAT:       "fake-token",
			UserName:  "Test User",
			UserEmail: "test@example.com",
		}

		err := p.Push(ctx, opts)
		if err == nil {
			t.Errorf("expected error for nonexistent repo, got nil")
		}

		// Verify PAT is not in error message.
		if err != nil && strings.Contains(err.Error(), "fake-token") {
			t.Errorf("error message contains PAT: %v", err)
		}
	})
}

func TestRunGitCommand(t *testing.T) {
	// Skip if git is not available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	t.Run("successful command", func(t *testing.T) {
		ctx := context.Background()
		err := runGitCommand(ctx, "", nil, "version")
		if err != nil {
			t.Errorf("runGitCommand(version) failed: %v", err)
		}
	})

	t.Run("command with custom env", func(t *testing.T) {
		ctx := context.Background()
		env := []string{"GIT_TERMINAL_PROMPT=0"}
		err := runGitCommand(ctx, "", env, "version")
		if err != nil {
			t.Errorf("runGitCommand(version) with env failed: %v", err)
		}
	})

	t.Run("failing command", func(t *testing.T) {
		ctx := context.Background()
		err := runGitCommand(ctx, "", nil, "invalid-command")
		if err == nil {
			t.Errorf("expected error for invalid command, got nil")
		}
	})

	t.Run("command in specific directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		repoDir := filepath.Join(tmpDir, "test-repo")

		// Initialize git repo.
		cmd := exec.Command("git", "init", repoDir)
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to init git repo: %v", err)
		}

		ctx := context.Background()
		err := runGitCommand(ctx, repoDir, nil, "status")
		if err != nil {
			t.Errorf("runGitCommand(status) in directory failed: %v", err)
		}
	})
}

func TestNewPusher(t *testing.T) {
	p := NewPusher()
	if p == nil {
		t.Errorf("NewPusher() returned nil")
	}

	// Verify it implements the Pusher interface.
	var _ Pusher = (*pusher)(nil)
}
