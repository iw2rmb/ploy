package builders

import (
	"testing"
)

// TestBytesTrimSpace tests the bytesTrimSpace utility function
func TestBytesTrimSpace(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "no whitespace",
			input:    []byte("hello"),
			expected: "hello",
		},
		{
			name:     "leading spaces",
			input:    []byte("   hello"),
			expected: "hello",
		},
		{
			name:     "trailing spaces",
			input:    []byte("hello   "),
			expected: "hello",
		},
		{
			name:     "both leading and trailing spaces",
			input:    []byte("   hello   "),
			expected: "hello",
		},
		{
			name:     "newlines at start",
			input:    []byte("\n\nhello"),
			expected: "hello",
		},
		{
			name:     "newlines at end",
			input:    []byte("hello\n\n"),
			expected: "hello",
		},
		{
			name:     "mixed whitespace at both ends",
			input:    []byte("  \n\r hello\r\n  "),
			expected: "hello",
		},
		{
			name:     "carriage returns",
			input:    []byte("\r\rhello\r\r"),
			expected: "hello",
		},
		{
			name:     "whitespace in middle preserved",
			input:    []byte("  hello world  "),
			expected: "hello world",
		},
		{
			name:     "multiple lines with indentation",
			input:    []byte("  \nline1\nline2\n  "),
			expected: "line1\nline2",
		},
		{
			name:     "empty input",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    []byte("   \n\r  "),
			expected: "",
		},
		{
			name:     "single character",
			input:    []byte("a"),
			expected: "a",
		},
		{
			name:     "single space",
			input:    []byte(" "),
			expected: "",
		},
		{
			name:     "complex output with paths",
			input:    []byte("  \n/tmp/output/file.tar.gz\n  "),
			expected: "/tmp/output/file.tar.gz",
		},
		{
			name:     "Windows-style line endings",
			input:    []byte("\r\noutput.exe\r\n"),
			expected: "output.exe",
		},
		{
			name:     "tabs are not trimmed",
			input:    []byte("\thello\t"),
			expected: "\thello\t",
		},
		{
			name:     "mixed with tabs",
			input:    []byte("  \thello\t  "),
			expected: "\thello\t",
		},
		{
			name:     "very long string",
			input:    []byte("  " + string(make([]byte, 1000)) + "content" + string(make([]byte, 1000)) + "  "),
			expected: string(make([]byte, 1000)) + "content" + string(make([]byte, 1000)),
		},
		{
			name:     "unicode spaces not trimmed",
			input:    []byte("\u00A0hello\u00A0"), // non-breaking space
			expected: "\u00A0hello\u00A0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := bytesTrimSpace(tt.input)
			if result != tt.expected {
				t.Errorf("bytesTrimSpace(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestBytesTrimSpaceEdgeCases tests edge cases and boundary conditions
func TestBytesTrimSpaceEdgeCases(t *testing.T) {
	// Test with nil input (would panic if not handled)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("bytesTrimSpace panicked on nil input: %v", r)
		}
	}()

	// This should not panic
	result := bytesTrimSpace(nil)
	if result != "" {
		t.Errorf("bytesTrimSpace(nil) = %q, want empty string", result)
	}

	// Test with very large input
	largeInput := make([]byte, 10000)
	for i := range largeInput {
		if i < 100 || i >= 9900 {
			largeInput[i] = ' '
		} else {
			largeInput[i] = 'x'
		}
	}

	result = bytesTrimSpace(largeInput)
	expectedLen := 9900 - 100
	if len(result) != expectedLen {
		t.Errorf("bytesTrimSpace with large input: got length %d, want %d", len(result), expectedLen)
	}
}

// BenchmarkBytesTrimSpace benchmarks the bytesTrimSpace function
func BenchmarkBytesTrimSpace(b *testing.B) {
	testCases := []struct {
		name  string
		input []byte
	}{
		{"small_no_trim", []byte("hello")},
		{"small_trim", []byte("  hello  ")},
		{"medium_no_trim", []byte(string(make([]byte, 100)) + "content")},
		{"medium_trim", []byte("  " + string(make([]byte, 100)) + "content  ")},
		{"large_no_trim", []byte(string(make([]byte, 1000)) + "content")},
		{"large_trim", []byte("  \n\r" + string(make([]byte, 1000)) + "content\r\n  ")},
		{"only_spaces", []byte("     ")},
		{"mixed_whitespace", []byte(" \n\r \n\r ")},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = bytesTrimSpace(tc.input)
			}
		})
	}
}

// TestBytesTrimSpaceConsistency verifies consistency with expected behavior
func TestBytesTrimSpaceConsistency(t *testing.T) {
	// Test that the function is idempotent
	input := []byte("  hello  ")
	first := bytesTrimSpace(input)
	second := bytesTrimSpace([]byte(first))

	if first != second {
		t.Errorf("bytesTrimSpace is not idempotent: first=%q, second=%q", first, second)
	}

	// Test that it only trims specific characters (space, \n, \r)
	specialChars := []struct {
		char       byte
		shouldTrim bool
	}{
		{' ', true},
		{'\n', true},
		{'\r', true},
		{'\t', false},
		{'\v', false},
		{'\f', false},
		{'0', false},
		{'a', false},
	}

	for _, sc := range specialChars {
		input := []byte{sc.char, 'x', sc.char}
		result := bytesTrimSpace(input)

		if sc.shouldTrim {
			if result != "x" {
				t.Errorf("bytesTrimSpace should trim %q: got %q, want 'x'", sc.char, result)
			}
		} else {
			expected := string([]byte{sc.char, 'x', sc.char})
			if result != expected {
				t.Errorf("bytesTrimSpace should not trim %q: got %q, want %q", sc.char, result, expected)
			}
		}
	}
}
