# STORAGE

This document explains how Ploy abstracts storage using an S3-compatible API with **SeaweedFS** as the default backend.

## Goals
- Backend-agnostic via S3 API.
- Simple config; easy migration later.

## Configuration
See `configs/storage-config.yaml`.

### Local SeaweedFS (Homebrew)

For macOS dev machines that rely on the Homebrew formula:

1. Install the service: `brew install seaweedfs`.
2. Copy `configs/seaweedfs/homebrew-launchd.plist.example` to `~/Library/LaunchAgents/homebrew.mxcl.seaweedfs.plist`, replacing `@HOMEBREW_PREFIX@` with the output of `brew --prefix` and adjusting paths as needed.
3. (Re)load the customized service:
   ```bash
   launchctl unload ~/Library/LaunchAgents/homebrew.mxcl.seaweedfs.plist 2>/dev/null || true
   launchctl load -w ~/Library/LaunchAgents/homebrew.mxcl.seaweedfs.plist
   ```
4. After the filer is listening on `http://localhost:8888`, run `make seaweedfs-bootstrap` to provision the collections (`test-collection`, `artifacts`, `test-bucket`) used by unit tests and local dev flows.

The bootstrap step is idempotent and safe to run after every restart; it ensures the directories exist and pins each collection to replication `000` so local writes do not require additional volumes.

## Code
`internal/storage/storage.go` initializes an S3 client. Artifacts are uploaded under `artifacts/<app>/<sha>/` by the api.

## Migration
Mirror buckets with `mc mirror` or `rclone`, flip endpoint when ready.


---

# Appendix from ploy.md


Ploy Storage Abstraction Pack

This pack includes:
	1.	config.yaml template
	2.	storage.go (Go helper to load an S3-compatible client)
	3.	Integration notes (how to call it from your handlers/builders)
	4.	Migration strategy playbook
	5.	STORAGE.md (summary + comparison table + guidance)

⸻

1) config.yaml (template)

# Ploy storage configuration
storage:
  provider: s3                 # keep this "s3" to ensure portability
  endpoint: http://seaweedfs.ploy.local:8333  # SeaweedFS S3 API/Ceph RGW/AWS S3
  bucket: artifacts       # default artifact bucket
  region: us-east-1            # fake region is fine for most S3-compatible backends
  access_key: PLOYACCESSKEY
  secret_key: PLOYSECRETKEY
  path_style: true             # required for SeaweedFS/Ceph RGW
  # Optional features
  # server_side_encryption: "AES256"   # or "aws:kms" if using AWS S3
  # tls_insecure_skip_verify: false     # set true ONLY for dev

# Optional: separate buckets per domain
buckets:
  artifacts: artifacts
  logs: ploy-logs
  cache: ploy-cache

# Optional lifecycle defaults (Ploy can use these to pre-create rules)
lifecycle:
  logs_days: 30
  artifacts_days: 180
  cache_days: 7


⸻

2) storage.go (portable S3 client loader)

Drop this into internal/storage/storage.go (or similar) and wire it in your server setup.

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

// Config mirrors config.yaml
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

	// Optional named buckets
	Buckets struct {
		Artifacts string `yaml:"artifacts"`
		Logs      string `yaml:"logs"`
		Cache     string `yaml:"cache"`
	} `yaml:"buckets"`
}

// Client bundles S3 svc plus commonly used buckets
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
		tr.TLSClientConfig.InsecureSkipVerify = true // dev only
	}

	httpClient := &http.Client{ Transport: tr }

	sess, err := session.NewSession(&aws.Config{
		Credentials:      credentials.NewStaticCredentials(cfg.AccessKey, cfg.SecretKey, ""),
		Endpoint:         aws.String(cfg.Endpoint),
		Region:           aws.String(cfg.Region),
		S3ForcePathStyle: aws.Bool(cfg.PathStyle),
		HTTPClient:       httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("create aws session: %w", err)
	}

	c := &Client{
		S3:        s3.New(sess),
		Artifacts: firstNonEmpty(cfg.Buckets.Artifacts, cfg.Bucket),
		Logs:      cfg.Buckets.Logs,
		Cache:     cfg.Buckets.Cache,
		SSE:       cfg.SSE,
	}
	return c, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals { if v != "" { return v } }
	return ""
}

// PutObject convenience with optional SSE
func (c *Client) PutObject(bucket, key string, body aws.ReadSeekCloser, contentType string) (*s3.PutObjectOutput, error) {
	in := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	}
	if c.SSE != "" { in.ServerSideEncryption = aws.String(c.SSE) }
	return c.S3.PutObject(in)
}

// GetObject convenience
func (c *Client) GetObject(bucket, key string) (*s3.GetObjectOutput, error) {
	return c.S3.GetObject(&s3.GetObjectInput{ Bucket: aws.String(bucket), Key: aws.String(key) })
}

// CreateBucketIfMissing is useful in dev environments
func (c *Client) CreateBucketIfMissing(bucket string) error {
	_, err := c.S3.HeadBucket(&s3.HeadBucketInput{ Bucket: aws.String(bucket) })
	if err == nil { return nil }
	_, err = c.S3.CreateBucket(&s3.CreateBucketInput{ Bucket: aws.String(bucket) })
	return err
}

Minimal server wiring (example)

// in cmd/ploy-server/main.go

var cfg struct {
	Storage storage.Config `yaml:"storage"`
}

// after loading YAML into cfg ...
s3c, err := storage.New(cfg.Storage)
if err != nil { panic(err) }

// ensure buckets exist in dev
_ = s3c.CreateBucketIfMissing(s3c.Artifacts)
if s3c.Logs != "" { _ = s3c.CreateBucketIfMissing(s3c.Logs) }

Example usage in a build handler

// Save build artifact
data := bytes.NewReader(artifactBytes)
_, err := s3c.PutObject(s3c.Artifacts, fmt.Sprintf("%s/%s", appID, artifactName),
	aws.ReadSeekCloser(data), "application/octet-stream")
if err != nil { return err }


⸻

3) Dual-write (optional) for migration

If you want seamless cutovers later, add a second client and write to both during a transition window.

// Pseudo: if cfg.Secondary is present, mirror writes
if s3c2 != nil {
	go func(){ _, _ = s3c2.PutObject(bucket, key, aws.ReadSeekCloser(bytes.NewReader(buf)), ct) }()
}


⸻

4) Migration Strategy (Storage Backend Migration)

Principles
	•	Treat storage as an S3 endpoint only. No vendor-specific calls.
	•	Keep bucket versioning on from day 1 to avoid overwrite races.
	•	Prefer DNS indirection (e.g., s3.ploy.local) so a cutover is a single DNS change.

Steps
	1.	Deploy target backend (Alternative S3-compatible storage).
	2.	Mirror: rclone sync s3:source/artifacts s3:target/artifacts (or mc mirror).
	3.	Dual-write (optional): enable temporary dual writes for new artifacts.
	4.	Smoke-test: run ploy push + artifact fetch against target.
	5.	Flip: change endpoint in config.yaml or repoint DNS.
	6.	Read-only tail: keep old endpoint read-only for N days.

Edge checks
	•	Multipart upload semantics (ploy push tar streaming).
	•	SSE (server-side encryption) parity.
	•	Bucket policies/IAM differences; keep client creds thin (access/secret only).

⸻

5) STORAGE.md (ready-to-commit)

# STORAGE

This document explains how Ploy abstracts storage, what backends are recommended, and how to migrate between them.

## Goals
- Keep Ploy **backend-agnostic** using the **S3 API** as the single interface.
- Optimize for **developer speed** (small objects, fast artifact IO).
- Allow **easy migration** (self-hosted ↔ managed, SeaweedFS ↔ other S3-compatible backends).

## Configuration
See `config.yaml` for a minimal, portable configuration. Key flags:
- `endpoint`: S3-compatible URL (SeaweedFS, Ceph RGW, AWS S3, Wasabi, B2).
- `path_style: true`: required for most non-AWS endpoints.
- `region`: any string; defaults work for non-AWS.
- `server_side_encryption`: optional (e.g., `AES256`).

## Supported Backends (Comparison)

| Solution      | License     | Deployment Ease                 | Performance Notes                               | Interfaces        | Pros                                              | Cons                                           |
|---------------|-------------|---------------------------------|--------------------------------------------------|-------------------|---------------------------------------------------|------------------------------------------------|
| **SeaweedFS** | Apache 2.0  | ✅ Single binary; simple cluster | Excellent small-object perf; in-memory metadata  | S3 + Filer (POSIX) | Permissive license; FS+Object; simple ops        | Smaller ecosystem                              |
| **Garage**    | AGPLv3      | ✅ Simple; Rust binary           | Low-latency; 2–10 node clusters                  | S3 only           | Minimal footprint; self-healing                  | AGPLv3; limited scale                          |
| **Zenko**     | Apache 2.0  | ❌ Multi-service                 | Enterprise-grade; multi-cloud replication        | S3 + connectors   | Multi-cloud sync and policy                       | Heavier ops                                    |
| **LeoFS**     | Apache 2.0  | ⚠ Erlang cluster                 | Stable; high concurrency                         | S3 only           | Reliable; niche deployments                      | Smaller community                              |
| **JuiceFS**   | Apache 2.0  | ⚠ Client+metadata DB             | Great sequential IO; strong FS semantics         | POSIX + S3 gw     | POSIX + S3 hybrid                                | Metadata dependency; more moving parts         |
| **Ceph (RGW)**| LGPLv2.1    | ❌ Heavier (cephadm/Rook)        | Good large-object; slower small-object latency   | S3 + Block + File | Unified storage; battle-tested                    | Complex ops; resource-heavy                    |
| **Swift**     | Apache 2.0  | ❌ Complex                        | Stable; slower vs SeaweedFS                      | Object only       | Proven at scale                                  | Legacy feel; less active community             |
| **Managed S3**| Proprietary | ✅ Zero deployment                | Scales transparently; WAN latency considerations | S3 API            | No ops burden; global availability               | Latency; lock-in; cost                         |

## Recommended Paths
- **Dev/Test (fast, light)**: SeaweedFS (default for Ploy).
- **License-sensitive**: SeaweedFS (Apache 2.0).
- **Need Block/POSIX later**: Ceph (add via Rook when required).
- **Production**: SeaweedFS for most workloads; consider managed S3 for global scale.

## Migration Playbook (Storage Backend Migration)
1. Deploy target backend.
2. Mirror buckets (`rclone sync` or `mc mirror`).
3. Optional dual write for new artifacts.
4. Smoke-test `ploy push`/fetch.
5. Change `endpoint` (or flip DNS).
6. Keep old backend read-only for N days.

## Operational Defaults
- Enable **bucket versioning** and **lifecycle** (logs: 30d, artifacts: 180d, cache: 7d).
- Monitor 4xx/5xx error rates and PUT/GET latencies in CI.

## FAQ
- **Why path-style addressing?** Better compatibility with non-AWS S3.
- **Do we need real regions?** Not for non-AWS; use `us-east-1` by convention.
- **Encryption?** Set `server_side_encryption: AES256` if the backend supports SSE.


⸻

6) Next Steps
	•	Commit config.yaml, internal/storage/storage.go, and STORAGE.md.
	•	Wire the client into build apis (artifact upload/download) and logs.
	•	(Optional) Add a --storage-config CLI flag to override the path per environment.
