package orchestration

import (
    "fmt"

    consulapi "github.com/hashicorp/consul/api"
    "github.com/iw2rmb/ploy/internal/utils"
)

// KV provides minimal Consul KV operations used by higher layers
type KV interface {
    Put(key string, value []byte) error
    Get(key string) ([]byte, error)
    Keys(prefix, separator string) ([]string, error)
    Delete(key string) error
}

// NewKV returns a Consul KV adapter using CONSUL_ADDR (or default)
func NewKV() KV { return &consulKV{client: newConsul()} }

type consulKV struct{ client *consulapi.Client }

func newConsul() *consulapi.Client {
    cfg := consulapi.DefaultConfig()
    if addr := utils.Getenv("CONSUL_ADDR", ""); addr != "" { cfg.Address = addr }
    c, _ := consulapi.NewClient(cfg)
    return c
}

func (k *consulKV) Put(key string, value []byte) error {
    if k.client == nil { return fmt.Errorf("consul client unavailable") }
    _, err := k.client.KV().Put(&consulapi.KVPair{Key: key, Value: value}, nil)
    return err
}

func (k *consulKV) Get(key string) ([]byte, error) {
    if k.client == nil { return nil, fmt.Errorf("consul client unavailable") }
    p, _, err := k.client.KV().Get(key, nil)
    if err != nil { return nil, err }
    if p == nil { return nil, nil }
    return p.Value, nil
}

func (k *consulKV) Keys(prefix, separator string) ([]string, error) {
    if k.client == nil { return nil, fmt.Errorf("consul client unavailable") }
    keys, _, err := k.client.KV().Keys(prefix, separator, nil)
    return keys, err
}

func (k *consulKV) Delete(key string) error {
    if k.client == nil { return fmt.Errorf("consul client unavailable") }
    _, err := k.client.KV().Delete(key, nil)
    return err
}

