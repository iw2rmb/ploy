package routing

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/iw2rmb/ploy/internal/routing"
	"github.com/iw2rmb/ploy/internal/utils"
)

// RoutingCmd handles `ploy routing` subcommands.
func RoutingCmd(args []string, controller string) {
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "resync":
		resyncCmd(args[1:])
	default:
		printUsage()
	}
}

func resyncCmd(args []string) {
	fs := flag.NewFlagSet("routing resync", flag.ExitOnError)
	timeout := fs.Duration("timeout", 30*time.Second, "operation timeout")
	if err := fs.Parse(args); err != nil {
		log.Printf("failed to parse flags: %v", err)
		return
	}

	apps := fs.Args()
	if len(apps) == 0 {
		fmt.Println("usage: ploy routing resync <app> [<app>...]")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	store, err := newRoutingStore(ctx)
	if err != nil {
		log.Printf("failed to initialize routing store: %v", err)
		return
	}

	for _, app := range apps {
		if err := store.RebroadcastApp(ctx, app); err != nil {
			log.Printf("failed to rebroadcast %s: %v", app, err)
			continue
		}
		fmt.Printf("rebroadcast routing metadata for %s\n", app)
	}
}

func newRoutingStore(ctx context.Context) (*routing.Store, error) {
	url := utils.Getenv("PLOY_ROUTING_JETSTREAM_URL", utils.Getenv("PLOY_JETSTREAM_URL", ""))
	if url == "" {
		return nil, fmt.Errorf("PLOY_ROUTING_JETSTREAM_URL must be set")
	}

	cfg := routing.StoreConfig{
		URL:           url,
		UserCreds:     utils.Getenv("PLOY_ROUTING_JETSTREAM_CREDS", utils.Getenv("PLOY_JETSTREAM_CREDS", "")),
		User:          utils.Getenv("PLOY_ROUTING_JETSTREAM_USER", utils.Getenv("PLOY_JETSTREAM_USER", "")),
		Password:      utils.Getenv("PLOY_ROUTING_JETSTREAM_PASSWORD", utils.Getenv("PLOY_JETSTREAM_PASSWORD", "")),
		Bucket:        utils.Getenv("PLOY_ROUTING_OBJECT_BUCKET", "routing_maps"),
		Stream:        utils.Getenv("PLOY_ROUTING_EVENT_STREAM", "routing_events"),
		SubjectPrefix: utils.Getenv("PLOY_ROUTING_EVENT_SUBJECT_PREFIX", "routing.app"),
		Replicas:      atoiEnv("PLOY_ROUTING_JETSTREAM_REPLICAS", 3),
	}

	return routing.NewStore(ctx, cfg)
}

func atoiEnv(key string, def int) int {
	val := utils.Getenv(key, "")
	if val == "" {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}

func printUsage() {
	fmt.Println("ploy routing commands:")
	fmt.Println("  ploy routing resync <app> [<app>...]")
}
