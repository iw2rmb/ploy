package arf

import (
	"testing"
)

func TestGenerateAttemptPath(t *testing.T) {
	tests := []struct {
		name       string
		rootID     string
		parentPath string
		existing   []HealingAttempt
		expected   string
	}{
		{
			name:       "first root attempt",
			rootID:     "transform-123",
			parentPath: "",
			existing:   []HealingAttempt{},
			expected:   "1",
		},
		{
			name:       "second root attempt",
			rootID:     "transform-123",
			parentPath: "",
			existing: []HealingAttempt{
				{AttemptPath: "1"},
			},
			expected: "2",
		},
		{
			name:       "third root attempt with gap",
			rootID:     "transform-123",
			parentPath: "",
			existing: []HealingAttempt{
				{AttemptPath: "1"},
				{AttemptPath: "3"},
			},
			expected: "4",
		},
		{
			name:       "first child of parent 1",
			rootID:     "transform-123",
			parentPath: "1",
			existing: []HealingAttempt{
				{AttemptPath: "1", Children: []HealingAttempt{}},
			},
			expected: "1.1",
		},
		{
			name:       "second child of parent 1",
			rootID:     "transform-123",
			parentPath: "1",
			existing: []HealingAttempt{
				{
					AttemptPath: "1",
					Children: []HealingAttempt{
						{AttemptPath: "1.1"},
					},
				},
			},
			expected: "1.2",
		},
		{
			name:       "deep nesting - first child of 1.2",
			rootID:     "transform-123",
			parentPath: "1.2",
			existing: []HealingAttempt{
				{
					AttemptPath: "1",
					Children: []HealingAttempt{
						{AttemptPath: "1.1"},
						{AttemptPath: "1.2", Children: []HealingAttempt{}},
					},
				},
			},
			expected: "1.2.1",
		},
		{
			name:       "deep nesting - second child of 1.2.3",
			rootID:     "transform-123",
			parentPath: "1.2.3",
			existing: []HealingAttempt{
				{
					AttemptPath: "1",
					Children: []HealingAttempt{
						{
							AttemptPath: "1.2",
							Children: []HealingAttempt{
								{
									AttemptPath: "1.2.3",
									Children: []HealingAttempt{
										{AttemptPath: "1.2.3.1"},
									},
								},
							},
						},
					},
				},
			},
			expected: "1.2.3.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateAttemptPath(tt.rootID, tt.parentPath, tt.existing)
			if result != tt.expected {
				t.Errorf("GenerateAttemptPath() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetNextSiblingNumber(t *testing.T) {
	tests := []struct {
		name       string
		children   []HealingAttempt
		parentPath string
		expected   int
	}{
		{
			name:       "no children at root",
			children:   []HealingAttempt{},
			parentPath: "",
			expected:   1,
		},
		{
			name: "one child at root",
			children: []HealingAttempt{
				{AttemptPath: "1"},
			},
			parentPath: "",
			expected:   2,
		},
		{
			name: "multiple children at root",
			children: []HealingAttempt{
				{AttemptPath: "1"},
				{AttemptPath: "2"},
				{AttemptPath: "3"},
			},
			parentPath: "",
			expected:   4,
		},
		{
			name: "children under parent 1",
			children: []HealingAttempt{
				{
					AttemptPath: "1",
					Children: []HealingAttempt{
						{AttemptPath: "1.1"},
						{AttemptPath: "1.2"},
					},
				},
			},
			parentPath: "1",
			expected:   3,
		},
		{
			name: "no children under parent 2",
			children: []HealingAttempt{
				{AttemptPath: "1"},
				{AttemptPath: "2", Children: []HealingAttempt{}},
			},
			parentPath: "2",
			expected:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNextSiblingNumber(tt.children, tt.parentPath)
			if result != tt.expected {
				t.Errorf("GetNextSiblingNumber() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateAttemptPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid single digit",
			path:    "1",
			wantErr: false,
		},
		{
			name:    "valid two levels",
			path:    "1.2",
			wantErr: false,
		},
		{
			name:    "valid deep nesting",
			path:    "1.2.3.4.5",
			wantErr: false,
		},
		{
			name:    "valid with large numbers",
			path:    "10.25.999",
			wantErr: false,
		},
		{
			name:    "invalid - empty",
			path:    "",
			wantErr: true,
		},
		{
			name:    "invalid - starts with dot",
			path:    ".1",
			wantErr: true,
		},
		{
			name:    "invalid - ends with dot",
			path:    "1.",
			wantErr: true,
		},
		{
			name:    "invalid - double dots",
			path:    "1..2",
			wantErr: true,
		},
		{
			name:    "invalid - contains letters",
			path:    "1.a.2",
			wantErr: true,
		},
		{
			name:    "invalid - contains zero",
			path:    "1.0.2",
			wantErr: true,
		},
		{
			name:    "invalid - negative number",
			path:    "1.-2",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAttemptPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAttemptPath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetPathDepth(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{
			name:     "single level",
			path:     "1",
			expected: 1,
		},
		{
			name:     "two levels",
			path:     "1.2",
			expected: 2,
		},
		{
			name:     "three levels",
			path:     "1.2.3",
			expected: 3,
		},
		{
			name:     "deep nesting",
			path:     "1.2.3.4.5.6.7",
			expected: 7,
		},
		{
			name:     "empty path",
			path:     "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPathDepth(tt.path)
			if result != tt.expected {
				t.Errorf("GetPathDepth(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetParentPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "parent of root level",
			path:     "1",
			expected: "",
		},
		{
			name:     "parent of two-level path",
			path:     "1.2",
			expected: "1",
		},
		{
			name:     "parent of three-level path",
			path:     "1.2.3",
			expected: "1.2",
		},
		{
			name:     "parent of deep path",
			path:     "1.2.3.4.5",
			expected: "1.2.3.4",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetParentPath(tt.path)
			if result != tt.expected {
				t.Errorf("GetParentPath(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestFindAttemptByPath(t *testing.T) {
	// Create a test tree structure
	attempts := []HealingAttempt{
		{
			AttemptPath: "1",
			Children: []HealingAttempt{
				{
					AttemptPath: "1.1",
					Children:    []HealingAttempt{},
				},
				{
					AttemptPath: "1.2",
					Children: []HealingAttempt{
						{
							AttemptPath: "1.2.1",
							Children:    []HealingAttempt{},
						},
					},
				},
			},
		},
		{
			AttemptPath: "2",
			Children:    []HealingAttempt{},
		},
	}

	tests := []struct {
		name      string
		attempts  []HealingAttempt
		path      string
		wantFound bool
	}{
		{
			name:      "find root level attempt",
			attempts:  attempts,
			path:      "1",
			wantFound: true,
		},
		{
			name:      "find second level attempt",
			attempts:  attempts,
			path:      "1.1",
			wantFound: true,
		},
		{
			name:      "find third level attempt",
			attempts:  attempts,
			path:      "1.2.1",
			wantFound: true,
		},
		{
			name:      "not found - non-existent path",
			attempts:  attempts,
			path:      "3",
			wantFound: false,
		},
		{
			name:      "not found - non-existent nested path",
			attempts:  attempts,
			path:      "1.3",
			wantFound: false,
		},
		{
			name:      "empty attempts",
			attempts:  []HealingAttempt{},
			path:      "1",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FindAttemptByPath(tt.attempts, tt.path)
			if (result != nil) != tt.wantFound {
				t.Errorf("FindAttemptByPath() found = %v, wantFound %v", result != nil, tt.wantFound)
			}
			if result != nil && result.AttemptPath != tt.path {
				t.Errorf("FindAttemptByPath() returned wrong attempt: got %s, want %s", result.AttemptPath, tt.path)
			}
		})
	}
}
