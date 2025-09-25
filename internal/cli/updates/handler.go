package updates

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/iw2rmb/ploy/api/selfupdate"
)

// UpdatesCmd is the CLI entrypoint for self-update coordination helpers.
func UpdatesCmd(args []string, controllerURL string) {
	_ = controllerURL
	if len(args) == 0 {
		printUsage()
		return
	}

	switch args[0] {
	case "tail":
		tailCommand(args[1:])
	default:
		printUsage()
	}
}

func tailCommand(args []string) {
	fs := flag.NewFlagSet("updates tail", flag.ExitOnError)
	deployment := fs.String("deployment", "", "deployment id to follow")
	follow := fs.Bool("follow", false, "continue streaming after terminal state")
	fetchTimeout := fs.Duration("wait", 2*time.Second, "max wait per fetch (e.g. 2s)")
	_ = fs.Parse(args)

	if *deployment == "" {
		if fs.NArg() > 0 {
			*deployment = fs.Arg(0)
		}
	}
	if *deployment == "" {
		fmt.Println("error: deployment id is required")
		fs.Usage()
		return
	}

	connCfg, opts, err := loadConfig(*deployment, *fetchTimeout, *follow)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	ctx := context.Background()
	if err := tailWithConnection(ctx, connCfg, opts); err != nil {
		fmt.Println("error:", err)
	}
}

type connectionConfig struct {
	URL        string
	CredsPath  string
	User       string
	Password   string
	ClientName string
}

func tailWithConnection(ctx context.Context, cfg connectionConfig, opts TailOptions) error {
	if cfg.URL == "" {
		return fmt.Errorf("PLOY_UPDATES_JETSTREAM_URL or PLOY_JETSTREAM_URL must be set")
	}

	natsOpts := []nats.Option{nats.Name(cfg.ClientName)}
	if cfg.CredsPath != "" {
		natsOpts = append(natsOpts, nats.UserCredentials(cfg.CredsPath))
	}
	if cfg.User != "" {
		natsOpts = append(natsOpts, nats.UserInfo(cfg.User, cfg.Password))
	}

	conn, err := nats.Connect(cfg.URL, natsOpts...)
	if err != nil {
		return fmt.Errorf("connect jetstream: %w", err)
	}
	defer conn.Close()

	js, err := conn.JetStream()
	if err != nil {
		return fmt.Errorf("jetstream context: %w", err)
	}

	return ConsumeStatusEvents(ctx, js, opts, func(event selfupdate.StatusEvent) {
		fmt.Println(formatEventLine(event))
	})
}

func loadConfig(deploymentID string, fetchTimeout time.Duration, follow bool) (connectionConfig, TailOptions, error) {
	url := firstNonEmpty(
		os.Getenv("PLOY_UPDATES_JETSTREAM_URL"),
		os.Getenv("PLOY_JETSTREAM_URL"),
		os.Getenv("NATS_URL"),
	)
	creds := firstNonEmpty(
		os.Getenv("NATS_UPDATES_CREDS"),
		os.Getenv("PLOY_UPDATES_JETSTREAM_CREDS"),
		os.Getenv("PLOY_JETSTREAM_CREDS"),
	)
	user := firstNonEmpty(os.Getenv("PLOY_UPDATES_JETSTREAM_USER"), os.Getenv("PLOY_JETSTREAM_USER"))
	password := firstNonEmpty(os.Getenv("PLOY_UPDATES_JETSTREAM_PASSWORD"), os.Getenv("PLOY_JETSTREAM_PASSWORD"))
	stream := firstNonEmpty(os.Getenv("PLOY_UPDATES_STATUS_STREAM"), "updates.control-plane.status")
	subjectPrefix := firstNonEmpty(os.Getenv("PLOY_UPDATES_STATUS_SUBJECT_PREFIX"), "updates.control-plane.status")
	durablePrefix := firstNonEmpty(os.Getenv("PLOY_UPDATES_STATUS_DURABLE_PREFIX"), "updates-status-cli")
	ackWait := parseDurationWithFallback(os.Getenv("PLOY_UPDATES_ACK_WAIT"), 2*time.Minute)

	connCfg := connectionConfig{
		URL:        url,
		CredsPath:  creds,
		User:       user,
		Password:   password,
		ClientName: "ploy-cli-updates",
	}

	if connCfg.URL == "" {
		return connectionConfig{}, TailOptions{}, fmt.Errorf("jetstream url is not configured")
	}

	opts := TailOptions{
		Stream:        stream,
		SubjectPrefix: subjectPrefix,
		DurablePrefix: durablePrefix,
		DeploymentID:  deploymentID,
		AckWait:       ackWait,
		FetchTimeout:  fetchTimeout,
		Follow:        follow,
	}

	return connCfg, opts, nil
}

func formatEventLine(event selfupdate.StatusEvent) string {
	ts := event.Timestamp.UTC().Format(time.RFC3339)
	ph := strings.ToUpper(event.Phase)
	msg := event.Message
	if msg == "" {
		msg = "no message"
	}
	extra := make([]string, 0, 2)
	if event.Executor != "" {
		extra = append(extra, fmt.Sprintf("executor=%s", event.Executor))
	}
	if lane, ok := event.Metadata["lane"]; ok && lane != "" {
		extra = append(extra, fmt.Sprintf("lane=%s", lane))
	}
	if len(extra) > 0 {
		msg = fmt.Sprintf("%s (%s)", msg, strings.Join(extra, ", "))
	}
	return fmt.Sprintf("%s %-10s %3d%% %s", ts, ph, event.Progress, msg)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseDurationWithFallback(value string, fallback time.Duration) time.Duration {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return d
}

func printUsage() {
	fmt.Println("usage:")
	fmt.Println("  ploy updates tail <deployment-id> [--follow] [--wait=2s]")
}
