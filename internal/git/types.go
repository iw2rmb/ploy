package git

import "time"

// Repository represents a Git repository with validation capabilities
type Repository struct {
	Path         string
	URL          string
	Branch       string
	SHA          string
	IsClean      bool
	HasUntracked bool
	LastCommit   *Commit
	RemoteOrigin *Remote
}

// Commit represents a Git commit
type Commit struct {
	SHA       string
	Message   string
	Author    string
	Email     string
	Timestamp time.Time
	GPGSigned bool
}

// Remote represents a Git remote
type Remote struct {
	Name string
	URL  string
	Type string // push, fetch
}

// ValidationResult contains the results of repository validation
type ValidationResult struct {
	Valid          bool
	Warnings       []string
	Errors         []string
	SecurityIssues []string
	Suggestions    []string
}

// RepositoryInfo provides comprehensive repository information
type RepositoryInfo struct {
	Repository   *Repository
	Validation   *ValidationResult
	Contributors []string
	BranchCount  int
	TagCount     int
	CommitCount  int
	FirstCommit  time.Time
	LastActivity time.Time
	Languages    map[string]int64 // language -> lines of code
}
