package storage

import (
	"errors"
	"strings"

	"github.com/stretchr/testify/mock"
)

type countProvider struct {
	MockStorageProvider
	listCalls, verifyCalls int
}

func (cp *countProvider) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	cp.listCalls++
	if cp.listCalls == 1 {
		return nil, errors.New("connection reset by peer")
	}
	return []ObjectInfo{{Key: prefix + "/a", Size: 1}}, nil
}

func (cp *countProvider) VerifyUpload(key string) error {
	cp.verifyCalls++
	if cp.verifyCalls == 1 {
		return errors.New("service unavailable")
	}
	return nil
}

// Mock ReadSeekerResetter for testing
// Ensures Reset can be asserted while swapping the reader content.
type MockReadSeekerResetter struct {
	mock.Mock
	*strings.Reader
}

func NewMockReadSeekerResetter(content string) *MockReadSeekerResetter {
	return &MockReadSeekerResetter{
		Reader: strings.NewReader(content),
	}
}

func (m *MockReadSeekerResetter) Reset() error {
	args := m.Called()
	m.Reader = strings.NewReader("test content")
	return args.Error(0)
}

// MockReadCloser tracks close calls and enforces expected behaviour in tests.
type MockReadCloser struct {
	mock.Mock
	*strings.Reader
	closed bool
}

func NewMockReadCloser(content string) *MockReadCloser {
	return &MockReadCloser{
		Reader: strings.NewReader(content),
		closed: false,
	}
}

func (m *MockReadCloser) Read(p []byte) (int, error) {
	if m.closed {
		return 0, errors.New("reader closed")
	}
	return m.Reader.Read(p)
}

func (m *MockReadCloser) Close() error {
	args := m.Called()
	m.closed = true
	return args.Error(0)
}

// FailingReader implements ReadCloser but fails on the first read to trigger retry paths.
type FailingReader struct {
	mock.Mock
	closed bool
}

func (f *FailingReader) Read(p []byte) (int, error) {
	if f.closed {
		return 0, errors.New("reader closed")
	}
	return 0, errors.New("connection reset")
}

func (f *FailingReader) Close() error {
	args := f.Called()
	f.closed = true
	return args.Error(0)
}
