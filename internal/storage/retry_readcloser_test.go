package storage

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeReadCloser returns the provided data and then the provided error.
type fakeReadCloser struct {
	data []byte
	pos  int
	err  error
}

func (f *fakeReadCloser) Read(p []byte) (int, error) {
	if f.pos >= len(f.data) {
		return 0, f.err
	}
	n := copy(p, f.data[f.pos:])
	f.pos += n
	if f.pos >= len(f.data) {
		return n, f.err
	}
	return n, nil
}
func (f *fakeReadCloser) Close() error { return nil }

// fakeProvider implements the minimal parts of StorageProvider we need.
type fakeProvider struct {
	reader io.ReadCloser
	getErr error
}

func (p *fakeProvider) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	return nil, nil
}
func (p *fakeProvider) UploadArtifactBundle(keyPrefix, artifactPath string) error { return nil }
func (p *fakeProvider) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
	return nil, nil
}
func (p *fakeProvider) VerifyUpload(key string) error { return nil }
func (p *fakeProvider) GetObject(bucket, key string) (io.ReadCloser, error) {
	return p.reader, p.getErr
}
func (p *fakeProvider) ListObjects(bucket, prefix string) ([]ObjectInfo, error) { return nil, nil }
func (p *fakeProvider) GetProviderType() string                                 { return "fake" }
func (p *fakeProvider) GetArtifactsBucket() string                              { return "artifacts" }

func TestRetryableReadCloser_RetryOnReadError(t *testing.T) {
	// First reader simulates a network error at EOF; second reader returns clean EOF
	bad := &fakeReadCloser{data: []byte("abc"), err: errors.New("connection reset by peer")}
	good := &fakeReadCloser{data: []byte("abc"), err: io.EOF}
	prov := &fakeProvider{reader: bad}

	r := &retryableReadCloser{reader: bad, client: prov, bucket: "b", key: "k", config: DefaultRetryConfig()}

	// Override provider to return good reader on reopen
	prov.reader = good

	buf := make([]byte, 8)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error on read: %v", err)
	}
	if got := string(buf[:n]); got != "abc" {
		t.Fatalf("expected to read 'abc' after retry, got %q", got)
	}
}

func TestReadSeekerResetterContract(t *testing.T) {
	// Ensure our helper type compiles against the interface and Reset works
	m := NewMockReadSeekerResetter("hello")
	m.On("Reset").Return(nil)
	if err := m.Reset(); err != nil {
		t.Fatalf("reset failed: %v", err)
	}
	buf := make([]byte, 5)
	n, _ := m.Read(buf)
	if got := string(buf[:n]); !strings.HasPrefix(got, "test content") {
		t.Fatalf("expected test content prefix after reset, got %q", got)
	}
}
