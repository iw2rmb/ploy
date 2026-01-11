package httpx

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestGunzipToBytes_Success(t *testing.T) {
	want := []byte("diff --git a/a b/a\n+line\n")

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write(want)
	_ = gw.Close()

	got, err := GunzipToBytes(bytes.NewReader(gzBuf.Bytes()), MaxGunzipOutputBytes)
	if err != nil {
		t.Fatalf("GunzipToBytes() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("GunzipToBytes() got %q, want %q", string(got), string(want))
	}
}

func TestGunzipToBytes_EmptyInput(t *testing.T) {
	got, err := GunzipToBytes(bytes.NewReader(nil), MaxGunzipOutputBytes)
	if err != nil {
		t.Fatalf("GunzipToBytes() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GunzipToBytes() got %d bytes, want 0", len(got))
	}
}

func TestGunzipToBytes_TooLarge(t *testing.T) {
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write([]byte(strings.Repeat("a", 11)))
	_ = gw.Close()

	_, err := GunzipToBytes(bytes.NewReader(gzBuf.Bytes()), 10)
	if err == nil {
		t.Fatalf("GunzipToBytes() error = nil, want non-nil")
	}
}
