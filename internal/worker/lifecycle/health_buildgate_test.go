package lifecycle

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

type fakeRunner struct {
	stdout string
	stderr string
	err    error
}

func (f fakeRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	return f.stdout, f.stderr, f.err
}

func TestBuildGateJavaChecker_DefaultsAndDockerMissing(t *testing.T) {
	chk := NewBuildGateJavaChecker(BuildGateJavaCheckerOptions{
		Runner:  fakeRunner{err: exec.ErrNotFound},
		Timeout: 100 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(0, 0).UTC() },
	})
	st := chk.Check(context.Background())
	if st.State != stateDegraded {
		t.Fatalf("expected degraded when docker missing, got %s", st.State)
	}
	if st.CheckedAt.IsZero() {
		t.Fatal("expected CheckedAt to be set")
	}
}

func TestBuildGateJavaChecker_ImagePresent(t *testing.T) {
	chk := NewBuildGateJavaChecker(BuildGateJavaCheckerOptions{
		Runner:  fakeRunner{stdout: "ok"},
		Timeout: 100 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(1, 0).UTC() },
	})
	st := chk.Check(context.Background())
	if st.State != stateOK {
		t.Fatalf("expected ok, got %s", st.State)
	}
	if st.Details["image"] == "" {
		t.Fatal("expected image detail to be set")
	}
}

// no extra helpers
