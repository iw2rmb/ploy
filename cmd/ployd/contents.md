[config_dsn.go](config_dsn.go) Resolves PostgreSQL DSN from env first, then config, while treating unresolved placeholders as unset.
[config_objectstore.go](config_objectstore.go) Builds object-store settings from config with environment variable overrides for endpoint, bucket, creds, TLS, and region.
[gate_catalog_seed.go](gate_catalog_seed.go) Loads stacks catalog entries, uploads default gate profiles to blob storage, and upserts default stack/profile records.
[gate_catalog_seed_test.go](gate_catalog_seed_test.go) Verifies catalog seeding idempotence, object-key/profile JSON writes, registry prefix expansion, and missing-profile failures.
[logging.go](logging.go) Configures global slog logging level, text/JSON handler selection, output destination, and static fields.
[main.go](main.go) Starts ployd by parsing flags, loading config, initializing dependencies, seeding catalog defaults, and running the server loop.
[main_test.go](main_test.go) Tests DSN and logging helpers plus run-loop startup/shutdown and scheduler task wiring scenarios.
[server.go](server.go) Constructs runtime services and background tasks, restores persisted global config overlays, and starts HTTP and metrics servers.
