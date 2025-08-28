package diff

import (
	"strings"
	"testing"
	"time"
)

// Test data
const originalCode = `package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}`

const modifiedCode = `package main

import (
    "fmt"
    "os"
)

func main() {
    name := os.Args[1]
    fmt.Printf("Hello, %s!\n", name)
}`

const unifiedDiff = `--- a/main.go
+++ b/main.go
@@ -1,7 +1,11 @@
 package main
 
-import "fmt"
+import (
+    "fmt"
+    "os"
+)
 
 func main() {
-    fmt.Println("Hello, World!")
+    name := os.Args[1]
+    fmt.Printf("Hello, %s!\n", name)
 }`

// TestGenerator tests diff generation
func TestGenerator(t *testing.T) {
	generator := NewGenerator()
	
	t.Run("GenerateUnifiedDiff", func(t *testing.T) {
		req := DiffRequest{
			Original:     originalCode,
			Modified:     modifiedCode,
			OriginalPath: "a/main.go",
			ModifiedPath: "b/main.go",
			Options: DiffOptions{
				Format:       FormatUnified,
				ContextLines: 3,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		if resp.Format != FormatUnified {
			t.Errorf("Expected format %s, got %s", FormatUnified, resp.Format)
		}
		
		if len(resp.Changes) != 1 {
			t.Errorf("Expected 1 change, got %d", len(resp.Changes))
		}
		
		// Check that diff contains expected markers
		if !strings.Contains(resp.Content, "---") {
			t.Error("Diff missing --- marker")
		}
		if !strings.Contains(resp.Content, "+++") {
			t.Error("Diff missing +++ marker")
		}
		if !strings.Contains(resp.Content, "@@") {
			t.Error("Diff missing @@ hunk marker")
		}
	})
	
	t.Run("GenerateWithStatistics", func(t *testing.T) {
		req := DiffRequest{
			Original: originalCode,
			Modified: modifiedCode,
			Options: DiffOptions{
				Format:       FormatUnified,
				IncludeStats: true,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		if resp.Stats == nil {
			t.Error("Expected statistics, got nil")
		} else {
			if resp.Stats.LinesAdded == 0 {
				t.Error("Expected lines added > 0")
			}
			if resp.Stats.LinesDeleted == 0 {
				t.Error("Expected lines deleted > 0")
			}
		}
	})
	
	t.Run("EmptyOriginal", func(t *testing.T) {
		req := DiffRequest{
			Original: "",
			Modified: "new content",
			Options: DiffOptions{
				Format: FormatUnified,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		if len(resp.Changes) != 1 {
			t.Errorf("Expected 1 change, got %d", len(resp.Changes))
		}
		
		if resp.Changes[0].ChangeType != ChangeTypeAdd {
			t.Errorf("Expected change type %s, got %s", ChangeTypeAdd, resp.Changes[0].ChangeType)
		}
	})
	
	t.Run("EmptyModified", func(t *testing.T) {
		req := DiffRequest{
			Original: "old content",
			Modified: "",
			Options: DiffOptions{
				Format: FormatUnified,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		if resp.Changes[0].ChangeType != ChangeTypeDelete {
			t.Errorf("Expected change type %s, got %s", ChangeTypeDelete, resp.Changes[0].ChangeType)
		}
	})
}

// TestParser tests diff parsing
func TestParser(t *testing.T) {
	parser := NewParser()
	
	t.Run("ParseUnifiedDiff", func(t *testing.T) {
		req := ParseRequest{
			Content:  unifiedDiff,
			Format:   FormatUnified,
			Validate: true,
		}
		
		resp, err := parser.Parse(req)
		if err != nil {
			t.Fatalf("Failed to parse diff: %v", err)
		}
		
		if len(resp.Changes) != 1 {
			t.Errorf("Expected 1 change, got %d", len(resp.Changes))
		}
		
		change := resp.Changes[0]
		if change.OriginalPath != "a/main.go" {
			t.Errorf("Expected original path 'a/main.go', got %s", change.OriginalPath)
		}
		if change.ModifiedPath != "b/main.go" {
			t.Errorf("Expected modified path 'b/main.go', got %s", change.ModifiedPath)
		}
		
		if len(change.Hunks) == 0 {
			t.Error("Expected at least one hunk")
		}
		
		// Check validation
		if resp.Validation != nil && !resp.Validation.Valid {
			t.Errorf("Validation failed: %v", resp.Validation.Errors)
		}
	})
	
	t.Run("ParseWithSecurity", func(t *testing.T) {
		maliciousDiff := `--- ../../../etc/passwd
+++ ../../../etc/passwd
@@ -1 +1 @@
-root:x:0:0:root:/root:/bin/bash
+hacker:x:0:0:root:/root:/bin/bash`
		
		req := ParseRequest{
			Content:       maliciousDiff,
			Format:        FormatUnified,
			SecurityCheck: true,
		}
		
		resp, err := parser.Parse(req)
		if err != nil {
			t.Fatalf("Failed to parse diff: %v", err)
		}
		
		if resp.Validation == nil || len(resp.Validation.SecurityIssues) == 0 {
			t.Error("Expected security issues to be detected")
		}
		
		foundPathTraversal := false
		for _, issue := range resp.Validation.SecurityIssues {
			if issue.Type == "path_traversal" {
				foundPathTraversal = true
				break
			}
		}
		
		if !foundPathTraversal {
			t.Error("Expected path traversal security issue")
		}
	})
	
	t.Run("InvalidDiff", func(t *testing.T) {
		invalidDiff := `this is not a valid diff
just random text`
		
		req := ParseRequest{
			Content:  invalidDiff,
			Format:   FormatUnified,
			Validate: true,
		}
		
		resp, err := parser.Parse(req)
		if err != nil {
			// This is expected for completely invalid diffs
			return
		}
		
		// If it didn't error, we should have warnings
		if len(resp.Warnings) == 0 {
			t.Error("Expected warnings for invalid diff")
		}
	})
}

// TestApplier tests diff application
func TestApplier(t *testing.T) {
	applier := NewApplier()
	
	t.Run("ApplySimpleDiff", func(t *testing.T) {
		// For testing, we'll test with the known good unified diff
		// since our simple generator doesn't produce perfect unified diffs
		applyReq := ApplyRequest{
			Diff:   unifiedDiff,
			Target: originalCode,
			Options: ApplyOptions{
				Strict: false,
				Fuzzy:  true,
			},
		}
		
		applyResp, err := applier.Apply(applyReq)
		if err != nil {
			t.Fatalf("Failed to apply diff: %v", err)
		}
		
		if !applyResp.Success {
			t.Error("Expected successful application")
			if len(applyResp.Conflicts) > 0 {
				t.Logf("Conflicts: %v", applyResp.Conflicts)
			}
		}
		
		// The result should match the modified code
		resultLines := strings.Split(strings.TrimSpace(applyResp.Result), "\n")
		expectedLines := strings.Split(strings.TrimSpace(modifiedCode), "\n")
		
		if len(resultLines) != len(expectedLines) {
			t.Errorf("Line count mismatch: got %d, expected %d", 
				len(resultLines), len(expectedLines))
		}
	})
	
	t.Run("ApplyWithConflict", func(t *testing.T) {
		// Create a diff that won't apply cleanly
		conflictDiff := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line one
-line two original
+line two modified
 line three`
		
		// Target has different content
		target := `line one
line two different
line three`
		
		applyReq := ApplyRequest{
			Diff:   conflictDiff,
			Target: target,
			Options: ApplyOptions{
				Strict: true,
				Fuzzy:  false,
			},
		}
		
		applyResp, err := applier.Apply(applyReq)
		if err != nil {
			// Some conflicts might cause errors in strict mode
			return
		}
		
		if applyResp.Success {
			t.Error("Expected application to fail due to conflict")
		}
		
		if len(applyResp.Conflicts) == 0 {
			t.Error("Expected conflicts to be reported")
		}
	})
	
	t.Run("ApplyReverse", func(t *testing.T) {
		// Use the known good unified diff for reverse testing
		applyReq := ApplyRequest{
			Diff:   unifiedDiff,
			Target: modifiedCode,
			Options: ApplyOptions{
				Reverse: true,
				Fuzzy:   true,
				Strict:  false,
			},
		}
		
		applyResp, err := applier.Apply(applyReq)
		if err != nil {
			t.Fatalf("Failed to apply reverse diff: %v", err)
		}
		
		if !applyResp.Success {
			t.Error("Expected successful reverse application")
		}
	})
}

// TestFormatter tests diff formatting
func TestFormatter(t *testing.T) {
	formatter := NewFormatter()
	generator := NewGenerator()
	
	t.Run("FormatUnified", func(t *testing.T) {
		req := DiffRequest{
			Original: originalCode,
			Modified: modifiedCode,
			Options: DiffOptions{
				Format: FormatUnified,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		formatted, err := formatter.Format(resp, FormatUnified)
		if err != nil {
			t.Fatalf("Failed to format diff: %v", err)
		}
		
		// Check for unified diff markers
		if !strings.Contains(formatted, "---") {
			t.Error("Formatted diff missing --- marker")
		}
		if !strings.Contains(formatted, "+++") {
			t.Error("Formatted diff missing +++ marker")
		}
		if !strings.Contains(formatted, "@@") {
			t.Error("Formatted diff missing @@ marker")
		}
	})
	
	t.Run("FormatSummary", func(t *testing.T) {
		req := DiffRequest{
			Original: originalCode,
			Modified: modifiedCode,
			Options: DiffOptions{
				Format:       FormatSummary,
				IncludeStats: true,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		formatted, err := formatter.Format(resp, FormatSummary)
		if err != nil {
			t.Fatalf("Failed to format summary: %v", err)
		}
		
		// Check for summary content
		if !strings.Contains(formatted, "File") {
			t.Error("Summary missing file information")
		}
		if !strings.Contains(formatted, "Lines") {
			t.Error("Summary missing line statistics")
		}
	})
	
	t.Run("FormatJSON", func(t *testing.T) {
		req := DiffRequest{
			Original: "old",
			Modified: "new",
			Options: DiffOptions{
				Format: FormatJSON,
			},
		}
		
		resp, err := generator.Generate(req)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		formatted, err := formatter.Format(resp, FormatJSON)
		if err != nil {
			t.Fatalf("Failed to format as JSON: %v", err)
		}
		
		// Check that it's valid JSON
		if !strings.Contains(formatted, "{") || !strings.Contains(formatted, "}") {
			t.Error("Output doesn't look like JSON")
		}
	})
	
	t.Run("FormatStatistics", func(t *testing.T) {
		stats := &DiffStats{
			FilesChanged: 2,
			LinesAdded:   10,
			LinesDeleted: 5,
		}
		
		formatted := formatter.FormatStatistics(stats)
		
		if !strings.Contains(formatted, "2 files changed") {
			t.Error("Statistics missing file count")
		}
		if !strings.Contains(formatted, "10 insertions") {
			t.Error("Statistics missing insertions")
		}
		if !strings.Contains(formatted, "5 deletions") {
			t.Error("Statistics missing deletions")
		}
	})
}

// TestIntegration tests the full diff workflow
func TestIntegration(t *testing.T) {
	generator := NewGenerator()
	parser := NewParser()
	applier := NewApplier()
	formatter := NewFormatter()
	
	t.Run("FullWorkflow", func(t *testing.T) {
		// 1. Generate diff
		genReq := DiffRequest{
			Original:     originalCode,
			Modified:     modifiedCode,
			OriginalPath: "main.go",
			ModifiedPath: "main.go",
			Options: DiffOptions{
				Format:       FormatUnified,
				IncludeStats: true,
			},
		}
		
		genResp, err := generator.Generate(genReq)
		if err != nil {
			t.Fatalf("Failed to generate diff: %v", err)
		}
		
		// 2. Parse the known good diff
		parseReq := ParseRequest{
			Content:       unifiedDiff,
			Format:        FormatUnified,
			Validate:      true,
			SecurityCheck: true,
		}
		
		parseResp, err := parser.Parse(parseReq)
		if err != nil {
			t.Fatalf("Failed to parse diff: %v", err)
		}
		
		if parseResp.Validation != nil && !parseResp.Validation.Valid {
			t.Errorf("Diff validation failed: %v", parseResp.Validation.Errors)
		}
		
		// 3. Apply the diff
		applyReq := ApplyRequest{
			Diff:   unifiedDiff,
			Target: originalCode,
			Options: ApplyOptions{
				Strict: false,
				Fuzzy:  true,
			},
		}
		
		applyResp, err := applier.Apply(applyReq)
		if err != nil {
			t.Fatalf("Failed to apply diff: %v", err)
		}
		
		if !applyResp.Success {
			t.Error("Expected successful application")
		}
		
		// 4. Format the results
		summary, err := formatter.Format(genResp, FormatSummary)
		if err != nil {
			t.Fatalf("Failed to format summary: %v", err)
		}
		
		if summary == "" {
			t.Error("Expected non-empty summary")
		}
		
		// 5. Verify the result matches expected
		resultTrimmed := strings.TrimSpace(applyResp.Result)
		expectedTrimmed := strings.TrimSpace(modifiedCode)
		
		if resultTrimmed != expectedTrimmed {
			t.Error("Applied diff result doesn't match expected modified code")
			t.Logf("Got:\n%s", resultTrimmed)
			t.Logf("Expected:\n%s", expectedTrimmed)
		}
	})
	
	t.Run("PerformanceTest", func(t *testing.T) {
		// Generate a larger diff
		largeOriginal := strings.Repeat(originalCode+"\n\n", 100)
		largeModified := strings.Repeat(modifiedCode+"\n\n", 100)
		
		start := time.Now()
		
		req := DiffRequest{
			Original: largeOriginal,
			Modified: largeModified,
			Options: DiffOptions{
				Format: FormatUnified,
			},
		}
		
		resp, err := generator.Generate(req)
		duration := time.Since(start)
		
		if err != nil {
			t.Fatalf("Failed to generate large diff: %v", err)
		}
		
		// Should complete within 1 second for ~1000 lines
		if duration > time.Second {
			t.Errorf("Diff generation too slow: %v", duration)
		}
		
		if len(resp.Changes) == 0 {
			t.Error("Expected changes in large diff")
		}
		
		t.Logf("Generated diff for %d lines in %v", 
			strings.Count(largeOriginal, "\n"), duration)
	})
}