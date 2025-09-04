package models

import "time"

type TransformationError struct {
    Message string `json:"message"`
    File    string `json:"file,omitempty"`
    Line    int    `json:"line,omitempty"`
    Column  int    `json:"column,omitempty"`
}

type TransformationResult struct {
    Success         bool          `json:"success"`
    ChangesApplied  int           `json:"changes_applied"`
    FilesModified   []string      `json:"files_modified"`
    ExecutionTime   time.Duration `json:"execution_time"`
    ValidationScore float64       `json:"validation_score"`
    Errors          []TransformationError `json:"errors,omitempty"`
    Warnings        []TransformationError `json:"warnings,omitempty"`
}

