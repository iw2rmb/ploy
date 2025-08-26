package performance

import (
	"context"
	"fmt"
	"sync"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
)

// ConsulPool manages a pool of Consul clients for better performance
type ConsulPool struct {
	clients chan *consulapi.Client
	config  *consulapi.Config
	mu      sync.RWMutex
}

// NewConsulPool creates a new Consul client pool
func NewConsulPool(consulAddr string, poolSize int) (*ConsulPool, error) {
	config := consulapi.DefaultConfig()
	if consulAddr != "" {
		config.Address = consulAddr
	}
	
	pool := &ConsulPool{
		clients: make(chan *consulapi.Client, poolSize),
		config:  config,
	}
	
	// Pre-populate the pool
	for i := 0; i < poolSize; i++ {
		client, err := consulapi.NewClient(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create consul client: %w", err)
		}
		pool.clients <- client
	}
	
	return pool, nil
}

// GetClient retrieves a client from the pool
func (p *ConsulPool) GetClient(ctx context.Context) (*consulapi.Client, error) {
	select {
	case client := <-p.clients:
		return client, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		// If pool is empty, create a new client
		return consulapi.NewClient(p.config)
	}
}

// PutClient returns a client to the pool
func (p *ConsulPool) PutClient(client *consulapi.Client) {
	select {
	case p.clients <- client:
		// Successfully returned to pool
	default:
		// Pool is full, let the client be garbage collected
	}
}

// WithClient executes a function with a pooled client
func (p *ConsulPool) WithClient(ctx context.Context, fn func(*consulapi.Client) error) error {
	client, err := p.GetClient(ctx)
	if err != nil {
		return err
	}
	defer p.PutClient(client)
	
	return fn(client)
}

// Size returns the current number of available clients in the pool
func (p *ConsulPool) Size() int {
	return len(p.clients)
}

// NomadPool manages a pool of Nomad clients
type NomadPool struct {
	clients chan *nomadapi.Client
	config  *nomadapi.Config
	mu      sync.RWMutex
}

// NewNomadPool creates a new Nomad client pool
func NewNomadPool(nomadAddr string, poolSize int) (*NomadPool, error) {
	config := nomadapi.DefaultConfig()
	if nomadAddr != "" {
		config.Address = nomadAddr
	}
	
	pool := &NomadPool{
		clients: make(chan *nomadapi.Client, poolSize),
		config:  config,
	}
	
	// Pre-populate the pool
	for i := 0; i < poolSize; i++ {
		client, err := nomadapi.NewClient(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create nomad client: %w", err)
		}
		pool.clients <- client
	}
	
	return pool, nil
}

// GetClient retrieves a client from the pool
func (p *NomadPool) GetClient(ctx context.Context) (*nomadapi.Client, error) {
	select {
	case client := <-p.clients:
		return client, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		// If pool is empty, create a new client
		return nomadapi.NewClient(p.config)
	}
}

// PutClient returns a client to the pool
func (p *NomadPool) PutClient(client *nomadapi.Client) {
	select {
	case p.clients <- client:
		// Successfully returned to pool
	default:
		// Pool is full, let the client be garbage collected
	}
}

// WithClient executes a function with a pooled client
func (p *NomadPool) WithClient(ctx context.Context, fn func(*nomadapi.Client) error) error {
	client, err := p.GetClient(ctx)
	if err != nil {
		return err
	}
	defer p.PutClient(client)
	
	return fn(client)
}

// Size returns the current number of available clients in the pool
func (p *NomadPool) Size() int {
	return len(p.clients)
}

// RetryConfig defines retry behavior with exponential backoff
type RetryConfig struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	Jitter       bool
}

// DefaultRetryConfig returns sensible retry defaults
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
}

// WithRetry executes a function with exponential backoff retry
func WithRetry(ctx context.Context, config RetryConfig, fn func() error) error {
	var lastErr error
	
	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(config.InitialDelay) * 
				float64(attempt) * config.Multiplier)
			
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
			
			// Add jitter to prevent thundering herd
			if config.Jitter {
				jitter := time.Duration(float64(delay) * 0.1 * float64(attempt))
				delay += jitter
			}
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		
		err := fn()
		if err == nil {
			return nil
		}
		
		lastErr = err
		
		// Check if context is cancelled
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	
	return fmt.Errorf("max retry attempts exceeded, last error: %w", lastErr)
}