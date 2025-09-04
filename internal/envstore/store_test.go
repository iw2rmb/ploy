package envstore_test

import (
    "os"
    "path/filepath"
    "testing"

    envstore "github.com/iw2rmb/ploy/internal/envstore"
)

func TestStore_SetGetDelete_All(t *testing.T) {
    dir := t.TempDir()
    s := envstore.New(filepath.Join(dir, "env"))

    // Initially empty
    if got, err := s.GetAll("app1"); err != nil || len(got) != 0 {
        t.Fatalf("expected empty, got %#v, err=%v", got, err)
    }

    // Set single
    if err := s.Set("app1", "KEY1", "VAL1"); err != nil {
        t.Fatalf("set error: %v", err)
    }
    v, ok, err := s.Get("app1", "KEY1")
    if err != nil || !ok || v != "VAL1" {
        t.Fatalf("get mismatch: v=%q ok=%v err=%v", v, ok, err)
    }

    // SetAll
    all := envstore.AppEnvVars{"A": "1", "B": "2"}
    if err := s.SetAll("app1", all); err != nil {
        t.Fatalf("setall error: %v", err)
    }
    got, err := s.GetAll("app1")
    if err != nil || len(got) != 2 || got["A"] != "1" || got["B"] != "2" {
        t.Fatalf("getall mismatch: %#v err=%v", got, err)
    }

    // ToStringArray (order not guaranteed)
    arr, err := s.ToStringArray("app1")
    if err != nil {
        t.Fatalf("toStringArray error: %v", err)
    }
    if len(arr) != 2 {
        t.Fatalf("expected 2 items, got %d", len(arr))
    }

    // Delete
    if err := s.Delete("app1", "A"); err != nil {
        t.Fatalf("delete error: %v", err)
    }
    got, err = s.GetAll("app1")
    if err != nil {
        t.Fatalf("getall post-delete error: %v", err)
    }
    if _, exists := got["A"]; exists {
        t.Fatalf("expected A deleted, got: %#v", got)
    }

    // Ensure files are written under base path
    if _, err := os.Stat(filepath.Join(dir, "env", "app1.env.json")); err != nil {
        t.Fatalf("expected env file to exist: %v", err)
    }
}

