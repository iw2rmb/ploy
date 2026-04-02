[artifacts.go](artifacts.go) Defines artifact identity value objects (CID and sha256 digest) with strict text/JSON validation.
[artifacts_test.go](artifacts_test.go) Tests artifact value-type normalization, validation failures, and JSON/text round-trip behavior.
[base.go](base.go) Provides shared type utilities for normalization, JSON text marshaling helpers, and generic string adapters.
[diff_types.go](diff_types.go) Declares diff job-type enum values and validation for allowed diff producer kinds.
[diffsummary.go](diffsummary.go) Implements raw-JSON diff summary wrapper with typed accessors for common summary metrics fields.
[diffsummary_test.go](diffsummary_test.go) Verifies diff summary accessor behavior, empty/null handling, and JSON marshal/unmarshal semantics.
[doc.go](doc.go) Documents the package purpose as the home for strongly typed domain identifiers and value objects.
[duration.go](duration.go) Defines a duration wrapper with text/JSON/YAML encoding and parse-time validation helpers.
[duration_test.go](duration_test.go) Covers duration parsing, encoding contracts, invalid inputs, and duration conversion helper utilities.
[eventid_test.go](eventid_test.go) Tests SSE event ID validation, conversion helpers, and serialization round-trips.
[idgen.go](idgen.go) Centralizes RunID/JobID/Node key generation using KSUID and NanoID-backed helpers.
[ids.go](ids.go) Defines core domain ID string types with text/JSON codecs plus canonical validation and normalization rules.
[ids_test.go](ids_test.go) Tests ID type validation boundaries and marshal/unmarshal behavior across supported ID variants.
[logging.go](logging.go) Defines typed logging payload structures and helpers used by domain-level log event handling.
[logging_test.go](logging_test.go) Validates domain logging payload conversions, field mapping, and edge-case handling.
[migref_test.go](migref_test.go) Tests MigRef validation rules for accepted characters, normalization, and rejection cases.
[migs.go](migs.go) Declares migration domain types (including MigRef) and related validation helpers.
[network.go](network.go) Defines network-related domain value types such as protocol enums and associated validation logic.
[network_test.go](network_test.go) Tests network domain type parsing, accepted values, and invalid protocol/input cases.
[resources.go](resources.go) Defines resource sizing/value types and validation helpers for CPU, memory, and related limits.
[resources_test.go](resources_test.go) Verifies resource type parsing and validation behavior for valid and invalid resource values.
[runstats.go](runstats.go) Defines run/job statistics domain models and helper logic for computed aggregate run metrics.
[runstats_test.go](runstats_test.go) Tests run statistics aggregation, derived-field calculations, and edge-case metric behavior.
[runsummary.go](runsummary.go) Defines run summary DTO/value types used for API and persistence-facing run status views.
[runsummary_test.go](runsummary_test.go) Validates run summary decoding, required-field checks, and normalization/validation behavior.
[scope.go](scope.go) Defines global env target scope enum and matching rules for server, node, gate, and step injection.
[scope_test.go](scope_test.go) Tests global env target parsing, validation, and job-type matching semantics.
[sse.go](sse.go) Declares SSE event type enum and validation for supported streaming event categories.
[statuses.go](statuses.go) Defines canonical run/job/repo status enums with parser and SQL scan/value implementations.
[statuses_test.go](statuses_test.go) Tests status parsing, scan/value validation, and allowed enum invariants.
[vcs.go](vcs.go) Defines VCS value types (repo URL, git ref, commit SHA) with normalization and codec validation.
[vcs_test.go](vcs_test.go) Tests VCS type parsing, URL normalization rules, and text/JSON round-trip contracts.
