package orchestration

import (
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

	consulapi "github.com/hashicorp/consul/api"
	"github.com/iw2rmb/ploy/internal/utils"
	"github.com/nats-io/nats.go"
)

// KV provides minimal Consul KV operations used by higher layers
type KV interface {
	Put(key string, value []byte) error
	Get(key string) ([]byte, error)
	Keys(prefix, separator string) ([]string, error)
	Delete(key string) error
}

// NewKV returns the configured KV adapter. Defaults to Consul unless
// PLOY_USE_JETSTREAM_KV is truthy, in which case a JetStream-backed adapter is
// attempted. Failures fall back to Consul.
func NewKV() KV {
	if useJetstreamKV() {
		if kv, err := newJetstreamKV(); err != nil {
			log.Printf("orchestration: jetstream KV unavailable, falling back to Consul: %v", err)
		} else if kv != nil {
			return kv
		}
	}
	return &consulKV{client: newConsul()}
}

type consulKV struct{ client *consulapi.Client }

func newConsul() *consulapi.Client {
	cfg := consulapi.DefaultConfig()
	if addr := utils.Getenv("CONSUL_ADDR", ""); addr != "" {
		cfg.Address = addr
	}
	c, _ := consulapi.NewClient(cfg)
	return c
}

func (k *consulKV) Put(key string, value []byte) error {
	if k.client == nil {
		return fmt.Errorf("consul client unavailable")
	}
	_, err := k.client.KV().Put(&consulapi.KVPair{Key: key, Value: value}, nil)
	return err
}

func (k *consulKV) Get(key string) ([]byte, error) {
	if k.client == nil {
		return nil, fmt.Errorf("consul client unavailable")
	}
	p, _, err := k.client.KV().Get(key, nil)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	return p.Value, nil
}

func (k *consulKV) Keys(prefix, separator string) ([]string, error) {
	if k.client == nil {
		return nil, fmt.Errorf("consul client unavailable")
	}
	keys, _, err := k.client.KV().Keys(prefix, separator, nil)
	return keys, err
}

func (k *consulKV) Delete(key string) error {
	if k.client == nil {
		return fmt.Errorf("consul client unavailable")
	}
	_, err := k.client.KV().Delete(key, nil)
	return err
}

type jetstreamKV struct {
	conn   *nats.Conn
	bucket nats.KeyValue
}

func useJetstreamKV() bool {
	switch strings.ToLower(utils.Getenv("PLOY_USE_JETSTREAM_KV", "")) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func newJetstreamKV() (*jetstreamKV, error) {
	url := utils.Getenv("PLOY_JETSTREAM_URL", "")
	if url == "" {
		url = utils.Getenv("NATS_ADDR", nats.DefaultURL)
	}
	if url == "" {
		return nil, fmt.Errorf("jetstream url not configured")
	}

	opts := []nats.Option{nats.Name("ploy-jetstream-kv")}
	if creds := utils.Getenv("PLOY_JETSTREAM_CREDS", ""); creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	}
	user := utils.Getenv("PLOY_JETSTREAM_USER", "")
	if user != "" {
		opts = append(opts, nats.UserInfo(user, utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")))
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, err
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, err
	}

	bucketName := utils.Getenv("PLOY_JETSTREAM_KV_BUCKET", "ploy_kv")
	bucket, err := js.KeyValue(bucketName)
	if errors.Is(err, nats.ErrBucketNotFound) {
		bucket, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: bucketName})
	}
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &jetstreamKV{conn: conn, bucket: bucket}, nil
}

func (k *jetstreamKV) Put(key string, value []byte) error {
	if k == nil || k.bucket == nil {
		return fmt.Errorf("jetstream bucket unavailable")
	}
	_, err := k.bucket.Put(key, value)
	return err
}

func (k *jetstreamKV) Get(key string) ([]byte, error) {
	if k == nil || k.bucket == nil {
		return nil, fmt.Errorf("jetstream bucket unavailable")
	}
	entry, err := k.bucket.Get(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entry.Value(), nil
}

func (k *jetstreamKV) Keys(prefix, separator string) ([]string, error) {
	if k == nil || k.bucket == nil {
		return nil, fmt.Errorf("jetstream bucket unavailable")
	}
	keys, err := k.bucket.Keys()
	if errors.Is(err, nats.ErrKeyNotFound) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(keys))
	seen := make(map[string]struct{})
	for _, key := range keys {
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		candidate := key
		if prefix != "" {
			relative := strings.TrimPrefix(key, prefix)
			if separator != "" {
				if idx := strings.Index(relative, separator); idx >= 0 {
					candidate = prefix + relative[:idx+len(separator)]
				} else {
					candidate = prefix + relative
				}
			} else {
				candidate = prefix + relative
			}
		} else if separator != "" {
			if idx := strings.Index(candidate, separator); idx >= 0 {
				candidate = candidate[:idx+len(separator)]
			}
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		filtered = append(filtered, candidate)
	}
	sort.Strings(filtered)
	return filtered, nil
}

func (k *jetstreamKV) Delete(key string) error {
	if k == nil || k.bucket == nil {
		return fmt.Errorf("jetstream bucket unavailable")
	}
	err := k.bucket.Delete(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return nil
	}
	return err
}
