package types

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestModRef tests the MigRef type for mig reference validation and serialization.
func TestModRef(t *testing.T) {
	t.Parallel()

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		ref := MigRef("my-mig")
		if ref.String() != "my-mig" {
			t.Errorf("MigRef.String() = %q, want %q", ref.String(), "my-mig")
		}
	})

	t.Run("IsZero", func(t *testing.T) {
		t.Parallel()
		var empty MigRef
		if !empty.IsZero() {
			t.Error("zero MigRef.IsZero() = false, want true")
		}
		nonEmpty := MigRef("mod123")
		if nonEmpty.IsZero() {
			t.Error("non-zero MigRef.IsZero() = true, want false")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name    string
			value   string
			wantErr error
		}{
			// Valid cases: mig IDs and mig names.
			{"valid_nanoid", "abc123", nil},
			{"valid_name", "my-mig", nil},
			{"valid_name_underscore", "my_mod_name", nil},
			{"valid_alphanumeric", "ModName123", nil},
			{"valid_uuid_like", "12345678-1234-1234-1234-123456789012", nil}, // No special treatment

			// Invalid: empty.
			{"empty", "", ErrEmpty},
			{"whitespace_only", "   ", ErrEmpty},

			// Invalid: contains URL-unsafe characters.
			{"contains_slash", "my/mig", ErrInvalidMigRef},
			{"contains_question", "mig?name", ErrInvalidMigRef},
			{"contains_space", "my mig", ErrInvalidMigRef},
			{"contains_tab", "my\tmod", ErrInvalidMigRef},
			{"contains_newline", "my\nmod", ErrInvalidMigRef},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ref := MigRef(tt.value)
				err := ref.Validate()
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("MigRef(%q).Validate() = %v, want %v", tt.value, err, tt.wantErr)
				}
			})
		}
	})

	t.Run("TextRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []string{"mod123", "my-mig", "ModName_v2"}
		for _, v := range tests {
			ref := MigRef(v)
			b, err := ref.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%q): %v", v, err)
			}
			var ref2 MigRef
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

		var ref MigRef
		if err := ref.UnmarshalText([]byte("  my-mig  ")); err != nil {
			t.Fatalf("UnmarshalText: %v", err)
		}
		if ref != "my-mig" {
			t.Errorf("got %q, want %q", ref, "my-mig")
		}
	})

	t.Run("TextUnmarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		var ref MigRef
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

		var ref MigRef
		err := ref.UnmarshalText([]byte("my/mig"))
		if !errors.Is(err, ErrInvalidMigRef) {
			t.Errorf("UnmarshalText(\"my/mig\") = %v, want ErrInvalidMigRef", err)
		}
		err = ref.UnmarshalText([]byte("mig?name"))
		if !errors.Is(err, ErrInvalidMigRef) {
			t.Errorf("UnmarshalText(\"mig?name\") = %v, want ErrInvalidMigRef", err)
		}
	})

	t.Run("TextMarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		ref := MigRef("")
		_, err := ref.MarshalText()
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("MarshalText(\"\") = %v, want ErrEmpty", err)
		}
	})

	t.Run("TextMarshalRejectsInvalidChars", func(t *testing.T) {
		t.Parallel()

		ref := MigRef("my/mig")
		_, err := ref.MarshalText()
		if !errors.Is(err, ErrInvalidMigRef) {
			t.Errorf("MarshalText(\"my/mig\") = %v, want ErrInvalidMigRef", err)
		}
	})

	t.Run("JSONRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []string{"mod123", "my-mig", "ModName_v2"}
		for _, v := range tests {
			ref := MigRef(v)
			b, err := json.Marshal(ref)
			if err != nil {
				t.Fatalf("json.Marshal(%q): %v", v, err)
			}
			var ref2 MigRef
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

		var ref MigRef
		err := json.Unmarshal([]byte(`"my/mig"`), &ref)
		if !errors.Is(err, ErrInvalidMigRef) {
			t.Errorf("json.Unmarshal(\"my/mig\") = %v, want ErrInvalidMigRef", err)
		}
	})
}
