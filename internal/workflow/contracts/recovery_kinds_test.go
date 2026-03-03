package contracts

import "testing"

func TestParseRecoveryLoopKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  RecoveryLoopKind
		ok    bool
	}{
		{name: "valid", input: "healing", want: RecoveryLoopKindHealing, ok: true},
		{name: "trimmed", input: " healing ", want: RecoveryLoopKindHealing, ok: true},
		{name: "invalid", input: "router", ok: false},
		{name: "empty", input: "", ok: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ParseRecoveryLoopKind(tc.input)
			if ok != tc.ok {
				t.Fatalf("ok=%v, want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Fatalf("kind=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseRecoveryErrorKind(t *testing.T) {
	t.Parallel()

	for _, kind := range RecoveryErrorKinds() {
		got, ok := ParseRecoveryErrorKind(kind.String())
		if !ok {
			t.Fatalf("expected %q to parse", kind)
		}
		if got != kind {
			t.Fatalf("kind=%q, want %q", got, kind)
		}
	}

	if _, ok := ParseRecoveryErrorKind("custom"); ok {
		t.Fatal("expected custom kind to fail parsing")
	}
}

func TestIsTerminalRecoveryErrorKind(t *testing.T) {
	t.Parallel()

	if !IsTerminalRecoveryErrorKind(RecoveryErrorKindMixed) {
		t.Fatal("mixed must be terminal")
	}
	if !IsTerminalRecoveryErrorKind(RecoveryErrorKindUnknown) {
		t.Fatal("unknown must be terminal")
	}
	if IsTerminalRecoveryErrorKind(RecoveryErrorKindInfra) {
		t.Fatal("infra must be non-terminal")
	}
	if IsTerminalRecoveryErrorKind(RecoveryErrorKindCode) {
		t.Fatal("code must be non-terminal")
	}
	if IsTerminalRecoveryErrorKind(RecoveryErrorKindDeps) {
		t.Fatal("deps must be non-terminal")
	}
}
