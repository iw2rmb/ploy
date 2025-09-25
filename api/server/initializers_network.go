package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/iw2rmb/ploy/api/certificates"
	"github.com/iw2rmb/ploy/api/dns"
	"github.com/iw2rmb/ploy/api/selfupdate"
	cfgsvc "github.com/iw2rmb/ploy/internal/config"
	internalStorage "github.com/iw2rmb/ploy/internal/storage"
	nats "github.com/nats-io/nats.go"
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
	store, err := initializeCertificateStore(cfg)
	if err != nil {
		return nil, err
	}

	dnsProvider, err := initializeDNSProvider()
	if err != nil {
		log.Printf("Warning: DNS provider initialization failed, certificates may not work: %v", err)
		dnsProvider = nil
	}

	certificateManager, err := certificates.NewCertificateManager(store, dnsProvider)
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

func initializeSelfUpdateHandler(cfg *ControllerConfig, cfgService *cfgsvc.Service, metricsRecorder selfupdate.MetricsRecorder) (*selfupdate.Handler, error) {
	if cfgService == nil {
		return nil, fmt.Errorf("config service required for self-update handler")
	}
	unified, err := resolveStorageFromConfigService(cfgService)
	if err != nil {
		return nil, fmt.Errorf("resolve storage for self-update: %w", err)
	}
	provider := internalStorage.NewProviderFromStorage(unified, "artifacts")

	updatesCfg := cfg.JetStreamUpdates
	if !updatesCfg.Enabled {
		return nil, fmt.Errorf("jetstream updates configuration disabled")
	}
	if updatesCfg.URL == "" {
		return nil, fmt.Errorf("jetstream updates url not configured")
	}

	opts := []nats.Option{nats.Name("ploy-selfupdate-handler")}
	if updatesCfg.CredentialsPath != "" {
		opts = append(opts, nats.UserCredentials(updatesCfg.CredentialsPath))
	}
	if updatesCfg.User != "" {
		opts = append(opts, nats.UserInfo(updatesCfg.User, updatesCfg.Password))
	}

	conn, err := nats.Connect(updatesCfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect jetstream updates: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create jetstream context: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	queueCfg := selfupdate.WorkQueueConfig{
		Stream:        updatesCfg.Stream,
		SubjectPrefix: updatesCfg.SubjectPrefix,
		DurablePrefix: updatesCfg.DurablePrefix,
		Lane:          updatesCfg.Lane,
		AckWait:       updatesCfg.AckWait,
		MaxAckPending: updatesCfg.MaxAckPending,
		MaxDeliver:    updatesCfg.MaxDeliver,
		Replicas:      updatesCfg.Replicas,
	}
	queue, err := selfupdate.NewJetStreamWorkQueue(ctx, js, queueCfg)
	if err != nil {
		if metricsRecorder != nil {
			metricsRecorder.RecordSelfUpdateBootstrap(queueCfg.Stream, "error")
		}
		conn.Close()
		return nil, fmt.Errorf("bootstrap self-update work queue: %w", err)
	}
	if metricsRecorder != nil {
		metricsRecorder.RecordSelfUpdateBootstrap(queueCfg.Stream, "success")
	}

	statusCfg := selfupdate.StatusStreamConfig{
		Stream:        updatesCfg.StatusStream,
		SubjectPrefix: updatesCfg.StatusSubjectPrefix,
		DurablePrefix: updatesCfg.StatusDurablePrefix,
		Replicas:      updatesCfg.StatusReplicas,
		MaxAge:        updatesCfg.StatusMaxAge,
	}
	statusPublisher, err := selfupdate.NewStatusPublisher(ctx, js, statusCfg)
	if err != nil {
		if metricsRecorder != nil {
			metricsRecorder.RecordSelfUpdateBootstrap(statusCfg.Stream, "error")
		}
		conn.Close()
		return nil, fmt.Errorf("bootstrap self-update status stream: %w", err)
	}
	if metricsRecorder != nil {
		metricsRecorder.RecordSelfUpdateBootstrap(statusCfg.Stream, "success")
	}

	currentVersion := selfupdate.GetCurrentVersion()
	handler, err := selfupdate.NewHandler(provider, queue, statusPublisher, currentVersion, metricsRecorder)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create self-update handler: %w", err)
	}

	// Start executor to process queued updates.
	handler.StartExecutor(context.Background())

	log.Printf("Self-update handler initialized (current version: %s, lane=%s)", currentVersion, updatesCfg.Lane)
	return handler, nil
}
