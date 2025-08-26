package openrewrite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/iw2rmb/ploy/internal/testutils/fixtures"
)

func TestGitManagerImpl_InitializeRepo(t *testing.T) {
	tests := []struct {
		name      string
		jobID     string
		setupTar  func() []byte
		wantError bool
		errorMsg  string
		validate  func(t *testing.T, repoPath string)
	}{
		{
			name:  "successful initialization with simple project",
			jobID: "test-job-001",
			setupTar: func() []byte {
				fixture := &fixtures.ApplicationTar{
					Name:     "test-project",
					Language: "java",
					Files: map[string]string{
						"pom.xml": `<?xml version="1.0" encoding="UTF-8"?>
<project>
	<groupId>com.example</groupId>
	<artifactId>demo</artifactId>
	<version>1.0.0</version>
</project>`,
						"src/main/java/App.java": `package com.example;
public class App {
	public static void main(String[] args) {
		System.out.println("Hello World");
	}
}`,
					},
				}
				tarData, err := fixtures.CreateTarballFromFixture(fixture)
				require.NoError(t, err)
				return tarData
			},
			wantError: false,
			validate: func(t *testing.T, repoPath string) {
				// Verify Git repository was initialized
				gitDir := filepath.Join(repoPath, ".git")
				assert.DirExists(t, gitDir, "Git directory should exist")
				
				// Verify files were extracted
				pomPath := filepath.Join(repoPath, "pom.xml")
				assert.FileExists(t, pomPath, "pom.xml should exist")
				
				javaPath := filepath.Join(repoPath, "src", "main", "java", "App.java")
				assert.FileExists(t, javaPath, "Java file should exist")
				
				// Verify initial commit was created
				gitLogFile := filepath.Join(gitDir, "logs", "HEAD")
				assert.FileExists(t, gitLogFile, "Git log should exist")
				
				// Verify tag was created
				tagPath := filepath.Join(gitDir, "refs", "tags", "before-transform")
				assert.FileExists(t, tagPath, "before-transform tag should exist")
			},
		},
		{
			name:  "handles empty tar archive",
			jobID: "test-job-002",
			setupTar: func() []byte {
				return []byte{}
			},
			wantError: true,
			errorMsg:  "invalid tar archive",
		},
		{
			name:  "handles invalid tar data",
			jobID: "test-job-003",
			setupTar: func() []byte {
				return []byte("not a tar file")
			},
			wantError: true,
			errorMsg:  "tar extraction failed",
		},
		{
			name:  "creates nested directory structure",
			jobID: "test-job-004",
			setupTar: func() []byte {
				fixture := &fixtures.ApplicationTar{
					Name:     "gradle-project",
					Language: "java",
					Files: map[string]string{
						"build.gradle": "plugins { id 'java' }",
						"src/main/java/com/example/service/UserService.java": "package com.example.service;",
						"src/test/java/com/example/service/UserServiceTest.java": "package com.example.service;",
					},
				}
				tarData, err := fixtures.CreateTarballFromFixture(fixture)
				require.NoError(t, err)
				return tarData
			},
			wantError: false,
			validate: func(t *testing.T, repoPath string) {
				servicePath := filepath.Join(repoPath, "src", "main", "java", "com", "example", "service", "UserService.java")
				assert.FileExists(t, servicePath, "Nested service file should exist")
				
				testPath := filepath.Join(repoPath, "src", "test", "java", "com", "example", "service", "UserServiceTest.java")
				assert.FileExists(t, testPath, "Test file should exist")
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tempDir := t.TempDir()
			config := &Config{
				WorkDir: tempDir,
				GitPath: "git",
			}
			
			manager := NewGitManager(config)
			ctx := context.Background()
			tarData := tt.setupTar()
			
			// Execute
			repoPath, err := manager.InitializeRepo(ctx, tt.jobID, tarData)
			
			// Verify
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, repoPath)
				assert.Contains(t, repoPath, tt.jobID)
				
				if tt.validate != nil {
					tt.validate(t, repoPath)
				}
				
				// Cleanup
				_ = manager.Cleanup(repoPath)
			}
		})
	}
}

func TestGitManagerImpl_GenerateDiff(t *testing.T) {
	tests := []struct {
		name         string
		initialFiles map[string]string
		makeChanges  func(t *testing.T, repoPath string)
		wantError    bool
		validateDiff func(t *testing.T, diff []byte)
	}{
		{
			name: "generates diff for file modifications",
			initialFiles: map[string]string{
				"App.java": `public class App {
	public static void main(String[] args) {
		System.out.println("Java 11");
	}
}`,
			},
			makeChanges: func(t *testing.T, repoPath string) {
				// Modify the file
				filePath := filepath.Join(repoPath, "App.java")
				newContent := `public class App {
	public static void main(String[] args) {
		System.out.println("Java 17");
	}
}`
				require.NoError(t, os.WriteFile(filePath, []byte(newContent), 0644))
			},
			wantError: false,
			validateDiff: func(t *testing.T, diff []byte) {
				diffStr := string(diff)
				assert.Contains(t, diffStr, "-		System.out.println(\"Java 11\");")
				assert.Contains(t, diffStr, "+		System.out.println(\"Java 17\");")
				assert.Contains(t, diffStr, "App.java")
			},
		},
		{
			name: "generates diff for new files",
			initialFiles: map[string]string{
				"existing.txt": "existing",
			},
			makeChanges: func(t *testing.T, repoPath string) {
				// Add new file
				newFilePath := filepath.Join(repoPath, "new-file.java")
				content := `public class NewFile {}`
				require.NoError(t, os.WriteFile(newFilePath, []byte(content), 0644))
			},
			wantError: false,
			validateDiff: func(t *testing.T, diff []byte) {
				diffStr := string(diff)
				assert.Contains(t, diffStr, "new-file.java")
				assert.Contains(t, diffStr, "+public class NewFile {}")
			},
		},
		{
			name: "generates diff for file deletions",
			initialFiles: map[string]string{
				"to-delete.txt": "delete me",
			},
			makeChanges: func(t *testing.T, repoPath string) {
				// Delete the file
				filePath := filepath.Join(repoPath, "to-delete.txt")
				require.NoError(t, os.Remove(filePath))
			},
			wantError: false,
			validateDiff: func(t *testing.T, diff []byte) {
				diffStr := string(diff)
				assert.Contains(t, diffStr, "to-delete.txt")
				assert.Contains(t, diffStr, "-delete me")
			},
		},
		{
			name: "handles no changes",
			initialFiles: map[string]string{
				"unchanged.txt": "no changes",
			},
			makeChanges: func(t *testing.T, repoPath string) {
				// No changes
			},
			wantError: false,
			validateDiff: func(t *testing.T, diff []byte) {
				// Empty diff when no changes
				assert.Empty(t, diff, "Diff should be empty when no changes are made")
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tempDir := t.TempDir()
			config := &Config{
				WorkDir: tempDir,
				GitPath: "git",
			}
			
			manager := NewGitManager(config)
			ctx := context.Background()
			
			// Create tar archive with initial files
			fixtureFiles := tt.initialFiles
			if fixtureFiles == nil {
				fixtureFiles = map[string]string{".gitkeep": ""}
			}
			fixture := &fixtures.ApplicationTar{
				Name:     "diff-test",
				Language: "text",
				Files:    fixtureFiles,
			}
			tarData, err := fixtures.CreateTarballFromFixture(fixture)
			require.NoError(t, err)
			
			// Initialize repo
			repoPath, err := manager.InitializeRepo(ctx, "test-diff", tarData)
			require.NoError(t, err)
			defer manager.Cleanup(repoPath)
			
			// Initial files are already in the repo from tar archive
			
			// Make changes
			if tt.makeChanges != nil {
				tt.makeChanges(t, repoPath)
			}
			
			// Generate diff
			diff, err := manager.GenerateDiff(ctx, repoPath)
			
			// Verify
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validateDiff != nil {
					tt.validateDiff(t, diff)
				}
			}
		})
	}
}

func TestGitManagerImpl_Cleanup(t *testing.T) {
	// Setup
	tempDir := t.TempDir()
	config := &Config{
		WorkDir: tempDir,
		GitPath: "git",
	}
	
	manager := NewGitManager(config)
	ctx := context.Background()
	
	// Create a simple tar archive
	fixture := &fixtures.ApplicationTar{
		Name:     "cleanup-test",
		Language: "text",
		Files: map[string]string{
			"test.txt": "test",
		},
	}
	tarData, err := fixtures.CreateTarballFromFixture(fixture)
	require.NoError(t, err)
	
	// Initialize repo
	repoPath, err := manager.InitializeRepo(ctx, "test-cleanup", tarData)
	require.NoError(t, err)
	
	// Verify repo exists
	assert.DirExists(t, repoPath)
	
	// Cleanup
	err = manager.Cleanup(repoPath)
	assert.NoError(t, err)
	
	// Verify repo is removed
	assert.NoDirExists(t, repoPath)
	
	// Test cleanup of non-existent path
	err = manager.Cleanup("/non/existent/path")
	assert.NoError(t, err) // Should not error on non-existent paths
}

func TestGitManagerImpl_ConcurrentOperations(t *testing.T) {
	// Test that multiple repos can be managed concurrently
	tempDir := t.TempDir()
	config := &Config{
		WorkDir: tempDir,
		GitPath: "git",
	}
	
	manager := NewGitManager(config)
	ctx := context.Background()
	
	// Create test data
	fixture := &fixtures.ApplicationTar{
		Name:     "concurrent-test",
		Language: "text",
		Files: map[string]string{
			"concurrent.txt": "test",
		},
	}
	tarData, err := fixtures.CreateTarballFromFixture(fixture)
	require.NoError(t, err)
	
	// Run concurrent initializations
	numJobs := 5
	repoPaths := make([]string, numJobs)
	errors := make([]error, numJobs)
	
	for i := 0; i < numJobs; i++ {
		i := i
		go func() {
			jobID := fmt.Sprintf("concurrent-job-%d", i)
			repoPaths[i], errors[i] = manager.InitializeRepo(ctx, jobID, tarData)
		}()
	}
	
	// Wait a bit for goroutines to complete
	time.Sleep(2 * time.Second)
	
	// Verify all succeeded
	for i := 0; i < numJobs; i++ {
		assert.NoError(t, errors[i], "Job %d should succeed", i)
		assert.NotEmpty(t, repoPaths[i], "Job %d should have repo path", i)
		assert.DirExists(t, repoPaths[i], "Job %d repo should exist", i)
		
		// Cleanup
		_ = manager.Cleanup(repoPaths[i])
	}
}

func TestGitManagerImpl_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	config := &Config{
		WorkDir: tempDir,
		GitPath: "git",
	}
	
	manager := NewGitManager(config)
	
	// Create a large tar archive that takes time to process
	fixtureFiles := make(map[string]string)
	for i := 0; i < 100; i++ {
		fixtureFiles[fmt.Sprintf("file%d.txt", i)] = strings.Repeat("x", 10000)
	}
	fixture := &fixtures.ApplicationTar{
		Name:     "large-test",
		Language: "text",
		Files:    fixtureFiles,
	}
	tarData, err := fixtures.CreateTarballFromFixture(fixture)
	require.NoError(t, err)
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	
	// Attempt initialization with cancelled context
	_, err = manager.InitializeRepo(ctx, "cancelled-job", tarData)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context")
}