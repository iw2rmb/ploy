package storage

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Config struct {
	Provider   string `yaml:"provider"`
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
}

type Client struct {
	S3        *s3.S3
	Artifacts string
	Logs      string
	Cache     string
	SSE       string
}

func New(cfg Config) (*Client, error) {
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

func (c *Client) PutObject(bucket, key string, body io.ReadSeeker, contentType string) (*s3.PutObjectOutput, error) {
	in := &s3.PutObjectInput{ Bucket: aws.String(bucket), Key: aws.String(key), Body: body, ContentType: aws.String(contentType) }
	if c.SSE != "" { in.ServerSideEncryption = aws.String(c.SSE) }
	return c.S3.PutObject(in)
}

// UploadArtifactBundle uploads an artifact and all its related files (SBOM, signature, certificate)
func (c *Client) UploadArtifactBundle(keyPrefix, artifactPath string) error {
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

// uploadFile is a helper function to upload a single file with retry logic
func (c *Client) uploadFile(keyPrefix, filePath, contentType string) error {
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
		
		output, err := c.PutObject(c.Artifacts, key, file, contentType)
		if err != nil {
			lastErr = err
			fmt.Printf("Upload attempt %d failed for %s: %v\n", attempt, key, err)
			continue
		}
		
		// Verify upload by checking if ETag is present (indicates successful upload)
		if output.ETag != nil && *output.ETag != "" {
			fmt.Printf("Successfully uploaded %s (size: %d bytes, ETag: %s)\n", key, fileInfo.Size(), *output.ETag)
			return nil
		}
		
		lastErr = fmt.Errorf("upload completed but no ETag received")
	}
	
	return fmt.Errorf("failed to upload after 3 attempts: %w", lastErr)
}

// VerifyUpload checks if an object exists in storage
func (c *Client) VerifyUpload(key string) error {
	_, err := c.S3.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(c.Artifacts),
		Key:    aws.String(key),
	})
	return err
}
