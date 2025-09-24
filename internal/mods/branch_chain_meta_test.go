package mods

import (
	"context"
	"encoding/json"
	"testing"
)

type metaPutCall struct {
	key  string
	body map[string]any
}

type metaRecordingUploader struct {
	writes []metaPutCall
}

func (r *metaRecordingUploader) UploadFile(context.Context, string, string, string, string) error {
	return nil
}

func (r *metaRecordingUploader) UploadJSON(_ context.Context, _ string, key string, body []byte) error {
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	r.writes = append(r.writes, metaPutCall{key: key, body: payload})
	return nil
}

func TestWriteBranchChainStepMeta_WritesMetaAndHead(t *testing.T) {
	// Mock getJSON to return existing HEAD with prev step
	prevGet := getJSONFn
	getJSONFn = func(seaweedBase, key string) ([]byte, int, error) {
		if key == "mods/e-1/branches/b-1/HEAD.json" {
			return []byte(`{"step_id":"s-prev"}`), 200, nil
		}
		return nil, 404, nil
	}
	defer func() { getJSONFn = prevGet }()

	uploader := &metaRecordingUploader{}

	if err := writeBranchChainStepMeta(context.Background(), uploader, "http://filer:8888", "e-1", "b-1", "s-2", "mods/e-1/branches/b-1/steps/s-2/diff.patch"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// Expect two writes: meta.json and HEAD.json
	if len(uploader.writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(uploader.writes))
	}
	// First is meta.json
	first := uploader.writes[0]
	if first.key != "mods/e-1/branches/b-1/steps/s-2/meta.json" {
		t.Fatalf("meta key mismatch: %s", first.key)
	}
	if first.body["step_id"] != "s-2" {
		t.Fatal("meta step_id mismatch")
	}
	if first.body["prev_step_id"] != "s-prev" {
		t.Fatal("meta prev_step_id mismatch")
	}
	if first.body["diff_key"] != "mods/e-1/branches/b-1/steps/s-2/diff.patch" {
		t.Fatal("meta diff_key mismatch")
	}
	// Second is HEAD
	second := uploader.writes[1]
	if second.key != "mods/e-1/branches/b-1/HEAD.json" {
		t.Fatalf("head key mismatch: %s", second.key)
	}
	if _, ok := second.body["step_id"]; !ok {
		t.Fatalf("head missing step_id")
	}
}

func TestWriteBranchChainStepMeta_NoPrevHead(t *testing.T) {
	prevGet := getJSONFn
	getJSONFn = func(seaweedBase, key string) ([]byte, int, error) {
		return nil, 404, nil
	}
	defer func() { getJSONFn = prevGet }()

	uploader := &metaRecordingUploader{}

	if err := writeBranchChainStepMeta(context.Background(), uploader, "http://filer:8888", "e-2", "b-2", "s-1", "mods/e-2/branches/b-2/steps/s-1/diff.patch"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(uploader.writes) != 2 {
		t.Fatalf("expected 2 writes, got %d", len(uploader.writes))
	}
}
