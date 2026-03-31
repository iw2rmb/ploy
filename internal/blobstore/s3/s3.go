// Package s3 provides an S3-compatible implementation of blobstore.Store.
package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/iw2rmb/ploy/internal/blobstore"
	"github.com/iw2rmb/ploy/internal/server/config"
)

const defaultRegion = "us-east-1"

// Store implements blobstore.Store using an S3-compatible backend.
type Store struct {
	client *awss3.Client
	bucket string
}

// Ensure Store implements blobstore.Store.
var _ blobstore.Store = (*Store)(nil)

// New creates a new S3 blobstore.Store.
func New(cfg config.ObjectStoreConfig) (*Store, error) {
	endpoint, err := normalizeEndpoint(cfg.Endpoint, cfg.Secure)
	if err != nil {
		return nil, err
	}
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, fmt.Errorf("s3: bucket is required")
	}
	accessKey := strings.TrimSpace(cfg.AccessKey)
	if accessKey == "" {
		return nil, fmt.Errorf("s3: access key is required")
	}
	secretKey := strings.TrimSpace(cfg.SecretKey)
	if secretKey == "" {
		return nil, fmt.Errorf("s3: secret key is required")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = defaultRegion
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
		awsconfig.WithResponseChecksumValidation(aws.ResponseChecksumValidationWhenRequired),
	)
	if err != nil {
		return nil, fmt.Errorf("s3: load aws config: %w", err)
	}

	client := awss3.NewFromConfig(awsCfg, func(o *awss3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})

	return &Store{
		client: client,
		bucket: bucket,
	}, nil
}

// Put uploads data to S3 at the given key.
func (s *Store) Put(ctx context.Context, key, contentType string, data []byte) (string, error) {
	input := &awss3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String(contentType),
	}
	output, err := s.client.PutObject(ctx, input)
	if err != nil {
		return "", fmt.Errorf("s3: put object %s: %w", key, err)
	}

	var etag string
	if output.ETag != nil {
		etag = *output.ETag
	}
	return etag, nil
}

// Get retrieves an object from S3.
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, int64, error) {
	output, err := s.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *s3types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, 0, fmt.Errorf("s3: get object %s: %w", key, blobstore.ErrNotFound)
		}
		return nil, 0, fmt.Errorf("s3: get object %s: %w", key, err)
	}

	var size int64
	if output.ContentLength != nil {
		size = *output.ContentLength
	}
	return output.Body, size, nil
}

// Delete removes an object from S3.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3: delete object %s: %w", key, err)
	}
	return nil
}

func normalizeEndpoint(raw string, secure bool) (string, error) {
	endpoint := strings.TrimSpace(raw)
	if endpoint == "" {
		return "", fmt.Errorf("s3: endpoint is required")
	}

	if !strings.Contains(endpoint, "://") {
		scheme := "http"
		if secure {
			scheme = "https"
		}
		endpoint = scheme + "://" + endpoint
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("s3: parse endpoint %q: %w", endpoint, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("s3: endpoint scheme must be http or https: %q", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("s3: endpoint host is required")
	}

	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}
