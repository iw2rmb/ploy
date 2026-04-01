[bootstrap_helpers.go](bootstrap_helpers.go) Shared bootstrap utilities for SSH/SCP args and cluster or node identifier generation.
[bootstrap_helpers_test.go](bootstrap_helpers_test.go) Tests for bootstrap helper behaviors including ID generation patterns and random token helpers.
[bootstrap_script.go](bootstrap_script.go) Bootstrap script renderer that emits deterministic env exports and host setup shell logic.
[bootstrap_script_test.go](bootstrap_script_test.go) Tests for bootstrap script rendering, quoting, and expected provisioning script fragments.
[bootstrap_types.go](bootstrap_types.go) Core deploy runner abstractions, I/O stream types, and default constants for remote provisioning.
[detect.go](detect.go) Remote cluster detection logic that probes host files and extracts cluster ID from server certificate CN.
[detect_test.go](detect_test.go) Tests for detection outcomes across missing files, parsing cases, and SSH command behaviors.
[provision.go](provision.go) Remote provisioning workflow that uploads ployd binaries, executes bootstrap scripts, and checks services.
[provision_and_workstation_test.go](provision_and_workstation_test.go) Tests for shared provisioning helpers such as shell quoting and merged bootstrap environment rendering.
[provision_host_test.go](provision_host_test.go) Host provisioning tests covering command sequencing and service-status failure fallback behavior.
[provision_test.go](provision_test.go) Provisioning script rendering tests for server and node configuration fragments and PostgreSQL setup logic.
