package storage

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Config struct {
	Provider   string `yaml:"provider"` // "minio", "seaweedfs", "s3"
	Endpoint   string `yaml:"endpoint"`
	Bucket     string `yaml:"bucket"`
	Region     string `yaml:"region"`
	AccessKey  string `yaml:"access_key"`
	SecretKey  string `yaml:"secret_key"`
	PathStyle  bool   `yaml:"path_style"`
	SSE        string `yaml:"server_side_encryption"`
	TLSInsecure bool  `yaml:"tls_insecure_skip_verify"`
	Buckets struct {
		Artifacts string `yaml:"artifacts"`
		Logs      string `yaml:"logs"`
		Cache     string `yaml:"cache"`
	} `yaml:"buckets"`
	
	// SeaweedFS-specific configuration
	SeaweedFS SeaweedFSConfig `yaml:"seaweedfs,omitempty"`
	
	// Hybrid storage configuration
	Hybrid HybridConfig `yaml:"hybrid,omitempty"`
}

type SeaweedFSConfig struct {
	Master      string `yaml:"master"`      // master server address (e.g., "localhost:9333")
	Filer       string `yaml:"filer"`       // filer server address (e.g., "localhost:8888")
	Collection  string `yaml:"collection"`  // collection name for artifacts
	Replication string `yaml:"replication"` // replication strategy (e.g., "001")
	Timeout     int    `yaml:"timeout"`     // timeout in seconds
}

type HybridConfig struct {
	Enabled        bool   `yaml:"enabled"`
	PrimaryProvider string `yaml:"primary_provider"`   // "seaweedfs" or "minio"
	DualWrite      bool   `yaml:"dual_write"`          // write to both providers
	FallbackRead   bool   `yaml:"fallback_read"`       // fallback to secondary on read failure
}

// MinIOClient implements StorageProvider for MinIO/S3 backends
type MinIOClient struct {
	S3        *s3.S3
	Artifacts string
	Logs      string
	Cache     string
	SSE       string
}

// Ensure MinIOClient implements StorageProvider
var _ StorageProvider = (*MinIOClient)(nil)

// Legacy Client type for backward compatibility
type Client = MinIOClient

func New(cfg Config) (*Client, error) {
	switch cfg.Provider {
	case "seaweedfs":
		return NewSeaweedFSClient(cfg)
	case "hybrid":
		return NewHybridClient(cfg)
	default: // "minio", "s3", or empty (default to MinIO)
		return NewMinIOClient(cfg)
	}
}

func NewMinIOClient(cfg Config) (*Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLSInsecure {
		if tr.TLSClientConfig == nil { tr.TLSClientConfig = &tls.Config{} }
		tr.TLSClientConfig.InsecureSkipVerify = true
	}
	httpClient := &http.Client{ Transport: tr }

	sess, err := session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, ""),
		Endpoint:         aws.String(cfg.Endpoint),
		Region:           aws.String(cfg.Region),
		S3ForcePathStyle: aws.Bool(cfg.PathStyle),
		HTTPClient:       httpClient,
	})
	if err != nil { return nil, fmt.Errorf("create aws session: %w", err) }

	c := &Client{
		S3:        s3.New(sess),
		Artifacts: first(cfg.Buckets.Artifacts, cfg.Bucket),
		Logs:      cfg.Buckets.Logs,
		Cache:     cfg.Buckets.Cache,
		SSE:       cfg.SSE,
	}
	return c, nil
}

func first(vals ...string) string { for _, v := range vals { if v != "" { return v } } ; return "" }

// StorageProvider interface implementation for MinIOClient

func (c *MinIOClient) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*PutObjectResult, error) {
	in := &s3.PutObjectInput{ 
		Bucket: aws.String(bucket), 
		Key: aws.String(key), 
		Body: body, 
		ContentType: aws.String(contentType),
	}
	if c.SSE != "" { 
		in.ServerSideEncryption = aws.String(c.SSE) 
	}
	
	output, err := c.S3.PutObject(in)
	if err != nil {
		return nil, err
	}
	
	result := &PutObjectResult{
		Location: fmt.Sprintf("%s/%s", bucket, key),
	}
	if output.ETag != nil {
		result.ETag = *output.ETag
	}
	
	// Get file size if available
	if body != nil {
		if seeker, ok := body.(io.Seeker); ok {
			if current, err := seeker.Seek(0, io.SeekCurrent); err == nil {
				if end, err := seeker.Seek(0, io.SeekEnd); err == nil {
					result.Size = end
					seeker.Seek(current, io.SeekStart) // Reset position
				}
			}
		}
	}
	
	return result, nil
}

func (c *MinIOClient) GetObject(bucket, key string) (io.ReadCloser, error) {
	output, err := c.S3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return output.Body, nil
}

func (c *MinIOClient) ListObjects(bucket, prefix string) ([]ObjectInfo, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	}
	
	var objects []ObjectInfo
	err := c.S3.ListObjectsV2Pages(input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			info := ObjectInfo{
				Key:  *obj.Key,
				Size: *obj.Size,
			}
			if obj.LastModified != nil {
				info.LastModified = obj.LastModified.Format(time.RFC3339)
			}
			if obj.ETag != nil {
				info.ETag = *obj.ETag
			}
			objects = append(objects, info)
		}
		return !lastPage
	})
	
	return objects, err
}

func (c *MinIOClient) GetProviderType() string {
	return "minio"
}

func (c *MinIOClient) GetArtifactsBucket() string {
	return c.Artifacts
}

// Legacy PutObject method for backward compatibility
func (c *Client) PutObjectLegacy(bucket, key string, body io.ReadSeeker, contentType string) (*s3.PutObjectOutput, error) {
	in := &s3.PutObjectInput{ Bucket: aws.String(bucket), Key: aws.String(key), Body: body, ContentType: aws.String(contentType) }
	if c.SSE != "" { in.ServerSideEncryption = aws.String(c.SSE) }
	return c.S3.PutObject(in)
}

// UploadArtifactBundle uploads an artifact and all its related files (SBOM, signature, certificate)
func (c *MinIOClient) UploadArtifactBundle(keyPrefix, artifactPath string) error {
	// Upload main artifact
	if err := c.uploadFile(keyPrefix, artifactPath, "application/octet-stream"); err != nil {
		return fmt.Errorf("failed to upload artifact %s: %w", artifactPath, err)
	}

	// Upload SBOM if exists
	sbomPath := artifactPath + ".sbom.json"
	if _, err := os.Stat(sbomPath); err == nil {
		if err := c.uploadFile(keyPrefix, sbomPath, "application/json"); err != nil {
			return fmt.Errorf("failed to upload SBOM %s: %w", sbomPath, err)
		}
	}

	// Upload signature if exists
	sigPath := artifactPath + ".sig"
	if _, err := os.Stat(sigPath); err == nil {
		if err := c.uploadFile(keyPrefix, sigPath, "application/octet-stream"); err != nil {
			return fmt.Errorf("failed to upload signature %s: %w", sigPath, err)
		}
	}

	// Upload certificate if exists (from keyless OIDC signing)
	crtPath := artifactPath + ".crt"
	if _, err := os.Stat(crtPath); err == nil {
		if err := c.uploadFile(keyPrefix, crtPath, "application/x-pem-file"); err != nil {
			return fmt.Errorf("failed to upload certificate %s: %w", crtPath, err)
		}
	}

	return nil
}

func (c *MinIOClient) VerifyUpload(key string) error {
	_, err := c.S3.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(c.Artifacts),
		Key:    aws.String(key),
	})
	return err
}

// uploadFile is a helper function to upload a single file with retry logic
func (c *MinIOClient) uploadFile(keyPrefix, filePath, contentType string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	key := keyPrefix + filepath.Base(filePath)
	
	// Get file info for verification
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	
	// Retry upload up to 3 times
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// Reset file pointer to beginning
		if _, err := file.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to reset file pointer: %w", err)
		}
		
		result, err := c.PutObject(c.Artifacts, key, file, contentType)
		if err != nil {
			lastErr = err
			fmt.Printf("Upload attempt %d failed for %s: %v\n", attempt, key, err)
			continue
		}
		
		// Verify upload by checking if ETag is present (indicates successful upload)
		if result.ETag != "" {
			fmt.Printf("Successfully uploaded %s (size: %d bytes, ETag: %s)\n", key, fileInfo.Size(), result.ETag)
			return nil
		}
		
		lastErr = fmt.Errorf("upload completed but no ETag received")
	}
	
	return fmt.Errorf("failed to upload after 3 attempts: %w", lastErr)
}

