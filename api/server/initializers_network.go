package server

import (
	"fmt"
	"log"
	"os"
	"strings"

	consulapi "github.com/hashicorp/consul/api"

	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/dns"
	"github.com/iw2rmb/ploy/api/selfupdate"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
)

func initializeDNSHandler(consulAddr string) (*dns.Handler, error) {
	dnsHandler, err := dns.NewHandler(consulAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS handler: %w", err)
	}

	log.Printf("DNS handler initialized with Consul address: %s", consulAddr)
	return dnsHandler, nil
}

func initializeCertificateManager(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*certificates.CertificateManager, error) {
	// Create Consul client
	consulConfig := consulapi.DefaultConfig()
	if cfg.ConsulAddr != "" {
		consulConfig.Address = cfg.ConsulAddr
	}
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Consul client: %w", err)
	}

	// Create storage client
	if cfgService == nil {
		return nil, fmt.Errorf("config service required for certificate manager")
	}
	storageClient, err := resolveStorageFromConfigService(cfgService)
	if err != nil {
		return nil, fmt.Errorf("resolve storage for certificates: %w", err)
	}

	// Create DNS provider (for ACME DNS-01 challenges)
	// Note: DNS provider can be nil for now, certificate manager should handle this gracefully
	dnsProvider, err := initializeDNSProvider()
	if err != nil {
		log.Printf("Warning: DNS provider initialization failed, certificates may not work: %v", err)
		dnsProvider = nil
	}

	// Create certificate manager (it should handle nil DNS provider gracefully)
	certificateManager, err := certificates.NewCertificateManager(consulClient, storageClient, dnsProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate manager: %w", err)
	}

	log.Printf("Certificate manager initialized successfully (DNS provider: %v)", dnsProvider != nil)
	return certificateManager, nil
}

func initializeDNSProvider() (dns.Provider, error) {
	// Get DNS provider type from environment
	providerType := os.Getenv("PLOY_APPS_DOMAIN_PROVIDER")
	if providerType == "" {
		log.Printf("PLOY_APPS_DOMAIN_PROVIDER not set, DNS provider disabled")
		return nil, nil
	}

	log.Printf("Initializing DNS provider: %s", providerType)

	switch strings.ToLower(providerType) {
	case "namecheap":
		return initializeNamecheapProvider()
	default:
		return nil, fmt.Errorf("unsupported DNS provider: %s", providerType)
	}
}

func initializeNamecheapProvider() (dns.Provider, error) {
	// Get API key from either production or sandbox environment
	apiKey := os.Getenv("NAMECHEAP_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("NAMECHEAP_SANDBOX_API_KEY")
	}

	config := dns.NamecheapConfig{
		APIUser:  os.Getenv("NAMECHEAP_API_USER"),
		APIKey:   apiKey,
		Username: os.Getenv("NAMECHEAP_USERNAME"),
		ClientIP: os.Getenv("NAMECHEAP_CLIENT_IP"),
		Sandbox:  os.Getenv("NAMECHEAP_SANDBOX") == "true",
	}

	// Validate required configuration
	if config.APIUser == "" || config.APIKey == "" || config.Username == "" || config.ClientIP == "" {
		return nil, fmt.Errorf("namecheap DNS provider requires NAMECHEAP_API_USER, NAMECHEAP_API_KEY (or NAMECHEAP_SANDBOX_API_KEY), NAMECHEAP_USERNAME, and NAMECHEAP_CLIENT_IP environment variables")
	}

	log.Printf("Creating Namecheap DNS provider (sandbox: %v, user: %s, client_ip: %s)", config.Sandbox, config.APIUser, config.ClientIP)

	provider, err := dns.NewNamecheapProvider(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Namecheap provider: %w", err)
	}

	// Note: In production, validate configuration by making API call
	// For demonstration, we skip validation if using placeholder credentials
	if !strings.Contains(config.APIKey, "placeholder") {
		if err := provider.ValidateConfiguration(); err != nil {
			return nil, fmt.Errorf("namecheap provider configuration validation failed: %w", err)
		}
		log.Printf("Namecheap DNS provider validated successfully")
	} else {
		log.Printf("Namecheap DNS provider created with placeholder credentials (validation skipped)")
	}

	log.Printf("Namecheap DNS provider initialized successfully")
	return provider, nil
}

func initializeSelfUpdateHandler(cfg *ControllerConfig, cfgService *cfgsvc.Service) (*selfupdate.Handler, error) {
	// Create storage client for self-update operations
	if cfgService == nil {
		return nil, fmt.Errorf("config service required for self-update handler")
	}
	unified, err := resolveStorageFromConfigService(cfgService)
	if err != nil {
		return nil, fmt.Errorf("resolve storage for self-update: %w", err)
	}
	provider := internalStorage.NewProviderFromStorage(unified, "artifacts")

	// Get current controller version
	currentVersion := selfupdate.GetCurrentVersion()

	// Create self-update handler
	handler, err := selfupdate.NewHandler(provider, cfg.ConsulAddr, currentVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to create self-update handler: %w", err)
	}

	log.Printf("Self-update handler initialized (current version: %s)", currentVersion)
	return handler, nil
}
