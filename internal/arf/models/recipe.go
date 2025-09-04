package models

import "time"

// Time wraps time.Time for JSON mapping compatibility
type Time time.Time

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
    Tags        []string `json:"tags,omitempty"`
    Categories  []string `json:"categories,omitempty"`
    Languages   []string `json:"languages,omitempty"`
}

// RecipeStep minimal subset used by CLI for counts/names/types
type RecipeStep struct {
    Name string `json:"name"`
    Type string `json:"type"`
}

// Recipe minimal structure to match API responses used by CLI
type Recipe struct {
    ID        string         `json:"id"`
    Metadata  RecipeMetadata `json:"metadata"`
    Steps     []RecipeStep   `json:"steps"`
    CreatedAt Time           `json:"created_at"`
}
