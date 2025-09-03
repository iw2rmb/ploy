package config

import (
    "log"

    consulapi "github.com/hashicorp/consul/api"
    yamlv3 "gopkg.in/yaml.v3"
)

// consulSource loads configuration YAML from Consul KV at a given prefix/key.
// It is tolerant to connectivity errors unless required is true.
type consulSource struct {
    addr     string
    key      string
    priority int
    required bool
}

func (c *consulSource) Name() string  { return "consul" }
func (c *consulSource) Priority() int { return c.priority }

func (c *consulSource) Load() (map[string]interface{}, error) {
    cfg := consulapi.DefaultConfig()
    if c.addr != "" {
        cfg.Address = c.addr
    }
    client, err := consulapi.NewClient(cfg)
    if err != nil {
        if c.required {
            return nil, err
        }
        log.Printf("config: consul source disabled (client error): %v", err)
        return map[string]interface{}{}, nil
    }
    kv := client.KV()
    pair, _, err := kv.Get(c.key, nil)
    if err != nil {
        if c.required {
            return nil, err
        }
        log.Printf("config: consul source '%s' read failed: %v", c.key, err)
        return map[string]interface{}{}, nil
    }
    if pair == nil || len(pair.Value) == 0 {
        if c.required {
            return map[string]interface{}{}, nil
        }
        return map[string]interface{}{}, nil
    }

    // Decode YAML value
    var out map[string]interface{}
    if err := yamlv3.Unmarshal(pair.Value, &out); err != nil {
        if c.required {
            return nil, err
        }
        log.Printf("config: consul source yaml decode failed: %v", err)
        return map[string]interface{}{}, nil
    }
    if out == nil {
        out = map[string]interface{}{}
    }
    return out, nil
}

