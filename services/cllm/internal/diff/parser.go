package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Parser parses diff content into structured format
type Parser struct {
	securityPatterns []*regexp.Regexp
}

// NewParser creates a new diff parser
func NewParser() *Parser {
	return &Parser{
		securityPatterns: initSecurityPatterns(),
	}
}

// initSecurityPatterns initializes security check patterns
func initSecurityPatterns() []*regexp.Regexp {
	patterns := []string{
		// Path traversal attempts
		`\.\.\/|\.\.\\`,
		// Absolute paths that might escape sandbox
		`^\/etc\/|^\/usr\/|^\/bin\/|^\/sbin\/|^\/root\/`,
		// Shell injection attempts
		`\$\(.*\)|` + "`" + `.*` + "`",
		// Common sensitive files
		`\.ssh\/|\.aws\/|\.env|private_key|secret`,
	}
	
	var compiled []*regexp.Regexp
	for _, pattern := range patterns {
		if re, err := regexp.Compile(pattern); err == nil {
			compiled = append(compiled, re)
		}
	}
	return compiled
}

// Parse parses a diff string into structured format
func (p *Parser) Parse(req ParseRequest) (*ParseResponse, error) {
	// Default to unified format if not specified
	if req.Format == "" {
		req.Format = FormatUnified
	}
	
	var changes []FileChange
	var warnings []string
	var errors []ValidationError
	var securityIssues []SecurityIssue
	
	switch req.Format {
	case FormatUnified:
		var err error
		changes, warnings, err = p.parseUnifiedDiff(req.Content)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported diff format: %s", req.Format)
	}
	
	// Perform validation if requested
	if req.Validate {
		validationErrors := p.validateChanges(changes)
		errors = append(errors, validationErrors...)
	}
	
	// Perform security check if requested
	if req.SecurityCheck {
		securityIssues = p.checkSecurity(req.Content, changes)
	}
	
	// Calculate statistics
	stats := p.calculateStats(changes)
	
	response := &ParseResponse{
		Changes:  changes,
		Stats:    stats,
		Warnings: warnings,
	}
	
	// Add validation results if performed
	if req.Validate || req.SecurityCheck {
		response.Validation = &ValidationResult{
			Valid:          len(errors) == 0 && len(securityIssues) == 0,
			Errors:         errors,
			SecurityIssues: securityIssues,
		}
	}
	
	return response, nil
}

// parseUnifiedDiff parses a unified diff format
func (p *Parser) parseUnifiedDiff(content string) ([]FileChange, []string, error) {
	lines := strings.Split(content, "\n")
	var changes []FileChange
	var warnings []string
	
	i := 0
	for i < len(lines) {
		// Skip empty lines
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}
		
		// Look for file header
		if strings.HasPrefix(lines[i], "---") {
			change, newIndex, warns := p.parseFileChange(lines, i)
			if change != nil {
				changes = append(changes, *change)
			}
			warnings = append(warnings, warns...)
			i = newIndex
		} else if strings.HasPrefix(lines[i], "diff") {
			// Git diff header - skip to actual diff
			i++
		} else if strings.HasPrefix(lines[i], "index") || 
				  strings.HasPrefix(lines[i], "new file") ||
				  strings.HasPrefix(lines[i], "deleted file") {
			// Git metadata - skip
			i++
		} else {
			// Unknown line - skip with warning
			if lines[i] != "" {
				warnings = append(warnings, fmt.Sprintf("skipping unknown line: %s", lines[i]))
			}
			i++
		}
	}
	
	return changes, warnings, nil
}

// parseFileChange parses a single file change
func (p *Parser) parseFileChange(lines []string, startIndex int) (*FileChange, int, []string) {
	var warnings []string
	i := startIndex
	
	// Parse --- line
	if !strings.HasPrefix(lines[i], "---") {
		return nil, i+1, []string{"expected --- line"}
	}
	originalPath := strings.TrimSpace(strings.TrimPrefix(lines[i], "---"))
	i++
	
	// Parse +++ line
	if i >= len(lines) || !strings.HasPrefix(lines[i], "+++") {
		return nil, i, []string{"expected +++ line after ---"}
	}
	modifiedPath := strings.TrimSpace(strings.TrimPrefix(lines[i], "+++"))
	i++
	
	// Parse hunks
	var hunks []Hunk
	for i < len(lines) && strings.HasPrefix(lines[i], "@@") {
		hunk, newIndex, warns := p.parseHunk(lines, i)
		if hunk != nil {
			hunks = append(hunks, *hunk)
		}
		warnings = append(warnings, warns...)
		i = newIndex
	}
	
	// Determine change type
	changeType := ChangeTypeModify
	if originalPath == "/dev/null" {
		changeType = ChangeTypeAdd
	} else if modifiedPath == "/dev/null" {
		changeType = ChangeTypeDelete
	}
	
	change := &FileChange{
		OriginalPath: originalPath,
		ModifiedPath: modifiedPath,
		ChangeType:   changeType,
		Hunks:        hunks,
		Stats:        p.calculateFileStats(hunks),
	}
	
	return change, i, warnings
}

// parseHunk parses a single hunk
func (p *Parser) parseHunk(lines []string, startIndex int) (*Hunk, int, []string) {
	var warnings []string
	i := startIndex
	
	// Parse hunk header
	hunkHeader := lines[i]
	if !strings.HasPrefix(hunkHeader, "@@") {
		return nil, i+1, []string{"expected @@ hunk header"}
	}
	
	// Extract line numbers from header
	// Format: @@ -oldStart,oldCount +newStart,newCount @@ optional header
	re := regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)`)
	matches := re.FindStringSubmatch(hunkHeader)
	if len(matches) < 5 {
		return nil, i+1, []string{"invalid hunk header format"}
	}
	
	oldStart, _ := strconv.Atoi(matches[1])
	oldLines := 1
	if matches[2] != "" {
		oldLines, _ = strconv.Atoi(matches[2])
	}
	
	newStart, _ := strconv.Atoi(matches[3])
	newLines := 1
	if matches[4] != "" {
		newLines, _ = strconv.Atoi(matches[4])
	}
	
	header := ""
	if len(matches) > 5 {
		header = strings.TrimSpace(matches[5])
	}
	
	i++
	
	// Parse hunk lines
	var hunkLines []DiffLine
	oldLineNum := oldStart
	newLineNum := newStart
	
	for i < len(lines) {
		line := lines[i]
		
		// Check if we've reached the next hunk or file
		if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "---") || 
		   strings.HasPrefix(line, "diff") {
			break
		}
		
		if len(line) == 0 {
			// Empty line - treat as context
			hunkLines = append(hunkLines, DiffLine{
				Type:      LineTypeContext,
				Content:   "",
				OldNumber: &oldLineNum,
				NewNumber: &newLineNum,
			})
			oldLineNum++
			newLineNum++
		} else {
			prefix := line[0:1]
			content := ""
			if len(line) > 1 {
				content = line[1:]
			}
			
			switch prefix {
			case " ":
				// Context line
				hunkLines = append(hunkLines, DiffLine{
					Type:      LineTypeContext,
					Content:   content,
					OldNumber: &oldLineNum,
					NewNumber: &newLineNum,
				})
				oldLineNum++
				newLineNum++
			case "+":
				// Added line
				hunkLines = append(hunkLines, DiffLine{
					Type:      LineTypeAdd,
					Content:   content,
					NewNumber: &newLineNum,
				})
				newLineNum++
			case "-":
				// Deleted line
				hunkLines = append(hunkLines, DiffLine{
					Type:      LineTypeDelete,
					Content:   content,
					OldNumber: &oldLineNum,
				})
				oldLineNum++
			case "\\":
				// No newline at end of file
				hunkLines = append(hunkLines, DiffLine{
					Type:    LineTypeNoNewline,
					Content: line,
				})
			default:
				warnings = append(warnings, fmt.Sprintf("unknown line prefix: %s", prefix))
			}
		}
		
		i++
	}
	
	hunk := &Hunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Header:   header,
		Lines:    hunkLines,
	}
	
	return hunk, i, warnings
}

// validateChanges validates the parsed changes
func (p *Parser) validateChanges(changes []FileChange) []ValidationError {
	var errors []ValidationError
	
	for _, change := range changes {
		// Validate file paths
		if change.OriginalPath == "" && change.ModifiedPath == "" {
			errors = append(errors, ValidationError{
				Type:    "missing_path",
				Message: "both original and modified paths are empty",
			})
		}
		
		// Validate hunks
		for hunkIdx, hunk := range change.Hunks {
			// Check line counts
			actualOldLines := 0
			actualNewLines := 0
			
			for _, line := range hunk.Lines {
				switch line.Type {
				case LineTypeContext:
					actualOldLines++
					actualNewLines++
				case LineTypeDelete:
					actualOldLines++
				case LineTypeAdd:
					actualNewLines++
				}
			}
			
			if actualOldLines != hunk.OldLines {
				errors = append(errors, ValidationError{
					Type:    "line_count_mismatch",
					Message: fmt.Sprintf("hunk %d: expected %d old lines, got %d", hunkIdx, hunk.OldLines, actualOldLines),
					File:    change.OriginalPath,
				})
			}
			
			if actualNewLines != hunk.NewLines {
				errors = append(errors, ValidationError{
					Type:    "line_count_mismatch",
					Message: fmt.Sprintf("hunk %d: expected %d new lines, got %d", hunkIdx, hunk.NewLines, actualNewLines),
					File:    change.ModifiedPath,
				})
			}
		}
	}
	
	return errors
}

// checkSecurity checks for security issues in the diff
func (p *Parser) checkSecurity(content string, changes []FileChange) []SecurityIssue {
	var issues []SecurityIssue
	
	// Check for suspicious patterns in the raw content
	for _, pattern := range p.securityPatterns {
		if matches := pattern.FindAllString(content, -1); len(matches) > 0 {
			issues = append(issues, SecurityIssue{
				Severity:    SeverityMedium,
				Type:        "suspicious_pattern",
				Description: fmt.Sprintf("suspicious pattern detected: %s", matches[0]),
			})
		}
	}
	
	// Check file paths
	for _, change := range changes {
		// Check for path traversal
		if strings.Contains(change.OriginalPath, "..") || strings.Contains(change.ModifiedPath, "..") {
			issues = append(issues, SecurityIssue{
				Severity:    SeverityHigh,
				Type:        "path_traversal",
				Description: "potential path traversal in file path",
				File:        change.OriginalPath,
			})
		}
		
		// Check for sensitive files
		sensitivePatterns := []string{".ssh", ".env", "private", "secret", "password", "token", "key"}
		for _, pattern := range sensitivePatterns {
			if strings.Contains(strings.ToLower(change.OriginalPath), pattern) ||
			   strings.Contains(strings.ToLower(change.ModifiedPath), pattern) {
				issues = append(issues, SecurityIssue{
					Severity:    SeverityMedium,
					Type:        "sensitive_file",
					Description: fmt.Sprintf("modification to potentially sensitive file containing '%s'", pattern),
					File:        change.OriginalPath,
				})
				break
			}
		}
		
		// Check for binary file manipulation
		if change.IsBinary {
			issues = append(issues, SecurityIssue{
				Severity:    SeverityLow,
				Type:        "binary_file",
				Description: "binary file modification detected",
				File:        change.OriginalPath,
			})
		}
	}
	
	return issues
}

// calculateStats calculates statistics from changes
func (p *Parser) calculateStats(changes []FileChange) DiffStats {
	stats := DiffStats{}
	
	for _, change := range changes {
		switch change.ChangeType {
		case ChangeTypeAdd:
			stats.FilesAdded++
		case ChangeTypeDelete:
			stats.FilesDeleted++
		case ChangeTypeModify:
			stats.FilesChanged++
		}
		
		stats.LinesAdded += change.Stats.LinesAdded
		stats.LinesDeleted += change.Stats.LinesDeleted
		stats.LinesChanged += change.Stats.LinesChanged
		
		if change.IsBinary {
			stats.BinaryFiles++
		}
	}
	
	return stats
}

// calculateFileStats calculates statistics for a single file
func (p *Parser) calculateFileStats(hunks []Hunk) FileStats {
	stats := FileStats{}
	
	for _, hunk := range hunks {
		for _, line := range hunk.Lines {
			switch line.Type {
			case LineTypeAdd:
				stats.LinesAdded++
			case LineTypeDelete:
				stats.LinesDeleted++
			}
		}
	}
	
	// Changed lines are the minimum of added and deleted
	stats.LinesChanged = min(stats.LinesAdded, stats.LinesDeleted)
	
	return stats
}