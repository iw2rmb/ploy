package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// LoadFromEnv builds server configuration from environment variables.
func LoadFromEnv() (Config, error) {
	cfg := defaultConfig()
	applyDefaults(&cfg)

	cfg.Postgres.DSN = strings.TrimSpace(os.Getenv("PLOY_DB_DSN"))
	cfg.Auth.BearerTokens.Secret = strings.TrimSpace(os.Getenv("PLOY_AUTH_SECRET"))
	cfg.ObjectStore.Endpoint = strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_ENDPOINT"))
	cfg.ObjectStore.Bucket = strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_BUCKET"))
	cfg.ObjectStore.AccessKey = strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_ACCESS_KEY"))
	cfg.ObjectStore.SecretKey = strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_SECRET_KEY"))
	cfg.ObjectStore.Region = strings.TrimSpace(os.Getenv("PLOY_OBJECTSTORE_REGION"))
	cfg.GitLab.Domain = strings.TrimSpace(os.Getenv("PLOY_GITLAB_DOMAIN"))
	cfg.GitLab.Token = strings.TrimSpace(os.Getenv("PLOY_GITLAB_TOKEN"))

	if secure, ok, err := envBool("PLOY_OBJECTSTORE_SECURE"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.ObjectStore.Secure = secure
	}
	if enabled, ok, err := envBool("PLOYD_AUTH_BEARER_TOKENS_ENABLED"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Auth.BearerTokens.Enabled = enabled
	}
	if jsonLogs, ok, err := envBool("PLOYD_LOG_JSON"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Logging.JSON = jsonLogs
	}
	if dropPartitions, ok, err := envBool("PLOYD_SCHEDULER_DROP_PARTITIONS"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Scheduler.DropPartitions = dropPartitions
	}

	if staticFields, ok, err := envStringMap("PLOYD_LOG_STATIC_FIELDS"); err != nil {
		return Config{}, err
	} else if ok {
		cfg.Logging.StaticFields = staticFields
	}

	if err := applyEnvString(&cfg.HTTP.Listen, "PLOYD_HTTP_LISTEN"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.Metrics.Listen, "PLOYD_METRICS_LISTEN"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.Admin.Socket, "PLOYD_ADMIN_SOCKET"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.Admin.Listen, "PLOYD_ADMIN_LISTEN"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.PKI.BundleDir, "PLOYD_PKI_BUNDLE_DIR"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.PKI.Certificate, "PLOYD_PKI_CERTIFICATE"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.PKI.Key, "PLOYD_PKI_KEY"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.PKI.CAEndpoint, "PLOYD_PKI_CA_ENDPOINT"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.Logging.Level, "PLOYD_LOG_LEVEL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvString(&cfg.Logging.File, "PLOYD_LOG_FILE"); err != nil {
		return Config{}, err
	}

	if err := applyEnvDuration(&cfg.HTTP.ReadTimeout, "PLOYD_HTTP_READ_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.HTTP.WriteTimeout, "PLOYD_HTTP_WRITE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.HTTP.IdleTimeout, "PLOYD_HTTP_IDLE_TIMEOUT"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.PKI.RenewBefore, "PLOYD_PKI_RENEW_BEFORE"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.HousekeepingInterval, "PLOYD_SCHEDULER_HOUSEKEEPING_INTERVAL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.DiskPruneInterval, "PLOYD_SCHEDULER_DISK_PRUNE_INTERVAL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.TTL, "PLOYD_SCHEDULER_TTL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.TTLInterval, "PLOYD_SCHEDULER_TTL_INTERVAL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.BatchSchedulerInterval, "PLOYD_SCHEDULER_BATCH_SCHEDULER_INTERVAL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.StaleJobRecoveryInterval, "PLOYD_SCHEDULER_STALE_JOB_RECOVERY_INTERVAL"); err != nil {
		return Config{}, err
	}
	if err := applyEnvDuration(&cfg.Scheduler.NodeStaleAfter, "PLOYD_SCHEDULER_NODE_STALE_AFTER"); err != nil {
		return Config{}, err
	}

	if err := applyEnvInt(&cfg.Logging.MaxSizeMB, "PLOYD_LOG_MAX_SIZE_MB"); err != nil {
		return Config{}, err
	}
	if err := applyEnvInt(&cfg.Logging.MaxBackups, "PLOYD_LOG_MAX_BACKUPS"); err != nil {
		return Config{}, err
	}
	if err := applyEnvInt(&cfg.Logging.MaxAgeDays, "PLOYD_LOG_MAX_AGE_DAYS"); err != nil {
		return Config{}, err
	}

	applyDefaults(&cfg)
	if err := validate(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnvString(dst *string, key string) error {
	if dst == nil {
		return nil
	}
	if raw, ok := os.LookupEnv(key); ok {
		if v := strings.TrimSpace(raw); v != "" {
			*dst = v
		}
	}
	return nil
}

func applyEnvDuration(dst *time.Duration, key string) error {
	if dst == nil {
		return nil
	}
	raw, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("config: %s: parse duration %q: %w", key, v, err)
	}
	*dst = d
	return nil
}

func applyEnvInt(dst *int, key string) error {
	if dst == nil {
		return nil
	}
	raw, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("config: %s: parse int %q: %w", key, v, err)
	}
	*dst = n
	return nil
}

func envBool(key string) (bool, bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return false, false, nil
	}
	v := strings.TrimSpace(raw)
	if v == "" {
		return false, false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false, fmt.Errorf("config: %s: parse bool %q: %w", key, v, err)
	}
	return b, true, nil
}

func envStringMap(key string) (map[string]string, bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return nil, false, nil
	}
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil, false, nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(v), &m); err != nil {
		return nil, false, fmt.Errorf("config: %s: parse json object: %w", key, err)
	}
	if m == nil {
		return map[string]string{}, true, nil
	}
	return m, true, nil
}
