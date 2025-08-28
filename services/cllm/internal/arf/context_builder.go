package arf

import (
	"sort"
	"strings"
)

// ContextBuilder manages token-budgeted context construction
type ContextBuilder struct {
	maxTokens int
	sections  []ContextSection
}

// ContextSection represents a section of the analysis context
type ContextSection struct {
	name     string
	priority int
	tokens   int
	content  func() string
}

// NewContextBuilder creates a new context builder with token budget
func NewContextBuilder(maxTokens int) *ContextBuilder {
	return &ContextBuilder{
		maxTokens: maxTokens,
		sections:  make([]ContextSection, 0),
	}
}

// AddSection adds a section to the context with priority and token budget
func (cb *ContextBuilder) AddSection(name string, maxTokensForSection int, contentFunc func() string) {
	cb.sections = append(cb.sections, ContextSection{
		name:     name,
		priority: len(cb.sections), // Lower number = higher priority
		tokens:   maxTokensForSection,
		content:  contentFunc,
	})
}

// Build constructs the final context within token limits
func (cb *ContextBuilder) Build() string {
	var result strings.Builder
	totalTokens := 0
	
	// Sort sections by priority (lower number = higher priority)
	sort.Slice(cb.sections, func(i, j int) bool {
		return cb.sections[i].priority < cb.sections[j].priority
	})
	
	// Add sections until we hit token budget
	for _, section := range cb.sections {
		if totalTokens >= cb.maxTokens {
			break
		}
		
		content := section.content()
		if content == "" {
			continue
		}
		
		// Estimate tokens (rough approximation: 1 token ≈ 4 characters)
		estimatedTokens := len(content) / 4
		
		// If section would exceed remaining budget, truncate it
		remainingTokens := cb.maxTokens - totalTokens
		if estimatedTokens > remainingTokens && remainingTokens > 50 {
			content = cb.truncateContent(content, remainingTokens*4) // Convert back to characters
		}
		
		if estimatedTokens > section.tokens {
			content = cb.truncateContent(content, section.tokens*4)
		}
		
		result.WriteString(content)
		totalTokens += min(estimatedTokens, section.tokens)
	}
	
	return result.String()
}

// truncateContent truncates content to fit within character limit
func (cb *ContextBuilder) truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	
	// Try to truncate at word boundaries
	truncated := content[:maxChars]
	lastSpace := strings.LastIndex(truncated, " ")
	lastNewline := strings.LastIndex(truncated, "\n")
	
	cutPoint := max(lastSpace, lastNewline)
	if cutPoint > maxChars/2 { // Only use word boundary if it's not too short
		truncated = content[:cutPoint]
	}
	
	return truncated + "\n... (truncated for token limit)"
}