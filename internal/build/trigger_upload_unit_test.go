package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iw2rmb/ploy/internal/storage"
	mem "github.com/iw2rmb/ploy/internal/storage/providers/memory"
)

// flakyStorage wraps a storage.Storage and fails Put for the first N calls.
type flakyStorage struct {
	storage.Storage
	failCount int
	calls     int
}

func (f *flakyStorage) Put(ctx context.Context, key string, reader io.Reader, opts ...storage.PutOption) error {
	f.calls++
	if f.calls <= f.failCount {
		return fmt.Errorf("simulated put error")
	}
	return f.Storage.Put(ctx, key, reader, opts...)
}

func TestUploadFileWithUnifiedStorage_RetryThenSuccess(t *testing.T) {
	// speed up retries for unit test
	oldDelay := retryBaseDelay
	retryBaseDelay = 5 * time.Millisecond
	t.Cleanup(func() { retryBaseDelay = oldDelay })
	base := mem.NewMemoryStorage(0)
	flaky := &flakyStorage{Storage: base, failCount: 1}
	ctx := context.Background()

	tmp := t.TempDir()
	file := filepath.Join(tmp, "a.txt")
	require.NoError(t, os.WriteFile(file, []byte("hello"), 0644))

	start := time.Now()
	err := uploadFileWithUnifiedStorage(ctx, flaky, file, "k1", "text/plain")
	require.NoError(t, err)
	_ = start

	ok, err := base.Exists(ctx, "k1")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestUploadBytesWithUnifiedStorage_AllFail(t *testing.T) {
	oldDelay := retryBaseDelay
	retryBaseDelay = 5 * time.Millisecond
	t.Cleanup(func() { retryBaseDelay = oldDelay })
	base := mem.NewMemoryStorage(0)
	// Fail for all configured attempts to ensure we surface an error
	flaky := &flakyStorage{Storage: base, failCount: retryMaxAttempts}
	ctx := context.Background()

	data := []byte("payload")
	err := uploadBytesWithUnifiedStorage(ctx, flaky, data, "k2", "application/octet-stream")
	require.Error(t, err)
}
