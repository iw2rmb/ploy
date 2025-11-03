package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	typesystem "github.com/docker/docker/api/types/system"
)

type fakeDocker struct {
	ping    types.Ping
	pingErr error
	info    typesystem.Info
	infoErr error
}

func (f fakeDocker) Ping(ctx context.Context) (types.Ping, error)      { return f.ping, f.pingErr }
func (f fakeDocker) Info(ctx context.Context) (typesystem.Info, error) { return f.info, f.infoErr }
func (f fakeDocker) Close() error                                      { return nil }

func TestDockerChecker_PingError(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fakeDocker{pingErr: errors.New("boom")},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(10, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateError {
		t.Fatalf("want error, got %s", st.State)
	}
}

func TestDockerChecker_InfoError(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client:  fakeDocker{infoErr: errors.New("no info")},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(11, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateDegraded {
		t.Fatalf("want degraded, got %s", st.State)
	}
}

func TestDockerChecker_OK(t *testing.T) {
	c, err := NewDockerChecker(DockerCheckerOptions{
		Client: fakeDocker{
			ping: types.Ping{APIVersion: "1.44"},
			info: typesystem.Info{ServerVersion: "25.0.0", Driver: "overlay2", ContainersRunning: 3},
		},
		Timeout: 50 * time.Millisecond,
		Clock:   func() time.Time { return time.Unix(12, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	st := c.Check(context.Background())
	if st.State != stateOK {
		t.Fatalf("want ok, got %s", st.State)
	}
	if st.Version != "25.0.0" {
		t.Fatalf("unexpected version: %s", st.Version)
	}
	if v, ok := st.Details["api_version"]; !ok || v.(string) != "1.44" {
		t.Fatalf("unexpected api_version: %#v", st.Details)
	}
}
