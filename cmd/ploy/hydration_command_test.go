package main

import (
	"bytes"
	"errors"
	"testing"
)

type fakeHydrationClient struct {
	err error
}

func (f *fakeHydrationClient) Inspect(ticket string) error {
	return f.err
}

func (f *fakeHydrationClient) Tune(ticket string, opts hydrationTuneOptions) error {
	return f.err
}

func TestHandleHydrationRequiresSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	if err := handleHydration(nil, buf, &fakeHydrationClient{}); err == nil {
		t.Fatalf("expected error when no subcommand provided")
	}
	if buf.Len() == 0 {
		t.Fatalf("expected usage printed")
	}
}

func TestHandleHydrationInspectPropagatesClientError(t *testing.T) {
	buf := &bytes.Buffer{}
	client := &fakeHydrationClient{err: errors.New("boom")}
	err := handleHydration([]string{"inspect", "mod-1"}, buf, client)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}
