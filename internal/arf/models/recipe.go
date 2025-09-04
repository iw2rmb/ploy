package models

import "time"

// Time wraps time.Time for JSON mapping compatibility
type Time time.Time

// Format proxies to time.Time formatting
func (t Time) Format(layout string) string { return time.Time(t).Format(layout) }

// Before compares underlying time values
func (t Time) Before(other Time) bool { return time.Time(t).Before(time.Time(other)) }

func (t *Time) UnmarshalJSON(data []byte) error {
    // Expect RFC3339 string
    var s string
    if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
        s = string(data[1:len(data)-1])
    } else {
        s = string(data)
    }
    if s == "" || s == "null" {
        *t = Time{}
        return nil
    }
    parsed, err := time.Parse(time.RFC3339, s)
    if err != nil {
        return err
    }
    *t = Time(parsed)
    return nil
}

// RecipeMetadata contains human-readable recipe info (subset for CLI)
type RecipeMetadata struct {
    Name        string   `json:"name"`
    Description string   `json:"description"`
    Author      string   `json:"author"`
    Version     string   `json:"version,omitempty"`
    License     string   `json:"license,omitempty"`
    Tags        []string `json:"tags,omitempty"`
    Categories  []string `json:"categories,omitempty"`
    Languages   []string `json:"languages,omitempty"`
    MinPlatform string   `json:"min_platform,omitempty"`
    MaxPlatform string   `json:"max_platform,omitempty"`
}

// RecipeStep minimal subset used by CLI for counts/names/types
type RecipeStep struct {
    Name string `json:"name"`
    Type string `json:"type"`
    Config map[string]interface{} `json:"config,omitempty"`
    Timeout Duration `json:"timeout,omitempty"`
}

// Recipe minimal structure to match API responses used by CLI
type Recipe struct {
    ID        string         `json:"id"`
    Metadata  RecipeMetadata `json:"metadata"`
    Steps     []RecipeStep   `json:"steps"`
    CreatedAt Time           `json:"created_at"`
    UpdatedAt Time           `json:"updated_at"`
    UploadedBy string        `json:"uploaded_by,omitempty"`
    Hash       string        `json:"hash,omitempty"`
    Execution  ExecutionConfig `json:"execution,omitempty"`
}

// Duration mirrors common duration wrapper used by API models
type Duration struct { Duration time.Duration }

func (d *Duration) UnmarshalJSON(data []byte) error {
    var s string
    if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
        s = string(data[1:len(data)-1])
    } else {
        s = string(data)
    }
    if s == "" || s == "null" {
        d.Duration = 0
        return nil
    }
    dur, err := time.ParseDuration(s)
    if err != nil { return err }
    d.Duration = dur
    return nil
}

// Validate provides a minimal no-op validation to satisfy CLI calls
func (r *Recipe) Validate() error { return nil }

// SetSystemFields minimal setter used by CLI export paths
func (r *Recipe) SetSystemFields(uploadedBy string) {
    if r.Metadata.Version == "" { r.Metadata.Version = "1.0.0" }
    r.UploadedBy = uploadedBy
    if r.ID == "" && r.Metadata.Name != "" {
        r.ID = r.Metadata.Name + "-" + r.Metadata.Version
    }
    if time.Time(r.CreatedAt).IsZero() {
        r.CreatedAt = Time(time.Now())
    }
    r.UpdatedAt = Time(time.Now())
}

// ExecutionConfig (minimal) matches fields used by CLI templates
type ExecutionConfig struct {
    MaxDuration Duration          `json:"max_duration,omitempty" yaml:"max_duration,omitempty"`
    Sandbox     SandboxConfig     `json:"sandbox,omitempty" yaml:"sandbox,omitempty"`
    Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
}

// SandboxConfig (minimal) matches fields used by CLI templates
type SandboxConfig struct {
    Enabled   bool   `json:"enabled" yaml:"enabled"`
    MaxMemory string `json:"max_memory,omitempty" yaml:"max_memory,omitempty"`
}
