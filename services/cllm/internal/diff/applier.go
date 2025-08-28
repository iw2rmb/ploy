package diff

import (
	"fmt"
	"strings"
	"time"
)

// Applier applies diffs to target content
type Applier struct {
	parser *Parser
}

// NewApplier creates a new diff applier
func NewApplier() *Applier {
	return &Applier{
		parser: NewParser(),
	}
}

// Apply applies a diff to target content
func (a *Applier) Apply(req ApplyRequest) (*ApplyResponse, error) {
	startTime := time.Now()
	
	// Set default options
	options := req.Options
	if options.FuzzFactor == 0 {
		options.FuzzFactor = 2 // Default fuzz factor
	}
	
	// Parse the diff
	parseReq := ParseRequest{
		Content:  req.Diff,
		Format:   FormatUnified,
		Validate: true,
	}
	
	parseResp, err := a.parser.Parse(parseReq)
	if err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}
	
	// Check validation results
	if parseResp.Validation != nil && !parseResp.Validation.Valid {
		if len(parseResp.Validation.Errors) > 0 {
			return nil, &ErrInvalidDiff{
				Message: parseResp.Validation.Errors[0].Message,
			}
		}
	}
	
	// Apply the diff
	result := req.Target
	var conflicts []Conflict
	appliedHunks := 0
	failedHunks := 0
	totalHunks := 0
	
	// For now, we'll handle single file diffs
	// In a real implementation, you'd handle multiple files
	if len(parseResp.Changes) > 0 {
		change := parseResp.Changes[0]
		targetLines := strings.Split(result, "\n")
		
		// Apply in reverse order if requested
		hunks := change.Hunks
		if options.Reverse {
			hunks = a.reverseHunks(hunks)
		}
		
		// Apply each hunk
		for hunkIdx, hunk := range hunks {
			totalHunks++
			
			applied, newLines, conflict := a.applyHunk(targetLines, hunk, options)
			if applied {
				targetLines = newLines
				appliedHunks++
			} else {
				failedHunks++
				if conflict != nil {
					conflict.File = change.OriginalPath
					conflict.HunkIndex = hunkIdx
					conflicts = append(conflicts, *conflict)
				}
				
				// In strict mode, fail on first conflict
				if options.Strict {
					break
				}
			}
		}
		
		result = strings.Join(targetLines, "\n")
	}
	
	response := &ApplyResponse{
		Result:       result,
		Success:      failedHunks == 0,
		AppliedHunks: appliedHunks,
		FailedHunks:  failedHunks,
		Conflicts:    conflicts,
		Report: ApplyReport{
			FilesProcessed: 1,
			FilesModified:  1,
			TotalHunks:     totalHunks,
			ProcessingTime: time.Since(startTime),
		},
	}
	
	return response, nil
}

// applyHunk applies a single hunk to target lines
func (a *Applier) applyHunk(targetLines []string, hunk Hunk, options ApplyOptions) (bool, []string, *Conflict) {
	// Find where to apply the hunk
	startLine := hunk.OldStart - 1 // Convert to 0-indexed
	
	if options.Fuzzy {
		// Try to find the best match with fuzzy matching
		bestMatch := a.findBestMatch(targetLines, hunk, startLine, options.FuzzFactor)
		if bestMatch >= 0 {
			startLine = bestMatch
		} else {
			return false, targetLines, &Conflict{
				Reason: "could not find matching context",
			}
		}
	}
	
	// Verify context matches
	if !a.verifyContext(targetLines, hunk, startLine) && !options.Fuzzy {
		expected, actual := a.getContextMismatch(targetLines, hunk, startLine)
		return false, targetLines, &Conflict{
			Reason:   "context mismatch",
			Expected: expected,
			Actual:   actual,
		}
	}
	
	// Apply the hunk
	newLines := make([]string, 0, len(targetLines))
	
	// Copy lines before the hunk
	newLines = append(newLines, targetLines[:startLine]...)
	
	// Apply hunk changes
	targetIdx := startLine
	for _, line := range hunk.Lines {
		switch line.Type {
		case LineTypeContext:
			// Context line - copy from target
			if targetIdx < len(targetLines) {
				newLines = append(newLines, targetLines[targetIdx])
				targetIdx++
			}
		case LineTypeDelete:
			// Delete line - skip in target
			targetIdx++
		case LineTypeAdd:
			// Add line
			newLines = append(newLines, line.Content)
		}
	}
	
	// Copy remaining lines after the hunk
	if targetIdx < len(targetLines) {
		newLines = append(newLines, targetLines[targetIdx:]...)
	}
	
	return true, newLines, nil
}

// findBestMatch finds the best matching location for a hunk using fuzzy matching
func (a *Applier) findBestMatch(targetLines []string, hunk Hunk, preferredStart int, fuzzFactor int) int {
	// Extract context lines from hunk
	var contextLines []string
	for _, line := range hunk.Lines {
		if line.Type == LineTypeContext || line.Type == LineTypeDelete {
			contextLines = append(contextLines, line.Content)
		}
	}
	
	if len(contextLines) == 0 {
		return preferredStart // No context to match
	}
	
	// Search within fuzz range
	minLine := max(0, preferredStart-fuzzFactor)
	maxLine := min(len(targetLines)-len(contextLines), preferredStart+fuzzFactor)
	
	bestMatch := -1
	bestScore := 0.0
	
	for i := minLine; i <= maxLine; i++ {
		score := a.matchScore(targetLines, contextLines, i)
		if score > bestScore {
			bestScore = score
			bestMatch = i
		}
	}
	
	// Require at least 80% match
	if bestScore >= 0.8 {
		return bestMatch
	}
	
	return -1
}

// matchScore calculates how well context lines match target lines at a given position
func (a *Applier) matchScore(targetLines []string, contextLines []string, startPos int) float64 {
	matches := 0
	total := len(contextLines)
	
	for i, contextLine := range contextLines {
		targetIdx := startPos + i
		if targetIdx >= len(targetLines) {
			break
		}
		
		if targetLines[targetIdx] == contextLine {
			matches++
		} else if a.fuzzyMatch(targetLines[targetIdx], contextLine) {
			matches++ // Count fuzzy matches
		}
	}
	
	return float64(matches) / float64(total)
}

// fuzzyMatch performs fuzzy string matching
func (a *Applier) fuzzyMatch(s1, s2 string) bool {
	// Simple fuzzy match - ignoring whitespace differences
	s1 = strings.TrimSpace(s1)
	s2 = strings.TrimSpace(s2)
	
	// Check if strings are similar after removing whitespace
	s1NoSpace := strings.ReplaceAll(s1, " ", "")
	s2NoSpace := strings.ReplaceAll(s2, " ", "")
	
	return s1NoSpace == s2NoSpace
}

// verifyContext verifies that the context lines match
func (a *Applier) verifyContext(targetLines []string, hunk Hunk, startLine int) bool {
	targetIdx := startLine
	
	for _, line := range hunk.Lines {
		if line.Type == LineTypeContext || line.Type == LineTypeDelete {
			if targetIdx >= len(targetLines) {
				return false
			}
			if targetLines[targetIdx] != line.Content {
				return false
			}
			targetIdx++
		}
	}
	
	return true
}

// getContextMismatch returns the expected vs actual context for error reporting
func (a *Applier) getContextMismatch(targetLines []string, hunk Hunk, startLine int) (string, string) {
	var expected, actual strings.Builder
	targetIdx := startLine
	
	for _, line := range hunk.Lines {
		if line.Type == LineTypeContext || line.Type == LineTypeDelete {
			expected.WriteString(line.Content + "\n")
			if targetIdx < len(targetLines) {
				actual.WriteString(targetLines[targetIdx] + "\n")
				targetIdx++
			} else {
				actual.WriteString("<EOF>\n")
			}
		}
	}
	
	return expected.String(), actual.String()
}

// reverseHunks reverses the effect of hunks (for unapply)
func (a *Applier) reverseHunks(hunks []Hunk) []Hunk {
	reversed := make([]Hunk, len(hunks))
	
	for i, hunk := range hunks {
		reversedHunk := Hunk{
			OldStart: hunk.NewStart,
			OldLines: hunk.NewLines,
			NewStart: hunk.OldStart,
			NewLines: hunk.OldLines,
			Header:   hunk.Header,
		}
		
		// Reverse the line types
		for _, line := range hunk.Lines {
			reversedLine := DiffLine{
				Content: line.Content,
			}
			
			switch line.Type {
			case LineTypeAdd:
				// Add becomes delete
				reversedLine.Type = LineTypeDelete
				reversedLine.OldNumber = line.NewNumber
			case LineTypeDelete:
				// Delete becomes add
				reversedLine.Type = LineTypeAdd
				reversedLine.NewNumber = line.OldNumber
			case LineTypeContext:
				// Context stays context
				reversedLine.Type = LineTypeContext
				reversedLine.OldNumber = line.NewNumber
				reversedLine.NewNumber = line.OldNumber
			case LineTypeNoNewline:
				reversedLine.Type = LineTypeNoNewline
			}
			
			reversedHunk.Lines = append(reversedHunk.Lines, reversedLine)
		}
		
		reversed[i] = reversedHunk
	}
	
	return reversed
}

// ApplyMultiple applies multiple diffs in sequence
func (a *Applier) ApplyMultiple(diffs []string, target string, options ApplyOptions) (*ApplyResponse, error) {
	result := target
	totalApplied := 0
	totalFailed := 0
	var allConflicts []Conflict
	
	for i, diff := range diffs {
		req := ApplyRequest{
			Diff:    diff,
			Target:  result,
			Options: options,
		}
		
		resp, err := a.Apply(req)
		if err != nil {
			return nil, fmt.Errorf("failed to apply diff %d: %w", i+1, err)
		}
		
		result = resp.Result
		totalApplied += resp.AppliedHunks
		totalFailed += resp.FailedHunks
		
		// Prefix conflicts with diff index
		for _, conflict := range resp.Conflicts {
			conflict.File = fmt.Sprintf("diff-%d/%s", i+1, conflict.File)
			allConflicts = append(allConflicts, conflict)
		}
		
		// Stop on failure in strict mode
		if options.Strict && !resp.Success {
			break
		}
	}
	
	return &ApplyResponse{
		Result:       result,
		Success:      totalFailed == 0,
		AppliedHunks: totalApplied,
		FailedHunks:  totalFailed,
		Conflicts:    allConflicts,
		Report: ApplyReport{
			FilesProcessed: len(diffs),
			FilesModified:  len(diffs) - totalFailed,
		},
	}, nil
}