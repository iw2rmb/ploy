package storage

import (
	"crypto/tls"
	"fmt"
	"net/http"

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

func (c *Client) PutObject(bucket, key string, body aws.ReadSeekCloser, contentType string) (*s3.PutObjectOutput, error) {
	in := &s3.PutObjectInput{ Bucket: aws.String(bucket), Key: aws.String(key), Body: body, ContentType: aws.String(contentType) }
	if c.SSE != "" { in.ServerSideEncryption = aws.String(c.SSE) }
	return c.S3.PutObject(in)
}
