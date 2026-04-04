[docs_guard_test.go](docs_guard_test.go) Guard tests that enforce documentation structure and cross-reference expectations.
[guard_config_home_test.go](guard_config_home_test.go) Guard that keeps tests isolated from real ~/.config/ploy by validating or auto-redirecting PLOY_CONFIG_HOME.
[legacy_job_fields_guard_test.go](legacy_job_fields_guard_test.go) Guard ensuring legacy job fields and wire compatibility assumptions stay intact.
[legacy_mod_naming_guard_test.go](legacy_mod_naming_guard_test.go) Guard preventing regressions in legacy mod naming compatibility rules.
[lints_guard_test.go](lints_guard_test.go) Guard that verifies required lint configuration files and policy checks remain wired.
[main_test.go](main_test.go) Shared TestMain setup for guard package bootstrapping and common helpers.
