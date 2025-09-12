package transflow

import (
    "encoding/json"
    "testing"
)

func TestWriteBranchChainStepMeta_WritesMetaAndHead(t *testing.T) {
    // Mock getJSON to return existing HEAD with prev step
    prevGet := getJSONFn
    getJSONFn = func(seaweedBase, key string) ([]byte, int, error) {
        if key == "transflow/e-1/branches/b-1/HEAD.json" {
            return []byte(`{"step_id":"s-prev"}`), 200, nil
        }
        return nil, 404, nil
    }
    defer func() { getJSONFn = prevGet }()

    // Capture putJSON writes
    type putCall struct{ key string; body map[string]any }
    var calls []putCall
    prevPut := putJSONFn
    putJSONFn = func(base, key string, body []byte) error {
        var m map[string]any
        _ = json.Unmarshal(body, &m)
        calls = append(calls, putCall{key: key, body: m})
        return nil
    }
    defer func() { putJSONFn = prevPut }()

    if err := writeBranchChainStepMeta("http://filer:8888", "e-1", "b-1", "s-2", "transflow/e-1/branches/b-1/steps/s-2/diff.patch"); err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    // Expect two writes: meta.json and HEAD.json
    if len(calls) != 2 { t.Fatalf("expected 2 writes, got %d", len(calls)) }
    // First is meta.json
    if calls[0].key != "transflow/e-1/branches/b-1/steps/s-2/meta.json" {
        t.Fatalf("meta key mismatch: %s", calls[0].key)
    }
    if calls[0].body["step_id"] != "s-2" { t.Fatal("meta step_id mismatch") }
    if calls[0].body["prev_step_id"] != "s-prev" { t.Fatal("meta prev_step_id mismatch") }
    if calls[0].body["diff_key"] != "transflow/e-1/branches/b-1/steps/s-2/diff.patch" { t.Fatal("meta diff_key mismatch") }
    // Second is HEAD
    if calls[1].key != "transflow/e-1/branches/b-1/HEAD.json" {
        t.Fatalf("head key mismatch: %s", calls[1].key)
    }
    // HEAD has only step_id field as string JSON
    if _, ok := calls[1].body["step_id"]; !ok {
        t.Fatalf("head missing step_id")
    }
}

func TestWriteBranchChainStepMeta_NoPrevHead(t *testing.T) {
    prevGet := getJSONFn
    getJSONFn = func(seaweedBase, key string) ([]byte, int, error) {
        return nil, 404, nil
    }
    defer func() { getJSONFn = prevGet }()

    count := 0
    prevPut := putJSONFn
    putJSONFn = func(base, key string, body []byte) error {
        count++
        return nil
    }
    defer func() { putJSONFn = prevPut }()

    if err := writeBranchChainStepMeta("http://filer:8888", "e-2", "b-2", "s-1", "transflow/e-2/branches/b-2/steps/s-1/diff.patch"); err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    if count != 2 {
        t.Fatalf("expected 2 writes, got %d", count)
    }
}
