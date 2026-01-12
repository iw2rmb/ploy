package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestModRef tests the ModRef type for mod reference validation and serialization.
func TestModRef(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		ref := ModRef("my-mod")
		if ref.String() != "my-mod" {
			t.Errorf("ModRef.String() = %q, want %q", ref.String(), "my-mod")
		}
	})

	t.Run("IsZero", func(t *testing.T) {
		t.Parallel()
		var empty ModRef
		if !empty.IsZero() {
			t.Error("zero ModRef.IsZero() = false, want true")
		}
		nonEmpty := ModRef("mod123")
		if nonEmpty.IsZero() {
			t.Error("non-zero ModRef.IsZero() = true, want false")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			value   string
			wantErr error
		}{
			// Valid cases: mod IDs and mod names.
			{"valid_nanoid", "abc123", nil},
			{"valid_name", "my-mod", nil},
			{"valid_name_underscore", "my_mod_name", nil},
			{"valid_alphanumeric", "ModName123", nil},
			{"valid_uuid_like", "12345678-1234-1234-1234-123456789012", nil}, // No special treatment

			// Invalid: empty.
			{"empty", "", ErrEmpty},
			{"whitespace_only", "   ", ErrEmpty},

			// Invalid: contains URL-unsafe characters.
			{"contains_slash", "my/mod", ErrInvalidModRef},
			{"contains_question", "mod?name", ErrInvalidModRef},
			{"contains_space", "my mod", ErrInvalidModRef},
			{"contains_tab", "my\tmod", ErrInvalidModRef},
			{"contains_newline", "my\nmod", ErrInvalidModRef},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ref := ModRef(tt.value)
				err := ref.Validate()
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("ModRef(%q).Validate() = %v, want %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})

	t.Run("TextRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []string{"mod123", "my-mod", "ModName_v2"}
		for _, v := range tests {
			ref := ModRef(v)
			b, err := ref.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%q): %v", v, err)
			}
			var ref2 ModRef
			if err := ref2.UnmarshalText(b); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", string(b), err)
			}
			if ref2 != ref {
				t.Errorf("text roundtrip: got %q, want %q", ref2, ref)
			}
		}
	})

	t.Run("TextUnmarshalTrimsWhitespace", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		if err := ref.UnmarshalText([]byte("  my-mod  ")); err != nil {
			t.Fatalf("UnmarshalText: %v", err)
		}
		if ref != "my-mod" {
			t.Errorf("got %q, want %q", ref, "my-mod")
		}
	})

	t.Run("TextUnmarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		err := ref.UnmarshalText([]byte(""))
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("UnmarshalText(\"\") = %v, want ErrEmpty", err)
		}
		err = ref.UnmarshalText([]byte("   "))
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("UnmarshalText(\"   \") = %v, want ErrEmpty", err)
		}
	})

	t.Run("TextUnmarshalRejectsInvalidChars", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		err := ref.UnmarshalText([]byte("my/mod"))
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("UnmarshalText(\"my/mod\") = %v, want ErrInvalidModRef", err)
		}
		err = ref.UnmarshalText([]byte("mod?name"))
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("UnmarshalText(\"mod?name\") = %v, want ErrInvalidModRef", err)
		}
	})

	t.Run("TextMarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		ref := ModRef("")
		_, err := ref.MarshalText()
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("MarshalText(\"\") = %v, want ErrEmpty", err)
		}
	})

	t.Run("TextMarshalRejectsInvalidChars", func(t *testing.T) {
		t.Parallel()

		ref := ModRef("my/mod")
		_, err := ref.MarshalText()
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("MarshalText(\"my/mod\") = %v, want ErrInvalidModRef", err)
		}
	})

	t.Run("JSONRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []string{"mod123", "my-mod", "ModName_v2"}
		for _, v := range tests {
			ref := ModRef(v)
			b, err := json.Marshal(ref)
			if err != nil {
				t.Fatalf("json.Marshal(%q): %v", v, err)
			}
			var ref2 ModRef
			if err := json.Unmarshal(b, &ref2); err != nil {
				t.Fatalf("json.Unmarshal(%q): %v", string(b), err)
			}
			if ref2 != ref {
				t.Errorf("json roundtrip: got %q, want %q", ref2, ref)
			}
		}
	})

	t.Run("JSONUnmarshalRejectsInvalid", func(t *testing.T) {
		t.Parallel()

		var ref ModRef
		err := json.Unmarshal([]byte(`"my/mod"`), &ref)
		if !errors.Is(err, ErrInvalidModRef) {
			t.Errorf("json.Unmarshal(\"my/mod\") = %v, want ErrInvalidModRef", err)
		}
	})
}
