package types

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRepoURL_AcceptsAndNormalizes(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  https://github.com/acme/repo.git  ", "https://github.com/acme/repo.git"},
		{" ssh://git@github.com/acme/repo.git ", "ssh://git@github.com/acme/repo.git"},
		{" file:///var/tmp/repo ", "file:///var/tmp/repo"},
	}
	for _, tt := range tests {
		var v RepoURL
		if err := v.UnmarshalText([]byte(tt.in)); err != nil {
			t.Fatalf("unmarshal %q: %v", tt.in, err)
		}
		if v.String() != tt.want {
			t.Fatalf("normalize: got %q want %q", v.String(), tt.want)
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		var v2 RepoURL
		if err := json.Unmarshal(b, &v2); err != nil {
			t.Fatalf("unmarshal json: %v", err)
		}
		if v2 != v {
			t.Fatalf("roundtrip mismatch: %q != %q", v2, v)
		}
	}
}

func TestRepoURL_RejectsEmpty(t *testing.T) {
	var v RepoURL
	if err := v.UnmarshalText([]byte("   ")); !errors.Is(err, ErrEmpty) {
		t.Fatalf("expected ErrEmpty, got %v", err)
	}
}

func TestGitRef_TrimAndJSON(t *testing.T) {
	var r GitRef
	if err := r.UnmarshalText([]byte("  main  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if r.String() != "main" {
		t.Fatalf("normalize got %q", r.String())
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var r2 GitRef
	if err := json.Unmarshal(b, &r2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if r2 != r {
		t.Fatalf("roundtrip mismatch: %q != %q", r2, r)
	}
}

func TestCommitSHA_TrimAndJSON(t *testing.T) {
	var c CommitSHA
	if err := c.UnmarshalText([]byte("  abcdef1  ")); err != nil {
		t.Fatalf("unmarshal text: %v", err)
	}
	if c.String() != "abcdef1" {
		t.Fatalf("normalize got %q", c.String())
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var c2 CommitSHA
	if err := json.Unmarshal(b, &c2); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	if c2 != c {
		t.Fatalf("roundtrip mismatch: %q != %q", c2, c)
	}
}
