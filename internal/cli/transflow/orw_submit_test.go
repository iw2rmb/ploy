package transflow

import (
    "context"
    "errors"
    "path/filepath"
    "testing"
    "time"
)

func TestSubmitORWJobAndFetchDiff_Errors(t *testing.T) {
    tmp := t.TempDir()
    hcl := filepath.Join(tmp, "job.hcl")
    diff := filepath.Join(tmp, "out.diff")
    // validation error
    err := submitORWJobAndFetchDiff(context.Background(), func(string) error { return errors.New("bad hcl") }, func(string, time.Duration) error { return nil }, nil, "http://filer", "e", "b", "s", "job-1", hcl, diff, time.Second)
    if err == nil { t.Fatalf("expected validation error") }
    // submit error
    err = submitORWJobAndFetchDiff(context.Background(), func(string) error { return nil }, func(string, time.Duration) error { return errors.New("bad submit") }, nil, "http://filer", "e", "b", "s", "job-1", hcl, diff, time.Second)
    if err == nil { t.Fatalf("expected submit error") }
    // fetch error (execID empty)
    err = submitORWJobAndFetchDiff(context.Background(), func(string) error { return nil }, func(string, time.Duration) error { return nil }, nil, "http://filer", "", "b", "s", "job-1", hcl, diff, time.Second)
    if err == nil { t.Fatalf("expected fetch error due to missing exec id") }
}

