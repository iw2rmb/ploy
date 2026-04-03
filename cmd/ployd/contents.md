[config_dsn.go](config_dsn.go) Resolves PostgreSQL DSN precedence between environment and server configuration.
[config_objectstore.go](config_objectstore.go) Resolves object store settings from environment variables over config defaults.
[gate_catalog_seed.go](gate_catalog_seed.go) Seeds default gate stacks and profiles into blob storage and database catalog tables.
[gate_catalog_seed_test.go](gate_catalog_seed_test.go) Tests gate-catalog seeding idempotence, object key writes, and profile-loading failure paths.
[logging.go](logging.go) Initializes process-wide structured logging with level, format, and output-file options.
[main.go](main.go) CLI entrypoint that loads config, initializes dependencies, seeds gate catalog, and runs ployd.
[main_test.go](main_test.go) Covers main-package helpers and server startup wiring behavior under test scenarios.
[server.go](server.go) Builds and runs ployd services, loads persisted config state, executes special-env migration, and starts HTTP/metrics servers.
