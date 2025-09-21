package domains

import (
	"context"
	"fmt"
	"time"

	"github.com/iw2rmb/ploy/internal/routing"
)

const domainStoreTimeout = 5 * time.Second

func (h *DomainHandler) storeDomainConfig(appName, domain string) error {
	store, err := h.routingStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), domainStoreTimeout)
	defer cancel()
	return store.AppendDomain(ctx, appName, domain)
}

func (h *DomainHandler) getStoredDomains(appName string) ([]string, error) {
	store, err := h.routingStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), domainStoreTimeout)
	defer cancel()
	return store.GetDomains(ctx, appName)
}

func (h *DomainHandler) removeDomainConfig(appName, domain string) error {
	store, err := h.routingStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), domainStoreTimeout)
	defer cancel()
	return store.RemoveDomain(ctx, appName, domain)
}

func (h *DomainHandler) routingStore() (*routing.Store, error) {
	if h.router == nil {
		return nil, fmt.Errorf("router not initialized")
	}
	store := h.router.GetRouteStore()
	if store == nil {
		return nil, fmt.Errorf("routing store unavailable")
	}
	return store, nil
}
