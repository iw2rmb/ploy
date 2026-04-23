package nodeagent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateJavaClasspathPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name: "accepts_root_gradle_and_maven_paths",
			content: strings.Join([]string{
				"/root/.gradle/caches/modules-2/files-2.1/a/b/c/lib.jar",
				"/root/.m2/repository/org/example/lib/1.0.0/lib-1.0.0.jar",
				"/workspace/build/classes/java/main",
				"",
			}, "\n"),
		},
		{
			name: "rejects_relative_paths",
			content: strings.Join([]string{
				"/root/.m2/repository/org/example/lib/1.0.0/lib-1.0.0.jar",
				"relative/path.jar",
			}, "\n"),
			wantErr: "must be absolute path",
		},
		{
			name: "rejects_home_gradle_cache_prefix",
			content: strings.Join([]string{
				"/workspace/build/classes/java/main",
				"/home/gradle/.gradle/caches/modules-2/files-2.1/a/b/c/lib.jar",
			}, "\n"),
			wantErr: "non-portable gradle cache path",
		},
		{
			name:    "rejects_home_gradle_cache_root",
			content: "/home/gradle/.gradle\n",
			wantErr: "non-portable gradle cache path",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "java.classpath")
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("write classpath file: %v", err)
			}

			err := validateJavaClasspathPath(path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateJavaClasspathPath() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateJavaClasspathPath() error = nil, want %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateJavaClasspathPath() error = %q, want substring %q", err, tc.wantErr)
			}
		})
	}
}
