package gitlab

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzExtractProjectIDFromURL exercises parsing for a variety of inputs and
// guarantees no panics while enforcing basic invariants for valid HTTPS URLs.
func FuzzExtractProjectIDFromURL(f *testing.F) {
	// Seeds
	f.Add("https://gitlab.com/group/project.git")
	f.Add("https://gitlab.example.com/a/b/c.git")
	f.Add("https://gitlab.example.com/group/project")
	f.Add("not a url")
	f.Add("")

	f.Fuzz(func(t *testing.T, raw string) {
		id, err := ExtractProjectIDFromURL(raw)

		// If input looks like a valid http(s) GitLab URL with a non-empty path,
		// expect a non-empty encoded id whose decoding matches the path sans
		// leading slash and optional .git suffix.
		if u, uerr := url.Parse(raw); uerr == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			// Normalize path for comparison.
			p := strings.TrimPrefix(u.Path, "/")
			p = strings.TrimSuffix(p, ".git")

			if p == "" {
				if err == nil || id != "" {
					t.Fatalf("want error for empty path; got id=%q err=%v", id, err)
				}
				return
			}

			if err != nil || id == "" {
				t.Fatalf("want non-empty id for %q; got id=%q err=%v", raw, id, err)
			}

			decoded, derr := url.PathUnescape(id)
			if derr != nil {
				t.Fatalf("PathUnescape failed: %v", derr)
			}
			if decoded != p {
				t.Fatalf("decoded id mismatch: got %q want %q", decoded, p)
			}
			return
		}

		// Non-URL or unsupported scheme should return an error or empty id.
		if err == nil && id != "" {
			t.Fatalf("unexpected success for invalid input %q: id=%q", raw, id)
		}
	})
}
