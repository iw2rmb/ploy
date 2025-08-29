package storage

import (
	"context"
	"fmt"

	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

// InternalStorageAdapter adapts internal storage client to ARF StorageService interface
type InternalStorageAdapter struct {
	client internalStorage.StorageProvider
	bucket string
}

// NewInternalStorageAdapter creates a new adapter for internal storage
func NewInternalStorageAdapter(client internalStorage.StorageProvider) StorageService {
	return &InternalStorageAdapter{
		client: client,
		bucket: client.GetArtifactsBucket(),
	}
}

// Put stores data at the given key
func (a *InternalStorageAdapter) Put(ctx context.Context, key string, data []byte) error {
	// Create a reader from the data
	reader := &bytesReadSeeker{data: data}
	_, err := a.client.PutObject(a.bucket, key, reader, "application/octet-stream")
	return err
}

// Get retrieves data from the given key
func (a *InternalStorageAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	reader, err := a.client.GetObject(a.bucket, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key %s: %w", key, err)
	}
	defer reader.Close()
	
	// Read all data from the reader
	data := make([]byte, 0)
	buffer := make([]byte, 4096)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			data = append(data, buffer[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
	}
	return data, nil
}

// Delete removes data at the given key
func (a *InternalStorageAdapter) Delete(ctx context.Context, key string) error {
	// StorageProvider doesn't have a Delete method, so we'll return an error
	return fmt.Errorf("delete operation not supported by storage provider")
}

// Exists checks if a key exists in storage
func (a *InternalStorageAdapter) Exists(ctx context.Context, key string) (bool, error) {
	// Try to verify the upload which checks existence
	err := a.client.VerifyUpload(key)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// bytesReadSeeker implements io.ReadSeeker for byte slices
type bytesReadSeeker struct {
	data []byte
	pos  int64
}

func (b *bytesReadSeeker) Read(p []byte) (n int, err error) {
	if b.pos >= int64(len(b.data)) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, b.data[b.pos:])
	b.pos += int64(n)
	return n, nil
}

func (b *bytesReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case 0: // io.SeekStart
		newPos = offset
	case 1: // io.SeekCurrent
		newPos = b.pos + offset
	case 2: // io.SeekEnd
		newPos = int64(len(b.data)) + offset
	default:
		return 0, fmt.Errorf("invalid whence value")
	}
	if newPos < 0 || newPos > int64(len(b.data)) {
		return 0, fmt.Errorf("seek position out of bounds")
	}
	b.pos = newPos
	return newPos, nil
}