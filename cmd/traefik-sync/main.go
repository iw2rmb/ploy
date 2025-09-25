package main

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"

	nats "github.com/nats-io/nats.go"

	configsync "github.com/iw2rmb/ploy/internal/routing/sync"
	"github.com/iw2rmb/ploy/internal/utils"
)

type appConfig struct {
	URL           string
	Credentials   string
	User          string
	Password      string
	Bucket        string
	Stream        string
	SubjectPrefix string
	Durable       string
	OutputPath    string
}

func loadConfig() appConfig {
	return appConfig{
		URL:           utils.Getenv("PLOY_ROUTING_JETSTREAM_URL", "nats://nats.ploy.local:4223"),
		Credentials:   utils.Getenv("PLOY_ROUTING_JETSTREAM_CREDS", ""),
		User:          utils.Getenv("PLOY_ROUTING_JETSTREAM_USER", ""),
		Password:      utils.Getenv("PLOY_ROUTING_JETSTREAM_PASSWORD", ""),
		Bucket:        utils.Getenv("PLOY_ROUTING_OBJECT_BUCKET", "routing_maps"),
		Stream:        utils.Getenv("PLOY_ROUTING_EVENT_STREAM", "routing_events"),
		SubjectPrefix: utils.Getenv("PLOY_ROUTING_EVENT_SUBJECT_PREFIX", "routing.app"),
		Durable:       utils.Getenv("PLOY_TRAEFIK_ROUTING_DURABLE", "traefik-routing-sync"),
		OutputPath:    utils.Getenv("PLOY_TRAEFIK_DYNAMIC_CONFIG", "/data/dynamic-config.yml"),
	}
}

func main() {
	cfg := loadConfig()
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if cfg.URL == "" {
		log.Fatal("PLOY_ROUTING_JETSTREAM_URL is required")
	}

	opts := []nats.Option{nats.Name("traefik-routing-sync")}
	if cfg.Credentials != "" {
		opts = append(opts, nats.UserCredentials(cfg.Credentials))
	}
	if cfg.User != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		log.Fatalf("failed to connect to NATS: %v", err)
	}
	defer func() {
		if err := conn.Drain(); err != nil {
			conn.Close()
		}
	}()

	js, err := conn.JetStream()
	if err != nil {
		log.Fatalf("failed to create JetStream context: %v", err)
	}

	bucket, err := js.ObjectStore(cfg.Bucket)
	if err != nil {
		log.Fatalf("failed to open routing object store %q: %v", cfg.Bucket, err)
	}

	writer := &configsync.FileWriter{Path: cfg.OutputPath}
	syncer := configsync.NewSyncer(configsync.Config{
		Bucket:        bucket,
		Stream:        cfg.Stream,
		SubjectPrefix: cfg.SubjectPrefix,
		Durable:       cfg.Durable,
		JetStream:     js,
		Writer:        writer,
	})

	if err := syncer.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("routing sync terminated with error: %v", err)
	}
}
