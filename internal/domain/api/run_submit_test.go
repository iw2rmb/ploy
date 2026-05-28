package api

import (
	"encoding/json"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestRunSubmitPayload verifies JSON field names and roundtrip stability for RunSubmitRequest.
func TestRunSubmitPayload(t *testing.T) {
	t.Parallel()

	t.Run("roundtrip_full", func(t *testing.T) {
		t.Parallel()
		in := RunSubmitRequest{
			RepoURL:   domaintypes.RepoURL("https://github.com/example/repo.git"),
			Ref:       domaintypes.GitRef("main"),
			Spec:      json.RawMessage(`{"key":"value"}`),
			CreatedBy: "ci-bot",
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var out RunSubmitRequest
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if out.RepoURL != in.RepoURL {
			t.Errorf("RepoURL: got %q, want %q", out.RepoURL, in.RepoURL)
		}
		if out.Ref != in.Ref {
			t.Errorf("Ref: got %q, want %q", out.Ref, in.Ref)
		}
		if out.CreatedBy != in.CreatedBy {
			t.Errorf("CreatedBy: got %q, want %q", out.CreatedBy, in.CreatedBy)
		}
	})

	t.Run("wire_field_names", func(t *testing.T) {
		t.Parallel()
		in := RunSubmitRequest{
			RepoURL: domaintypes.RepoURL("https://github.com/example/repo.git"),
			Ref:     domaintypes.GitRef("main"),
			Spec:    json.RawMessage(`{}`),
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		js := string(b)
		for _, want := range []string{`"repo_url":`, `"ref":`, `"spec":`} {
			if !strings.Contains(js, want) {
				t.Errorf("JSON missing field %s in %s", want, js)
			}
		}
		// created_by is omitempty — must be absent when empty.
		if strings.Contains(js, `"created_by"`) {
			t.Errorf("JSON must not contain \"created_by\" when empty, got %s", js)
		}
	})
}
