package domains

import (
	"encoding/json"
	"fmt"

	consulapi "github.com/hashicorp/consul/api"
)

func (h *DomainHandler) storeDomainConfig(appName, domain string) error {
	if h.router == nil {
		return fmt.Errorf("router not initialized")
	}

	domains, err := h.getStoredDomains(appName)
	if err != nil {
		return err
	}

	for _, existing := range domains {
		if existing == domain {
			return nil
		}
	}

	domains = append(domains, domain)

	key := fmt.Sprintf("ploy/domains/%s/config", appName)
	data, err := json.Marshal(domains)
	if err != nil {
		return fmt.Errorf("failed to marshal domains: %w", err)
	}

	pair := &consulapi.KVPair{Key: key, Value: data}
	if _, err := h.router.GetConsulClient().KV().Put(pair, nil); err != nil {
		return fmt.Errorf("failed to store domain config in Consul KV: %w", err)
	}
	return nil
}

func (h *DomainHandler) getStoredDomains(appName string) ([]string, error) {
	if h.router == nil {
		return nil, fmt.Errorf("router not initialized")
	}

	key := fmt.Sprintf("ploy/domains/%s/config", appName)
	pair, _, err := h.router.GetConsulClient().KV().Get(key, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain config: %w", err)
	}
	if pair == nil {
		return []string{}, nil
	}

	var domains []string
	if err := json.Unmarshal(pair.Value, &domains); err != nil {
		return nil, fmt.Errorf("failed to unmarshal domain config: %w", err)
	}
	return domains, nil
}

func (h *DomainHandler) removeDomainConfig(appName, domain string) error {
	domains, err := h.getStoredDomains(appName)
	if err != nil {
		return err
	}

	filtered := make([]string, 0, len(domains))
	for _, d := range domains {
		if d != domain {
			filtered = append(filtered, d)
		}
	}

	key := fmt.Sprintf("ploy/domains/%s/config", appName)
	data, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("failed to marshal domains: %w", err)
	}

	pair := &consulapi.KVPair{Key: key, Value: data}
	if _, err := h.router.GetConsulClient().KV().Put(pair, nil); err != nil {
		return fmt.Errorf("failed to update domain config in Consul KV: %w", err)
	}
	return nil
}
