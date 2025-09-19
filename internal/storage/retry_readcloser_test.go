package storage

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	buf := make([]byte, len("test content"))
	n, _ := m.Read(buf)
	if got := string(buf[:n]); !strings.HasPrefix(got, "test content") {
		t.Fatalf("expected test content prefix after reset, got %q", got)
	}
}

func TestRetryableReadCloser(t *testing.T) {
	tests := []struct {
		name        string
		setupMock   func(*MockStorageProvider)
		content     string
		expectError bool
		expectRetry bool
	}{
		{
			name:    "successful read",
			content: "test content",
			setupMock: func(m *MockStorageProvider) {
				// No additional GetObject calls expected for successful read
			},
			expectError: false,
			expectRetry: false,
		},
		{
			name:    "read error triggers retry",
			content: "test content",
			setupMock: func(m *MockStorageProvider) {
				// Retry call to GetObject
				reader := io.NopCloser(strings.NewReader("recovered content"))
				m.On("GetObject", "test-bucket", "test-key").Return(reader, nil)
			},
			expectError: false, // Should recover from error
			expectRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := &MockStorageProvider{}
			if tt.setupMock != nil {
				tt.setupMock(mockProvider)
			}

			var retryableReader *retryableReadCloser
			var expectedReader ReadCloser

			// Setup different readers based on test scenario
			if tt.expectRetry {
				// Use a failing reader that returns a network error
				failingReader := &FailingReader{}
				failingReader.On("Close").Return(nil)
				expectedReader = failingReader

				retryableReader = &retryableReadCloser{
					reader: failingReader,
					client: mockProvider,
					bucket: "test-bucket",
					key:    "test-key",
					config: DefaultRetryConfig(),
				}
			} else {
				// Create normal reader
				initialReader := NewMockReadCloser(tt.content)
				initialReader.On("Close").Return(nil)
				expectedReader = initialReader

				retryableReader = &retryableReadCloser{
					reader: initialReader,
					client: mockProvider,
					bucket: "test-bucket",
					key:    "test-key",
					config: DefaultRetryConfig(),
				}
			}

			// Test read (either normal or retry scenario)
			bufSize := len(tt.content)
			if tt.expectRetry {
				bufSize = len("recovered content") // Make buffer big enough for recovery
			}
			buf := make([]byte, bufSize)

			n, err := retryableReader.Read(buf)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectRetry {
					// Should have retried and got "recovered content"
					assert.Equal(t, "recovered content", string(buf[:n]))
				} else {
					assert.Equal(t, len(tt.content), n)
					assert.Equal(t, tt.content, string(buf[:n]))
				}
			}

			// Test close
			closeErr := retryableReader.Close()
			assert.NoError(t, closeErr)

			mockProvider.AssertExpectations(t)

			// Assert expectations on the reader that was actually used
			if mockReader, ok := expectedReader.(interface{ AssertExpectations(*testing.T) }); ok {
				mockReader.AssertExpectations(t)
			}
		})
	}
}

func TestRetryableReadCloser_Retry(t *testing.T) {
	mockProvider := &MockStorageProvider{}
	initialReader := NewMockReadCloser("initial content")
	retryReader := io.NopCloser(strings.NewReader("retry content"))

	initialReader.On("Close").Return(nil)
	mockProvider.On("GetObject", "test-bucket", "test-key").Return(retryReader, nil)

	retryableReader := &retryableReadCloser{
		reader: initialReader,
		client: mockProvider,
		bucket: "test-bucket",
		key:    "test-key",
		config: DefaultRetryConfig(),
	}

	// Test retry
	err := retryableReader.Retry()
	assert.NoError(t, err)

	// Verify new reader is set
	assert.NotEqual(t, initialReader, retryableReader.reader)

	// Test reading from new reader
	buf := make([]byte, 20)
	n, err := retryableReader.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, "retry content", string(buf[:n]))

	mockProvider.AssertExpectations(t)
	initialReader.AssertExpectations(t)
}

func TestIsRetryableReadError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "network error",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      errors.New("timeout occurred"),
			expected: true,
		},
		{
			name:     "random error",
			err:      errors.New("some random error"),
			expected: false,
		},
		{
			name:     "EOF error",
			err:      io.EOF,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableReadError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
