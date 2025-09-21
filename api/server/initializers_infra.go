package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	nomadapi "github.com/hashicorp/nomad/api"
	nats "github.com/nats-io/nats.go"

	"github.com/iw2rmb/ploy/api/consul_envstore"
	"github.com/iw2rmb/ploy/api/coordination"
	"github.com/iw2rmb/ploy/api/metrics"
	"github.com/iw2rmb/ploy/api/routing"
	"github.com/iw2rmb/ploy/internal/bluegreen"
	"github.com/iw2rmb/ploy/internal/cleanup"
	envstore "github.com/iw2rmb/ploy/internal/envstore"
	irouting "github.com/iw2rmb/ploy/internal/routing"
)

func initializeEnvStore(cfg *ControllerConfig, recorder consul_envstore.MetricsRecorder) (envstore.EnvStoreInterface, error) {
	var options []consul_envstore.Option
	if recorder != nil {
		options = append(options, consul_envstore.WithMetrics(recorder))
	}

	var jsWriter *jetStreamKVWriter
	if cfg.UseConsulEnv {
		if cfg.JetStreamEnv.DualWrite {
			writer, err := newJetStreamEnvWriter(cfg.JetStreamEnv)
			if err != nil {
				log.Printf("Warning: JetStream dual-write disabled (initialization failed): %v", err)
			} else {
				log.Printf("JetStream dual-write enabled for env store (bucket=%s)", cfg.JetStreamEnv.Bucket)
				jsWriter = writer
				options = append(options, consul_envstore.WithSecondary(writer))
			}
		}

		if consulEnvStore, err := consul_envstore.New(cfg.ConsulAddr, "ploy/apps", options...); err == nil {
			if err := consulEnvStore.HealthCheck(); err == nil {
				log.Printf("Using Consul KV store for environment variables at %s", cfg.ConsulAddr)
				return consulEnvStore, nil
			}
			log.Printf("Consul env store health check failed, falling back to file-based store: %v", err)
		} else {
			log.Printf("Failed to initialize Consul env store, falling back to file-based store: %v", err)
		}

		if jsWriter != nil {
			if err := jsWriter.Close(); err != nil {
				log.Printf("Warning: failed to close JetStream connection during fallback: %v", err)
			}
		}
	}

	// Fallback to file-based store
	fileEnvStore := envstore.New(cfg.EnvStorePath)
	log.Printf("Using file-based environment store at %s", cfg.EnvStorePath)
	return fileEnvStore, nil
}

type jetStreamKVWriter struct {
	conn   *nats.Conn
	bucket nats.KeyValue
}

func (w *jetStreamKVWriter) Put(key string, value []byte) error {
	if w == nil || w.bucket == nil {
		return fmt.Errorf("jetstream bucket unavailable")
	}
	_, err := w.bucket.Put(key, value)
	return err
}

func (w *jetStreamKVWriter) Delete(key string) error {
	if w == nil || w.bucket == nil {
		return fmt.Errorf("jetstream bucket unavailable")
	}
	err := w.bucket.Delete(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return nil
	}
	return err
}

func (w *jetStreamKVWriter) Close() error {
	if w == nil || w.conn == nil {
		return nil
	}
	if err := w.conn.Drain(); err != nil {
		w.conn.Close()
		return err
	}
	w.conn.Close()
	return nil
}

func newJetStreamEnvWriter(cfg JetStreamEnvConfig) (*jetStreamKVWriter, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("jetstream url not configured")
	}

	opts := []nats.Option{nats.Name("ploy-envstore-dual-writer")}
	if cfg.CredentialsPath != "" {
		opts = append(opts, nats.UserCredentials(cfg.CredentialsPath))
	}
	if cfg.User != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, err
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, err
	}

	bucket := cfg.Bucket
	if bucket == "" {
		bucket = "ploy_env"
	}

	kv, err := js.KeyValue(bucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(&nats.KeyValueConfig{Bucket: bucket})
	}
	if err != nil {
		conn.Close()
		return nil, err
	}

	return &jetStreamKVWriter{conn: conn, bucket: kv}, nil
}

func initializeTraefikRouter(cfg *ControllerConfig, metrics *metrics.Metrics) (*routing.TraefikRouter, error) {
	var store *irouting.Store
	if cfg.JetStreamRouting.Enabled {
		if cfg.JetStreamRouting.URL == "" && cfg.JetStreamRouting.CredentialsPath == "" && cfg.JetStreamRouting.User == "" {
			log.Printf("Warning: JetStream routing enabled without connection details; skipping initialization")
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			storeCfg := irouting.StoreConfig{
				URL:           cfg.JetStreamRouting.URL,
				Bucket:        cfg.JetStreamRouting.Bucket,
				Stream:        cfg.JetStreamRouting.Stream,
				SubjectPrefix: cfg.JetStreamRouting.SubjectPrefix,
				ChunkSize:     cfg.JetStreamRouting.ChunkSize,
				Replicas:      cfg.JetStreamRouting.Replicas,
			}
			if cfg.JetStreamRouting.CredentialsPath != "" {
				storeCfg.UserCreds = cfg.JetStreamRouting.CredentialsPath
			}
			if cfg.JetStreamRouting.User != "" {
				storeCfg.User = cfg.JetStreamRouting.User
				storeCfg.Password = cfg.JetStreamRouting.Password
			}
			createdStore, err := irouting.NewStore(ctx, storeCfg)
			if err != nil {
				log.Printf("[routing] Error during routing object store bootstrap (bucket=%s, stream=%s): %v", cfg.JetStreamRouting.Bucket, cfg.JetStreamRouting.Stream, err)
				if metrics != nil {
					metrics.RecordRoutingObjectStoreBootstrap("error")
				}
			} else {
				store = createdStore
				if metrics != nil {
					metrics.RecordRoutingObjectStoreBootstrap("success")
				}
				log.Printf("[routing] JetStream routing object store bootstrap complete (bucket=%s, stream=%s)", cfg.JetStreamRouting.Bucket, cfg.JetStreamRouting.Stream)
			}
		}
	}

	traefikRouter, err := routing.NewTraefikRouter(cfg.ConsulAddr, routing.RouterOptions{
		Store:   store,
		Metrics: metrics,
	})
	if err != nil {
		return nil, err
	}
	log.Printf("Traefik router initialized with Consul address: %s", cfg.ConsulAddr)
	return traefikRouter, nil
}

func initializeCleanupService(cfg *ControllerConfig) (*cleanup.CleanupHandler, *cleanup.TTLCleanupService, error) {
	configManager := cleanup.NewConfigManager(cfg.CleanupConfigPath)

	// Load or create cleanup configuration
	cleanupConfig, err := configManager.LoadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load cleanup configuration, using defaults: %v", err)
		cleanupConfig = cleanup.DefaultTTLConfig()
	}

	// Override with environment variables if present
	envConfig := cleanup.LoadConfigFromEnv()
	if envConfig.PreviewTTL != cleanupConfig.PreviewTTL {
		cleanupConfig.PreviewTTL = envConfig.PreviewTTL
	}
	if envConfig.CleanupInterval != cleanupConfig.CleanupInterval {
		cleanupConfig.CleanupInterval = envConfig.CleanupInterval
	}
	if envConfig.MaxAge != cleanupConfig.MaxAge {
		cleanupConfig.MaxAge = envConfig.MaxAge
	}
	if envConfig.DryRun != cleanupConfig.DryRun {
		cleanupConfig.DryRun = envConfig.DryRun
	}
	if envConfig.NomadAddr != cleanupConfig.NomadAddr {
		cleanupConfig.NomadAddr = envConfig.NomadAddr
	}

	// Create TTL cleanup service
	ttlCleanupService := cleanup.NewTTLCleanupService(cleanupConfig)
	cleanupHandler := cleanup.NewCleanupHandler(ttlCleanupService, configManager)

	// Start TTL cleanup service if enabled
	if cfg.CleanupAutoStart {
		if err := ttlCleanupService.Start(); err != nil {
			log.Printf("Warning: Failed to start TTL cleanup service: %v", err)
		} else {
			log.Printf("TTL cleanup service started (interval: %v, preview TTL: %v)",
				cleanupConfig.CleanupInterval, cleanupConfig.PreviewTTL)
		}
	}

	return cleanupHandler, ttlCleanupService, nil
}

func initializeCoordinationManagerWithMetrics(cfg *ControllerConfig, metrics *metrics.Metrics) (*coordination.CoordinationManager, error) {
	log.Printf("Initializing coordination manager for leader election")

	coordinationMgr, err := coordination.NewCoordinationManagerWithMetrics(cfg.ConsulAddr, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create coordination manager: %w", err)
	}

	log.Printf("Coordination manager initialized successfully")
	return coordinationMgr, nil
}

func initializeBlueGreenManager(cfg *ControllerConfig) (*bluegreen.Manager, error) {
	log.Printf("Initializing blue-green deployment manager")

	// Initialize Consul client
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = cfg.ConsulAddr
	consulClient, err := consulapi.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create consul client: %w", err)
	}

	// Initialize Nomad client
	nomadConfig := nomadapi.DefaultConfig()
	nomadConfig.Address = cfg.NomadAddr
	nomadClient, err := nomadapi.NewClient(nomadConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create nomad client: %w", err)
	}

	// Create blue-green manager
	blueGreenManager := bluegreen.NewManager(consulClient, nomadClient)

	log.Printf("Blue-green deployment manager initialized successfully")
	return blueGreenManager, nil
}
