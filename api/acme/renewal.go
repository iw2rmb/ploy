package acme

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/iw2rmb/ploy/api/dns"
)

// RenewalService manages automatic certificate renewal
type RenewalService struct {
	client      *Client
	storage     *CertificateStorage
	dnsProvider dns.Provider
	config      *RenewalConfig
	stopCh      chan struct{}
	wg          sync.WaitGroup
	running     bool
	mu          sync.RWMutex
}

// RenewalConfig contains configuration for the renewal service
type RenewalConfig struct {
	CheckInterval      time.Duration `json:"check_interval"`       // How often to check for expiring certificates
	RenewalThreshold   time.Duration `json:"renewal_threshold"`    // How long before expiry to renew
	RetryInterval      time.Duration `json:"retry_interval"`       // How long to wait between retries
	MaxRetries         int           `json:"max_retries"`          // Maximum number of retry attempts
	ConcurrentRenewals int           `json:"concurrent_renewals"`  // Maximum concurrent renewals
	NotifyWebhook      string        `json:"notify_webhook"`       // Webhook URL for notifications
}

// RenewalResult represents the result of a renewal attempt
type RenewalResult struct {
	Domain    string    `json:"domain"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Duration  time.Duration `json:"duration"`
}

// RenewalStats tracks renewal statistics
type RenewalStats struct {
	TotalRenewals     int       `json:"total_renewals"`
	SuccessfulRenewals int      `json:"successful_renewals"`
	FailedRenewals    int       `json:"failed_renewals"`
	LastRenewalCheck  time.Time `json:"last_renewal_check"`
	LastRenewalTime   time.Time `json:"last_renewal_time"`
	AverageRenewalTime time.Duration `json:"average_renewal_time"`
}

// DefaultRenewalConfig returns default configuration for the renewal service
func DefaultRenewalConfig() *RenewalConfig {
	return &RenewalConfig{
		CheckInterval:      6 * time.Hour,  // Check every 6 hours
		RenewalThreshold:   30 * 24 * time.Hour, // Renew 30 days before expiry
		RetryInterval:      1 * time.Hour,  // Retry every hour on failure
		MaxRetries:         3,              // Maximum 3 retry attempts
		ConcurrentRenewals: 2,              // Maximum 2 concurrent renewals
	}
}

// NewRenewalService creates a new certificate renewal service
func NewRenewalService(client *Client, storage *CertificateStorage, dnsProvider dns.Provider, config *RenewalConfig) *RenewalService {
	if config == nil {
		config = DefaultRenewalConfig()
	}

	return &RenewalService{
		client:      client,
		storage:     storage,
		dnsProvider: dnsProvider,
		config:      config,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the renewal service
func (rs *RenewalService) Start(ctx context.Context) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if rs.running {
		return fmt.Errorf("renewal service is already running")
	}

	rs.running = true
	rs.wg.Add(1)

	go rs.runRenewalLoop(ctx)

	log.Printf("Certificate renewal service started (check interval: %v, renewal threshold: %v)", 
		rs.config.CheckInterval, rs.config.RenewalThreshold)
	return nil
}

// Stop stops the renewal service
func (rs *RenewalService) Stop() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if !rs.running {
		return fmt.Errorf("renewal service is not running")
	}

	close(rs.stopCh)
	rs.wg.Wait()
	rs.running = false

	log.Printf("Certificate renewal service stopped")
	return nil
}

// IsRunning returns true if the renewal service is running
func (rs *RenewalService) IsRunning() bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.running
}

// TriggerRenewal triggers immediate renewal check
func (rs *RenewalService) TriggerRenewal(ctx context.Context) ([]*RenewalResult, error) {
	log.Printf("Manual renewal check triggered")
	return rs.checkAndRenewCertificates(ctx)
}

// RenewCertificate manually renews a specific certificate
func (rs *RenewalService) RenewCertificate(ctx context.Context, domain string) (*RenewalResult, error) {
	log.Printf("Manual renewal requested for domain: %s", domain)
	
	start := time.Now()
	result := &RenewalResult{
		Domain:    domain,
		Timestamp: start,
	}

	// Get existing certificate
	cert, _, err := rs.storage.GetCertificate(ctx, domain)
	if err != nil {
		result.Error = fmt.Sprintf("failed to get certificate: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Attempt renewal
	newCert, err := rs.client.RenewCertificate(ctx, cert)
	if err != nil {
		result.Error = fmt.Sprintf("failed to renew certificate: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Store renewed certificate
	if err := rs.storage.StoreCertificate(ctx, newCert); err != nil {
		result.Error = fmt.Sprintf("failed to store renewed certificate: %v", err)
		result.Duration = time.Since(start)
		return result, err
	}

	// Update renewal info
	if err := rs.storage.UpdateRenewalInfo(ctx, domain, true); err != nil {
		log.Printf("Warning: failed to update renewal info: %v", err)
	}

	result.Success = true
	result.Duration = time.Since(start)
	
	log.Printf("Certificate renewed successfully for domain: %s (took %v)", domain, result.Duration)
	return result, nil
}

// GetRenewalStats returns renewal statistics
func (rs *RenewalService) GetRenewalStats(ctx context.Context) (*RenewalStats, error) {
	// This would typically be stored in Consul or a database
	// For now, return basic stats
	return &RenewalStats{
		LastRenewalCheck: time.Now(), // Would be stored and retrieved
	}, nil
}

// runRenewalLoop runs the main renewal check loop
func (rs *RenewalService) runRenewalLoop(ctx context.Context) {
	defer rs.wg.Done()

	ticker := time.NewTicker(rs.config.CheckInterval)
	defer ticker.Stop()

	// Initial check
	go rs.performRenewalCheck(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Renewal service stopping due to context cancellation")
			return
		case <-rs.stopCh:
			log.Printf("Renewal service stopping due to stop signal")
			return
		case <-ticker.C:
			go rs.performRenewalCheck(ctx)
		}
	}
}

// performRenewalCheck performs a renewal check
func (rs *RenewalService) performRenewalCheck(ctx context.Context) {
	log.Printf("Starting automatic certificate renewal check")
	
	results, err := rs.checkAndRenewCertificates(ctx)
	if err != nil {
		log.Printf("Error during renewal check: %v", err)
		return
	}

	successful := 0
	failed := 0
	for _, result := range results {
		if result.Success {
			successful++
		} else {
			failed++
		}
	}

	log.Printf("Renewal check completed: %d successful, %d failed", successful, failed)
}

// checkAndRenewCertificates checks for expiring certificates and renews them
func (rs *RenewalService) checkAndRenewCertificates(ctx context.Context) ([]*RenewalResult, error) {
	// Get certificates that need renewal
	expiring, err := rs.storage.GetExpiringSoon(ctx, rs.config.RenewalThreshold)
	if err != nil {
		return nil, fmt.Errorf("failed to get expiring certificates: %w", err)
	}

	if len(expiring) == 0 {
		log.Printf("No certificates need renewal")
		return nil, nil
	}

	log.Printf("Found %d certificates that need renewal", len(expiring))

	// Create semaphore for concurrent renewals
	semaphore := make(chan struct{}, rs.config.ConcurrentRenewals)
	resultsCh := make(chan *RenewalResult, len(expiring))
	
	var wg sync.WaitGroup

	// Process each expiring certificate
	for _, metadata := range expiring {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			
			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Attempt renewal with retries
			result := rs.renewWithRetries(ctx, domain)
			resultsCh <- result
		}(metadata.Domain)
	}

	// Wait for all renewals to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	var results []*RenewalResult
	for result := range resultsCh {
		results = append(results, result)
	}

	return results, nil
}

// renewWithRetries attempts to renew a certificate with retries
func (rs *RenewalService) renewWithRetries(ctx context.Context, domain string) *RenewalResult {
	var lastErr error
	
	for attempt := 0; attempt < rs.config.MaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Retrying renewal for domain %s (attempt %d/%d)", domain, attempt+1, rs.config.MaxRetries)
			time.Sleep(rs.config.RetryInterval)
		}

		result, err := rs.RenewCertificate(ctx, domain)
		if err == nil {
			return result
		}

		lastErr = err
		log.Printf("Renewal attempt %d failed for domain %s: %v", attempt+1, domain, err)
	}

	// All attempts failed
	return &RenewalResult{
		Domain:    domain,
		Success:   false,
		Error:     fmt.Sprintf("renewal failed after %d attempts: %v", rs.config.MaxRetries, lastErr),
		Timestamp: time.Now(),
	}
}