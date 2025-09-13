package transflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultSignatureGenerator_GenerateSignature(t *testing.T) {
	sg := NewDefaultSignatureGenerator()

	tests := []struct {
		name     string
		lang     string
		compiler string
		stdout   []byte
		stderr   []byte
		expected func(string) bool // function to validate the signature
	}{
		{
			name:     "Java compilation error",
			lang:     "java",
			compiler: "javac",
			stdout:   []byte(""),
			stderr:   []byte("Test.java:10: error: cannot find symbol\n  symbol:   variable undefinedVar\n  location: class Test"),
			expected: func(sig string) bool {
				return len(sig) == 16 // Should be 8 bytes = 16 hex chars
			},
		},
		{
			name:     "Go build error",
			lang:     "go",
			compiler: "go",
			stdout:   []byte(""),
			stderr:   []byte("main.go:5:2: undefined: fmt.Println"),
			expected: func(sig string) bool {
				return ValidateSignature(sig)
			},
		},
		{
			name:     "Same logical error different paths",
			lang:     "java",
			compiler: "javac",
			stdout:   []byte(""),
			stderr:   []byte("/home/user1/project/Test.java:10: error: cannot find symbol"),
			expected: func(sig string) bool {
				// Generate another signature with different path
				sg2 := NewDefaultSignatureGenerator()
				stderr2 := []byte("/home/user2/different/Test.java:10: error: cannot find symbol")
				sig2 := sg2.GenerateSignature("java", "javac", []byte(""), stderr2)
				return sig == sig2 // Should be the same after normalization
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			signature := sg.GenerateSignature(test.lang, test.compiler, test.stdout, test.stderr)
			assert.True(t, test.expected(signature), "Signature validation failed for: %s", signature)
		})
	}
}

func TestDefaultSignatureGenerator_NormalizeErrorText(t *testing.T) {
	sg := NewDefaultSignatureGenerator()

	tests := []struct {
		name     string
		input    string
		expected func(string) bool
	}{
		{
			name:  "Remove timestamps",
			input: "2023-12-01T10:30:45Z Build failed\n10:30:45 Error occurred",
			expected: func(output string) bool {
				return !strings.Contains(output, "2023-12-01T10:30:45Z") &&
					!strings.Contains(output, "10:30:45")
			},
		},
		{
			name:  "Remove absolute paths",
			input: "/home/user/project/src/main/java/Test.java:10: error",
			expected: func(output string) bool {
				return strings.Contains(output, "[PATH]") &&
					!strings.Contains(output, "/home/user/project")
			},
		},
		{
			name:  "Remove line numbers",
			input: "Test.java:10:5: error at line 10",
			expected: func(output string) bool {
				return strings.Contains(output, "[LINE]")
			},
		},
		{
			name:  "Remove memory addresses",
			input: "Segmentation fault at 0x7fff5fbff5c0",
			expected: func(output string) bool {
				return !strings.Contains(output, "0x7fff5fbff5c0") &&
					strings.Contains(output, "[ADDR]")
			},
		},
		{
			name:  "Remove build IDs",
			input: "Build job-abc123def456 failed with build-789012345",
			expected: func(output string) bool {
				return strings.Contains(output, "[BUILD_ID]")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized := sg.normalizeErrorText(test.input)
			assert.True(t, test.expected(normalized), "Normalization failed. Output: %s", normalized)
		})
	}
}

func TestDefaultSignatureGenerator_ExtractKeyErrorPatterns(t *testing.T) {
	sg := NewDefaultSignatureGenerator()

	tests := []struct {
		name     string
		input    string
		expected func(string) bool
	}{
		{
			name: "Extract error lines",
			input: `INFO: Starting build
ERROR: Compilation failed
INFO: Cleaning up
FATAL: Process terminated`,
			expected: func(output string) bool {
				return strings.Contains(output, "ERROR: Compilation failed") &&
					strings.Contains(output, "FATAL: Process terminated") &&
					!strings.Contains(output, "INFO: Starting build")
			},
		},
		{
			name: "Extract exception lines",
			input: `Starting application
Exception: NullPointerException at line 42
Application started successfully`,
			expected: func(output string) bool {
				return strings.Contains(output, "Exception: NullPointerException")
			},
		},
		{
			name: "Handle undefined references",
			input: `Linking...
undefined reference to 'missing_function'
Link completed`,
			expected: func(output string) bool {
				return strings.Contains(output, "undefined reference")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := sg.extractKeyErrorPatterns(test.input)
			assert.True(t, test.expected(result), "Pattern extraction failed. Output: %s", result)
		})
	}
}

func TestDefaultSignatureGenerator_NormalizePatch(t *testing.T) {
	sg := NewDefaultSignatureGenerator()

	tests := []struct {
		name              string
		patch             []byte
		expectNorm        func([]byte) bool
		expectFingerprint func(string) bool
	}{
		{
			name: "Git diff patch",
			patch: []byte(`diff --git a/Test.java b/Test.java
index abc123..def456 100644
--- a/Test.java	2023-12-01 10:30:45.123456789 +0000
+++ b/Test.java	2023-12-01 10:31:12.987654321 +0000
@@ -1,4 +1,3 @@
-import java.util.*;
 import java.io.*;
 
 public class Test {`),
			expectNorm: func(normalized []byte) bool {
				content := string(normalized)
				return !strings.Contains(content, "diff --git") &&
					!strings.Contains(content, "index abc123") &&
					strings.Contains(content, "--- [FILE_A]") &&
					strings.Contains(content, "+++ [FILE_B]")
			},
			expectFingerprint: func(fp string) bool {
				return len(fp) == 64 // SHA-256 hex string
			},
		},
		{
			name: "Simple unified diff",
			patch: []byte(`--- original.txt
+++ modified.txt
@@ -1,3 +1,3 @@
 line 1
-old line 2
+new line 2
 line 3`),
			expectNorm: func(normalized []byte) bool {
				content := string(normalized)
				return strings.Contains(content, "--- [FILE_A]") &&
					strings.Contains(content, "+++ [FILE_B]") &&
					strings.Contains(content, "-old line 2") &&
					strings.Contains(content, "+new line 2")
			},
			expectFingerprint: func(fp string) bool {
				return len(fp) == 64
			},
		},
		{
			name: "Same logical change different timestamps",
			patch: []byte(`--- Test.java	2023-12-01 10:30:45
+++ Test.java	2023-12-01 10:31:45
@@ -1,2 +1,2 @@
-old content
+new content`),
			expectNorm: func(normalized []byte) bool {
				// Generate another patch with different timestamps
				patch2 := []byte(`--- Test.java	2023-12-02 15:22:33
+++ Test.java	2023-12-02 15:23:11
@@ -1,2 +1,2 @@
-old content
+new content`)
				normalized2, fp2 := sg.NormalizePatch(patch2)
				_, fp1 := sg.NormalizePatch(normalized)

				// Both should normalize to the same content and fingerprint
				return string(normalized) == string(normalized2) && fp1 == fp2
			},
			expectFingerprint: func(fp string) bool {
				return len(fp) == 64
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, fingerprint := sg.NormalizePatch(test.patch)
			assert.True(t, test.expectNorm(normalized), "Patch normalization failed")
			assert.True(t, test.expectFingerprint(fingerprint), "Fingerprint validation failed: %s", fingerprint)
		})
	}
}

func TestLogSanitizer_Sanitize(t *testing.T) {
	sanitizer := NewLogSanitizer()

	tests := []struct {
		name     string
		input    string
		expected func(string) bool
	}{
		{
			name:  "Remove GitHub token",
			input: "Using token ghp_1234567890abcdefghijklmnopqrstuvwxyz",
			expected: func(output string) bool {
				return !strings.Contains(output, "ghp_1234567890abcdefghijklmnopqrstuvwxyz") &&
					strings.Contains(output, "[REDACTED_TOKEN]")
			},
		},
		{
			name:  "Remove GitLab token",
			input: "GitLab token: glpat-abcdefghij1234567890",
			expected: func(output string) bool {
				return !strings.Contains(output, "glpat-abcdefghij1234567890") &&
					strings.Contains(output, "[REDACTED_TOKEN]")
			},
		},
		{
			name:  "Remove API key",
			input: `Config: {"api_key": "abc123def456ghi789jkl012mno345"}`,
			expected: func(output string) bool {
				return !strings.Contains(output, "abc123def456ghi789jkl012mno345") &&
					strings.Contains(output, "[REDACTED_KEY]")
			},
		},
		{
			name:  "Remove password",
			input: "mysql --password=secretpass123 -u user",
			expected: func(output string) bool {
				return !strings.Contains(output, "secretpass123") &&
					strings.Contains(output, "[REDACTED_PASSWORD]")
			},
		},
		{
			name:  "Remove URL with auth",
			input: "git clone https://username:password@github.com/user/repo.git",
			expected: func(output string) bool {
				return !strings.Contains(output, "username:password") &&
					strings.Contains(output, "[REDACTED_AUTH_URL]")
			},
		},
		{
			name:  "Remove JWT token",
			input: "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
			expected: func(output string) bool {
				return !strings.Contains(output, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9") &&
					strings.Contains(output, "[REDACTED_JWT]")
			},
		},
		{
			name:  "Handle very long logs",
			input: strings.Repeat("A", 60000), // 60KB log
			expected: func(output string) bool {
				return len(output) <= 50020 && // Max length + truncation message
					strings.Contains(output, "[TRUNCATED]")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sanitized := sanitizer.Sanitize(test.input)
			assert.True(t, test.expected(sanitized), "Sanitization failed. Output: %s", sanitized)
		})
	}
}

func TestCreateSanitizedLogs(t *testing.T) {
	stdout := "Build successful with token ghp_abc123def456"
	stderr := "Warning: API key detected: api_key=secret123456"

	logs := CreateSanitizedLogs(stdout, stderr)

	assert.NotNil(t, logs)
	assert.False(t, strings.Contains(logs.Stdout, "ghp_abc123def456"))
	assert.False(t, strings.Contains(logs.Stderr, "secret123456"))
	assert.Contains(t, logs.Stdout, "[REDACTED_TOKEN]")
	assert.Contains(t, logs.Stderr, "[REDACTED_KEY]")
	assert.False(t, logs.Truncated) // Short logs shouldn't be truncated
}

func TestValidateSignature(t *testing.T) {
	tests := []struct {
		name      string
		signature string
		valid     bool
	}{
		{"Valid signature", "abc123def456789a", true},
		{"Too short", "abc123", false},
		{"Too long", "abc123def456789abcdef", false},
		{"Invalid characters", "xyz123def456789g", false},
		{"Empty string", "", false},
		{"Uppercase letters", "ABC123DEF456789A", false}, // Should be lowercase
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := ValidateSignature(test.signature)
			assert.Equal(t, test.valid, result, "Validation failed for signature: %s", test.signature)
		})
	}
}

func TestSanitizeForStorage(t *testing.T) {
	input := "Build log with sensitive data: token=ghp_secrettoken123 and password=mysecret"
	sanitized := SanitizeForStorage(input)

	assert.NotContains(t, sanitized, "ghp_secrettoken123")
	assert.NotContains(t, sanitized, "mysecret")
	assert.Contains(t, sanitized, "[REDACTED_TOKEN]")
	assert.Contains(t, sanitized, "[REDACTED_PASSWORD]")
}

// Benchmark tests for performance
func BenchmarkSignatureGeneration(b *testing.B) {
	sg := NewDefaultSignatureGenerator()
	stderr := []byte(`
Test.java:10: error: cannot find symbol
  symbol:   variable undefinedVar
  location: class Test
Test.java:15: error: method foo() is undefined
  location: class Test
	`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sg.GenerateSignature("java", "javac", []byte(""), stderr)
	}
}

func BenchmarkLogSanitization(b *testing.B) {
	sanitizer := NewLogSanitizer()
	logContent := `
2023-12-01 10:30:45 INFO Starting build
Using GitHub token: ghp_1234567890abcdefghijklmnopqrstuvwxyz
Connecting to database with password=secretpass123
Build output follows...
Error: compilation failed at /home/user/project/src/Test.java:42
Memory dump at address 0x7fff5fbff5c0
Build ID: job-abc123def456
Thread [12345] completed
	`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizer.Sanitize(logContent)
	}
}

func BenchmarkPatchNormalization(b *testing.B) {
	sg := NewDefaultSignatureGenerator()
	patch := []byte(`diff --git a/Test.java b/Test.java
index abc123..def456 100644
--- a/Test.java	2023-12-01 10:30:45.123456789 +0000
+++ b/Test.java	2023-12-01 10:31:12.987654321 +0000
@@ -10,15 +10,12 @@
 public class Test {
-    import java.util.*;
-    import java.io.*;
-    import java.net.*;
+    import java.io.*;
     
     public void method() {
-        System.out.println("old");
+        System.out.println("new");
     }
 }`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sg.NormalizePatch(patch)
	}
}
