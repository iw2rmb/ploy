package types

import (
	"encoding/json"
	"testing"
)

// TestEventID tests the EventID type for SSE cursor validation and serialization.
func TestEventID(t *testing.T) {
	t.Parallel()

	t.Run("Valid", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			value int64
			want  bool
		}{
			// Valid: non-negative values.
			{"zero", 0, true},
			{"positive_small", 1, true},
			{"positive_large", 999999999, true},
			{"max_int64", 9223372036854775807, true},

			// Invalid: negative values.
			{"negative_small", -1, false},
			{"negative_large", -999999999, false},
			{"min_int64", -9223372036854775808, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				eid := EventID(tt.value)
				if got := eid.Valid(); got != tt.want {
					t.Errorf("EventID(%d).Valid() = %v, want %v", tt.value, got, tt.want)
				}
			})
		}
	})

	t.Run("Int64", func(t *testing.T) {
		t.Parallel()
		tests := []int64{0, 1, 100, 999999999}
		for _, v := range tests {
			eid := EventID(v)
			if got := eid.Int64(); got != v {
				t.Errorf("EventID(%d).Int64() = %d, want %d", v, got, v)
			}
		}
	})

	t.Run("String", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			value int64
			want  string
		}{
			{0, "0"},
			{1, "1"},
			{42, "42"},
			{999999999, "999999999"},
		}
		for _, tt := range tests {
			eid := EventID(tt.value)
			if got := eid.String(); got != tt.want {
				t.Errorf("EventID(%d).String() = %q, want %q", tt.value, got, tt.want)
			}
		}
	})

	t.Run("IsZero", func(t *testing.T) {
		t.Parallel()
		if !EventID(0).IsZero() {
			t.Error("EventID(0).IsZero() = false, want true")
		}
		if EventID(1).IsZero() {
			t.Error("EventID(1).IsZero() = true, want false")
		}
	})

	t.Run("TextRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []int64{0, 1, 42, 999999999}
		for _, v := range tests {
			eid := EventID(v)
			b, err := eid.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%d): %v", v, err)
			}
			var eid2 EventID
			if err := eid2.UnmarshalText(b); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", string(b), err)
			}
			if eid2 != eid {
				t.Errorf("text roundtrip: got %d, want %d", eid2, eid)
			}
		}
	})

	t.Run("TextUnmarshalRejectsNegative", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := eid.UnmarshalText([]byte("-1"))
		if err == nil {
			t.Error("UnmarshalText(-1) should fail, got nil")
		}
		err = eid.UnmarshalText([]byte("-999"))
		if err == nil {
			t.Error("UnmarshalText(-999) should fail, got nil")
		}
	})

	t.Run("TextUnmarshalRejectsEmpty", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := eid.UnmarshalText([]byte(""))
		if err == nil {
			t.Error("UnmarshalText(\"\") should fail, got nil")
		}
		err = eid.UnmarshalText([]byte("   "))
		if err == nil {
			t.Error("UnmarshalText(\"   \") should fail, got nil")
		}
	})

	t.Run("TextUnmarshalRejectsInvalid", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := eid.UnmarshalText([]byte("abc"))
		if err == nil {
			t.Error("UnmarshalText(\"abc\") should fail, got nil")
		}
		err = eid.UnmarshalText([]byte("12.5"))
		if err == nil {
			t.Error("UnmarshalText(\"12.5\") should fail, got nil")
		}
	})

	t.Run("TextMarshalRejectsNegative", func(t *testing.T) {
		t.Parallel()

		eid := EventID(-1)
		_, err := eid.MarshalText()
		if err == nil {
			t.Error("MarshalText(-1) should fail, got nil")
		}
	})

	t.Run("JSONRoundTrip", func(t *testing.T) {
		t.Parallel()

		tests := []int64{0, 1, 42, 999999999}
		for _, v := range tests {
			eid := EventID(v)
			b, err := json.Marshal(eid)
			if err != nil {
				t.Fatalf("json.Marshal(%d): %v", v, err)
			}
			var eid2 EventID
			if err := json.Unmarshal(b, &eid2); err != nil {
				t.Fatalf("json.Unmarshal(%q): %v", string(b), err)
			}
			if eid2 != eid {
				t.Errorf("json roundtrip: got %d, want %d", eid2, eid)
			}
		}
	})

	t.Run("JSONUnmarshalRejectsNegative", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := json.Unmarshal([]byte("-1"), &eid)
		if err == nil {
			t.Error("json.Unmarshal(-1) should fail, got nil")
		}
	})

	t.Run("JSONUnmarshalRejectsNull", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := json.Unmarshal([]byte("null"), &eid)
		if err == nil {
			t.Fatalf("json.Unmarshal(null) should fail, got nil (eid=%d)", eid)
		}
	})

	t.Run("JSONUnmarshalRejectsString", func(t *testing.T) {
		t.Parallel()

		var eid EventID
		err := json.Unmarshal([]byte(`"42"`), &eid)
		if err == nil {
			t.Fatalf("json.Unmarshal(\"42\") should fail, got nil (eid=%d)", eid)
		}
	})
}
