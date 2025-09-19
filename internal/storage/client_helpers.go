package storage

import (
	"io"
	"time"
)

type fileReadSeekerResetter struct {
	readSeeker io.ReadSeeker
}

func (f *fileReadSeekerResetter) Read(p []byte) (int, error) {
	return f.readSeeker.Read(p)
}

func (f *fileReadSeekerResetter) Seek(offset int64, whence int) (int64, error) {
	return f.readSeeker.Seek(offset, whence)
}

func (f *fileReadSeekerResetter) Reset() error {
	_, err := f.readSeeker.Seek(0, 0)
	return err
}

type metricsTrackingReadCloser struct {
	readCloser io.ReadCloser
	metrics    *StorageMetrics
	startTime  time.Time
	bytesRead  *int64
}

func (m *metricsTrackingReadCloser) Read(p []byte) (int, error) {
	n, err := m.readCloser.Read(p)
	if n > 0 {
		*m.bytesRead += int64(n)
	}
	return n, err
}

func (m *metricsTrackingReadCloser) Close() error {
	defer func() {
		if m.metrics != nil {
			duration := time.Since(m.startTime)
			m.metrics.RecordDownload(true, duration, *m.bytesRead, "")
		}
	}()
	return m.readCloser.Close()
}
