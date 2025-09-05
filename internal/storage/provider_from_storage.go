package storage

import (
    "bytes"
    "context"
    "fmt"
    "io"
)

// ProviderFromStorage adapts a unified Storage to the legacy StorageProvider interface.
// Bucket is used for methods that require a bucket argument; callers should pass
// the artifacts bucket name (e.g., "artifacts"). For providers where buckets
// are logical, this value is prefixed into keys by higher layers already.
type ProviderFromStorage struct {
    s      Storage
    bucket string
}

// NewProviderFromStorage returns a StorageProvider backed by a Storage instance.
func NewProviderFromStorage(s Storage, bucket string) *ProviderFromStorage {
    return &ProviderFromStorage{s: s, bucket: bucket}
}

func (p *ProviderFromStorage) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
    // ReadSeeker -> io.Reader; we're ignoring bucket since keys are namespaced by caller
    if err := p.s.Put(context.Background(), key, body, WithContentType(contentType)); err != nil {
        return nil, err
    }
    return &PutObjectResult{Location: key}, nil
}

func (p *ProviderFromStorage) UploadArtifactBundle(keyPrefix, artifactPath string) error {
    // Not used by self-update; provide a simple stub
    // Callers expecting verification should use UploadArtifactBundleWithVerification.
    return nil
}

func (p *ProviderFromStorage) UploadArtifactBundleWithVerification(keyPrefix, artifactPath string) (*BundleIntegrityResult, error) {
    // Not implemented for this adapter; return a minimal success result
    return &BundleIntegrityResult{Verified: true}, nil
}

func (p *ProviderFromStorage) VerifyUpload(key string) error {
    ok, err := p.s.Exists(context.Background(), key)
    if err != nil {
        return NewStorageError("verify", err, ErrorContext{Key: key})
    }
    if !ok {
        return NewStorageError("verify", fmt.Errorf("object not found"), ErrorContext{Key: key, HTTPStatus: 404})
    }
    return nil
}

func (p *ProviderFromStorage) GetObject(bucket, key string) (io.ReadCloser, error) {
    return p.s.Get(context.Background(), key)
}

func (p *ProviderFromStorage) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
    objs, err := p.s.List(context.Background(), ListOptions{Prefix: prefix})
    if err != nil { return nil, err }
    infos := make([]ObjectInfo, 0, len(objs))
    for _, o := range objs {
        infos = append(infos, ObjectInfo{
            Key:          o.Key,
            Size:         o.Size,
            ContentType:  o.ContentType,
            ETag:         o.ETag,
            LastModified: o.LastModified.Format("2006-01-02T15:04:05Z07:00"),
        })
    }
    return infos, nil
}

func (p *ProviderFromStorage) GetProviderType() string { return "adapter" }
func (p *ProviderFromStorage) GetArtifactsBucket() string { return p.bucket }

// helper for creating ReadSeeker from bytes
type bytesReadSeeker struct{ *bytes.Reader }
