package nodeagent

import (
    "bytes"
    "compress/gzip"
    "context"
    "crypto/rand"
    "io"
    "os"
    "path/filepath"
    "testing"

    "github.com/iw2rmb/ploy/internal/workflow/runtime/step"
)

type capturePublisher struct{
    lastReq step.ArtifactRequest
    resp step.PublishedArtifact
    err error
}

func (c *capturePublisher) Publish(ctx context.Context, req step.ArtifactRequest) (step.PublishedArtifact, error){
    c.lastReq = req
    if c.err != nil { return step.PublishedArtifact{}, c.err }
    // Return a predictable artifact
    return step.PublishedArtifact{CID: "test:cid", Kind: req.Kind, Digest: "sha256:test", Size: int64(len(req.Buffer))}, nil
}

func TestSizeLimitedPublisher_SmallBuffer_PassesAndCompresses(t *testing.T){
    base := &capturePublisher{}
    p := newSizeLimitedPublisher(base, 1<<20) // 1 MiB

    payload := bytes.Repeat([]byte("a"), 1024) // very compressible
    art, err := p.Publish(context.Background(), step.ArtifactRequest{Kind: step.ArtifactKindLogs, Buffer: payload})
    if err != nil {
        t.Fatalf("Publish error: %v", err)
    }
    if art.CID == "" { t.Fatalf("expected CID to be set") }

    // Ensure the delegate received gzipped data that decompresses to original payload.
    if len(base.lastReq.Buffer) == 0 {
        t.Fatalf("delegate buffer empty; expected compressed data")
    }
    zr, err := gzip.NewReader(bytes.NewReader(base.lastReq.Buffer))
    if err != nil { t.Fatalf("gzip reader: %v", err) }
    got, err := io.ReadAll(zr)
    if err != nil { t.Fatalf("decompress: %v", err) }
    _ = zr.Close()
    if !bytes.Equal(got, payload) {
        t.Fatalf("roundtrip mismatch: got %d bytes, want %d", len(got), len(payload))
    }
}

func TestSizeLimitedPublisher_MissingPayload_Err(t *testing.T){
    p := newSizeLimitedPublisher(&capturePublisher{}, 1<<20)
    if _, err := p.Publish(context.Background(), step.ArtifactRequest{Kind: step.ArtifactKindDiff}); err == nil {
        t.Fatalf("expected error for missing payload")
    }
}

func TestSizeLimitedPublisher_PathInput_Passes(t *testing.T){
    base := &capturePublisher{}
    p := newSizeLimitedPublisher(base, 1<<20)

    dir := t.TempDir()
    path := filepath.Join(dir, "artifact.txt")
    content := []byte("hello world")
    if err := os.WriteFile(path, content, 0o600); err != nil {
        t.Fatalf("write temp file: %v", err)
    }
    if _, err := p.Publish(context.Background(), step.ArtifactRequest{Kind: step.ArtifactKindDiff, Path: path}); err != nil {
        t.Fatalf("Publish error: %v", err)
    }
    // sanity: compressed buffer sent to delegate
    if len(base.lastReq.Buffer) == 0 { t.Fatalf("expected compressed data passed to delegate") }
}

func TestSizeLimitedPublisher_OversizeRejected(t *testing.T){
    base := &capturePublisher{}
    p := newSizeLimitedPublisher(base, 1<<20)

    // Create largely incompressible payload (~1.2 MiB)
    // ~1.1 MiB of random data to exceed 1 MiB limit after gzip.
    big := make([]byte, 1100*1024)
    if _, err := rand.Read(big); err != nil { t.Fatalf("rand: %v", err) }

    if _, err := p.Publish(context.Background(), step.ArtifactRequest{Kind: step.ArtifactKindLogs, Buffer: big}); err == nil {
        t.Fatalf("expected oversize error, got nil")
    }
    // Ensure delegate was not called
    if base.lastReq.Kind != "" {
        t.Fatalf("delegate should not be called on oversize")
    }
}
