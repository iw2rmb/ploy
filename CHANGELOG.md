# CHANGELOG

## [2025-08-18] - Environment Variables Implementation

### Added
- **Environment Variables API Endpoints**
  - `POST /v1/apps/:app/env` - Set multiple environment variables at once
  - `GET /v1/apps/:app/env` - List all environment variables for app
  - `PUT /v1/apps/:app/env/:key` - Update single environment variable
  - `DELETE /v1/apps/:app/env/:key` - Remove environment variable

- **Environment Variables CLI Commands**
  - `ploy env list <app>` - Display all environment variables
  - `ploy env set <app> <key> <value>` - Set environment variable
  - `ploy env get <app> <key>` - Get specific environment variable
  - `ploy env delete <app> <key>` - Delete environment variable

- **Storage Layer**
  - File-based persistence in configurable directory (default: `/tmp/ploy-env-store`)
  - JSON format storage with proper escaping for special characters
  - Thread-safe operations with read-write mutex
  - Persistent storage across controller restarts

### Integration
- **Build Phase Integration**
  - Environment variables passed to all build processes (Gradle, Maven, npm, etc.)
  - Support for all lanes (A-F) with proper environment variable injection
  - Variables available during compilation for Unikraft, OSv, OCI, and VM builds

- **Deploy Phase Integration**  
  - Nomad job templates updated with environment variable placeholders
  - `{{ENV_VARS}}` template rendering generates proper HCL `env {}` blocks
  - Runtime environment variables injected into all deployment targets
  - Updated all lane templates (A-F) to support environment variable rendering

### Testing
- **Comprehensive Test Suite**
  - Created `test-env-vars.sh` for API endpoint testing (scenarios 123-145)
  - Created `test-env-cli.sh` for CLI command testing (scenarios 127-130)
  - Added 23 new test scenarios to TESTS.md covering all functionality
  - API validation: JSON format, error handling, CRUD operations
  - CLI validation: User-friendly output, error messages, integration

### Technical Details
- **Backend Implementation**
  - New `envstore` package with thread-safe file-based storage
  - RESTful API handlers with proper JSON request/response handling
  - Environment variable inheritance in all builder functions
  - Template rendering system for Nomad job environment injection

- **Frontend Implementation**
  - Extended CLI router with `env` command category
  - JSON parsing for API responses with user-friendly formatting
  - Comprehensive error handling and usage messages
  - Integration with existing controller URL configuration

### Documentation
- **Updated Documentation**
  - FEATURES.md: Environment variables section updated to "implemented" status
  - REST.md: Full API specification with request/response examples
  - CLI.md: Complete command reference with usage examples
  - TESTS.md: 23 new test scenarios (123-145) for comprehensive coverage

### Status
**COMPLETED** - Phase 1, Step 4 from PLAN.md: "App environment variables: `POST/GET/PUT/DELETE /v1/apps/:app/env` API and `ploy env` CLI commands to manage per-app environment variables that are available during build and deploy phases"

Environment variables are now fully integrated across the entire Ploy stack, providing developers with complete configuration management for both build-time and runtime environments across all deployment lanes.

## [2025-08-18] - CLI Commands Implementation

### Added
- **Domain Management Commands**
  - `ploy domains add <app> <domain>` - Register custom domain for app
  - `ploy domains list <app>` - List all domains associated with app  
  - `ploy domains remove <app> <domain>` - Remove domain registration

- **Certificate Management Commands**
  - `ploy certs issue <domain>` - Issue TLS certificate via ACME
  - `ploy certs list` - List all managed certificates with expiration dates

- **Debug Commands**
  - `ploy debug shell <app>` - Create debug instance with SSH access
  - `ploy debug shell <app> --lane <A-F>` - Debug with specific lane override

- **Rollback Commands**
  - `ploy rollback <app> <sha>` - Rollback app to previous SHA version

### API Endpoints Added
- `POST /v1/apps/:app/domains` - Add domain to app
- `GET /v1/apps/:app/domains` - List app domains
- `DELETE /v1/apps/:app/domains/:domain` - Remove domain from app
- `POST /v1/certs/issue` - Issue certificate for domain
- `GET /v1/certs` - List all certificates
- `POST /v1/apps/:app/debug` - Create debug instance
- `POST /v1/apps/:app/rollback` - Rollback app to previous version

### Technical Details
- Extended CLI router to handle new command categories
- Added comprehensive error handling and usage messages
- Implemented REST API handlers with proper JSON responses
- Added test scenarios for all new CLI commands (scenarios 79-88)
- All commands follow consistent CLI patterns and conventions

### Testing
-  CLI commands build successfully
-  Proper help messages display for all commands  
-  Error handling works for invalid arguments
-  Commands attempt proper API calls to controller
-  Test scenarios documented in TESTS.md

### Status
**COMPLETED** - Phase 1, Step 1 from PLAN.md: "Complete missing CLI commands: domains add, certs issue, debug shell, rollback"

All essential CLI operations are now implemented, providing users with complete domain, certificate, debugging, and rollback capabilities.

## [2025-08-18] - Controller Fixes & API Testing

### Fixed
- **Controller Compilation Issues**
  - Fixed AWS SDK type error in `internal/storage/storage.go` (changed `aws.ReadSeekCloser` to `io.ReadSeeker`)
  - Resolved syntax error in `previewHostRouter` function (removed stray closing brace)
  - Replaced deprecated `c.Proxy()` with `c.Redirect()` for Fiber v2 compatibility
  - Fixed unused variable warning in `debugApp` function by including lane in log message

### Testing
- **Comprehensive API Test Suite**
  - Created `test-api-endpoints.sh` with 100+ test scenarios
  - All new API endpoints return proper HTTP status codes and JSON responses
  - Error handling validated for invalid JSON and missing required fields
  - Existing endpoints confirmed functional after changes
  - End-to-end CLI-to-API integration verified

### Technical Details
- Controller now compiles cleanly without errors or warnings
- All dependencies resolved via `go mod tidy`
- Successful deployment and testing on production VPS environment
- JSON response format validation ensures API consistency
- Proper error responses with meaningful messages

### Test Results
- ✅ **Domain Management**: add/list/remove operations working
- ✅ **Certificate Management**: issue/list operations working  
- ✅ **Debug Operations**: SSH-enabled debug instances working
- ✅ **Rollback Operations**: SHA-based rollbacks working
- ✅ **Error Handling**: 400 responses for invalid requests
- ✅ **Backward Compatibility**: Existing endpoints unaffected
- ✅ **CLI Integration**: Commands successfully communicate with API

## [2025-08-18] - Lane Picker Jib Detection Enhancement

### Added
- **Enhanced Jib Plugin Detection**
  - Comprehensive Jib detection for Gradle projects (`com.google.cloud.tools.jib`, `jib {}` blocks, `jibBuildTar` tasks)
  - Maven Jib plugin support (`jib-maven-plugin`, XML-based detection)
  - SBT Jib plugin detection for Scala projects (`sbt-jib`)
  - Extended file search to include build scripts (`.gradle`, `.gradle.kts`, `.kts`, `build.sbt`, `pom.xml`)

- **Improved Language Detection** 
  - Scala projects now correctly identified as "scala" instead of "java"
  - Kotlin projects properly handled as Java ecosystem
  - Better precedence for Scala detection over generic JVM tools

- **Lane Selection Logic**
  - Java/Scala with Jib → Lane E (optimal for containerless builds)
  - Java/Scala without Jib → Lane C (using OSv for JVM optimization)
  - Enhanced reasoning messages explain lane selection rationale

### Fixed
- **Build Script Parsing**: `grep()` function now searches Gradle, Maven, and SBT build files
- **False Negatives**: Jib detection was failing due to limited file type scanning
- **Language Misidentification**: Scala projects with Gradle now correctly identified

### Testing
- ✅ **Java with Jib**: Correctly identifies Lane E with detailed reasoning
- ✅ **Scala with Jib**: Properly detects Lane E and "scala" language
- ✅ **Java without Jib**: Correctly falls back to Lane C for OSv optimization
- ✅ **Multiple Build Systems**: Supports Gradle, Maven, and SBT configurations

### Technical Details
- New `hasJibPlugin()` function with comprehensive detection patterns
- Extended `grep()` function to include build configuration files
- Improved conditional logic for language and lane selection
- Clear reasoning messages for debugging and user understanding

## [2025-08-18] - Python C-Extension Detection Enhancement

### Added
- **Comprehensive Python C-Extension Detection**
  - Enhanced `hasPythonCExtensions()` function with multi-layered detection
  - C/C++/Cython source file detection (`.c`, `.cc`, `.cpp`, `.cxx`, `.pyx`, `.pxd`)
  - Setuptools/distutils configuration analysis (`ext_modules`, `Extension()`)
  - Cython usage detection (`from Cython`, `cythonize`, `.pyx` files)
  - Popular C-extension library detection (numpy, scipy, pandas, psycopg2, lxml, pillow, cryptography, cffi)
  - Build configuration hints (`build_ext`, `include_dirs`, `library_dirs`)
  - CMake integration detection for Python bindings (pybind11)

### Improved
- **Lane Selection Logic**
  - Python projects with C-extensions → Lane C (full POSIX environment)
  - Python projects without C-extensions → Lane B (Unikraft POSIX layer)
  - Enhanced reasoning: "Python C-extensions detected - requires full POSIX environment"

- **File Search Capabilities**
  - Extended `grep()` function to search Python build files (`setup.py`, `pyproject.toml`, `requirements.txt`)
  - Added C++ file extensions (`.cpp`, `.cxx`) and Cython files (`.pyx`) to search scope
  - Added CMake file support (`CMakeLists.txt`) for Python binding projects

### Fixed
- **Detection Accuracy**: Previous implementation only checked basic `.c` files and `ext_modules`
- **False Negatives**: Projects with complex C-extension setups now properly detected
- **Library Dependencies**: Popular libraries requiring C-extensions automatically force Lane C

### Testing
- ✅ **Comprehensive C-Extension Detection**: Covers multiple detection methods
- ✅ **Popular Libraries**: numpy, scipy, pandas, cryptography properly detected
- ✅ **Build Systems**: setuptools, distutils, CMake configurations covered
- ✅ **Cython Support**: .pyx files and cythonize usage detection

### Status
**COMPLETED** - Phase 1, Step 3 from PLAN.md: "Fix Python C-extension detection in lane picker (should force Lane C)"

Python projects requiring C-extensions now reliably route to Lane C for full POSIX compatibility, while pure Python projects remain on optimal Lane B.