# Ploy CLI

- Env `PLOY_CONTROLLER` — base URL (`http://localhost:8081/v1` by default).

## Commands
### `ploy apps new`
```
ploy apps new --lang <go|node|rust|cpp> --name <app>
```
Scaffolds a minimal app with `/healthz` on port 8080.

**WebAssembly Support**: Create WASM-compatible applications using `--lang rust` for Rust WASM projects or `--lang cpp` for Emscripten-based C++ projects.

### `ploy push`
```
ploy push -a <app> [-lane A|B|C|D|E|F|G] [-main com.example.Main] [-sha <sha>]
```
Streams a tar of the working tree (respects `.gitignore`) to the api, which lane-picks and builds & deploys.

**Lane G - WebAssembly Support**: Applications with WASM compilation targets are automatically detected and routed to Lane G for WebAssembly deployment with wazero runtime.


### `ploy open`
```
ploy open <app>
```
Opens the app domain from `manifests/<app>.yaml` or falls back to `<app>.ployd.app`.

### `ploy domains` (implemented)
```
ploy domains add <app> <domain>
ploy domains list <app>  
ploy domains remove <app> <domain>
```
**Domain Management**: Register custom domains for applications, list associated domains, and remove domain mappings.

### `ploy certs` (implemented)
```
ploy certs issue <domain>
ploy certs list
```
**Certificate Management**: Issue TLS certificates via ACME protocol and list all managed certificates with expiration dates.

### `ploy debug` (implemented)
```
ploy debug shell <app> [--lane <A-F|G>]
```
**Debug Operations**: Create debug instances with SSH access enabled. Optionally specify lane for debug build.

**WASM Debug Support**: Lane G debug instances provide WASM runtime debugging with SSH access to the wazero runtime environment.

### `ploy rollback` (implemented)
```
ploy rollback <app> <sha>
```
**Rollback Operations**: Rollback application to a previous SHA version for quick recovery.

### `ploy env` (implemented)
```
ploy env set <app> <key> <value>
ploy env get <app> <key>
ploy env list <app>
ploy env delete <app> <key>
```
**Environment Variables**: Manage per-app environment variables available during build and deployment phases.

**Examples:**
```bash
# Set environment variables
ploy env set myapp NODE_ENV production
ploy env set myapp DATABASE_URL "postgres://localhost:5432/myapp"

# List all environment variables
ploy env list myapp

# Get specific variable
ploy env get myapp NODE_ENV

# Delete variable
ploy env delete myapp DEBUG
```

**Features:**
- Variables available during build process (Gradle, Maven, npm, etc.)
- Variables injected into runtime environment via Nomad templates
- Persistent storage across api restarts
- Full CRUD operations with user-friendly output

### `ploy arf` (implemented - Enhanced with Self-Healing)
```
ploy arf recipes list [--language <java|python|rust>] [--category <cleanup|modernize|security>] [--min-confidence <0.0-1.0>]
ploy arf recipes get <recipe-id>
ploy arf recipes search <query>
ploy arf recipes stats <recipe-id>
ploy arf transform [--recipe <recipe-id>] [--prompt <llm-prompt>] [--repository <url>] [--archive <path>] 
                   [--branch <branch>] [--plan-model <model>] [--exec-model <model>]
                   [--max-iterations <n>] [--parallel-tries <n>] [--timeout <duration>]
                   [--output <archive|diff|mr>] [--output-path <path>] [--report <minimal|standard|detailed>]
ploy arf health
ploy arf cache stats
ploy arf cache clear
```
**Automated Remediation Framework**: Execute code transformations with self-healing capabilities powered by LLM.

**Transform Command - Unified Transformation Engine:**
The `transform` command now consolidates all transformation, benchmarking, and testing functionality with advanced self-healing capabilities:

**Examples:**
```bash
# Execute recipe-based transformation
ploy arf transform --recipe cleanup.unused-imports --repository https://github.com/user/repo

# Execute LLM-guided transformation
ploy arf transform --prompt "Migrate from JUnit 4 to JUnit 5" --repository https://github.com/user/repo

# Combine recipes and LLM prompts for hybrid approach
ploy arf transform --recipe modernize.java-8-to-11 --prompt "Also update logging to SLF4J" --repository https://github.com/user/repo

# Advanced self-healing with parallel solution attempts
ploy arf transform --recipe complex.migration --max-iterations 5 --parallel-tries 3 --plan-model codellama:13b

# Output as merge request
ploy arf transform --recipe security.fixes --output mr --output-path ./security-fixes.patch

# Detailed reporting with custom timeout
ploy arf transform --prompt "Optimize database queries" --report detailed --timeout 30m

# Transform local archive
ploy arf transform --archive ./myproject.tar.gz --recipe cleanup.all

# Multi-iteration benchmarking (replaces old benchmark command)
ploy arf transform --recipe performance.optimize --repository https://github.com/user/app --max-iterations 10
```

**Self-Healing Features:**
- **Automatic Error Recovery**: When transformations fail, the system automatically attempts recovery
- **LLM-Powered Planning**: Uses specified models to plan solutions when errors occur
- **Parallel Solution Attempts**: Tries multiple fix approaches simultaneously for faster resolution
- **Iterative Refinement**: Continues attempting fixes up to max-iterations
- **Build & Deploy Testing**: Validates each solution with actual compilation and deployment

**Output Formats:**
- **archive**: Produces transformed code as tar.gz archive
- **diff**: Generates unified diff showing all changes
- **mr**: Creates merge request format with commit messages

**Report Levels:**
- **minimal**: Only critical information and final status
- **standard**: Includes timing, key metrics, and major events
- **detailed**: Comprehensive logs with all transformation steps

**Migration from Legacy Commands:**
- `ploy arf sandbox` → Use `transform` with deployment testing (automatic)
- `ploy arf benchmark` → Use `transform --max-iterations N` for multi-run testing
- `ploy arf workflow` → Use `transform` with LLM prompts for complex workflows

**Features:**
- **2,800+ OpenRewrite Recipes**: Java transformations for cleanup, modernization, security, and migration
- **LLM-Powered Transformations**: Natural language prompts for custom transformations
- **Self-Healing Engine**: Automatic error recovery with parallel solution attempts
- **Hybrid Approach**: Combine recipes and LLM prompts for maximum flexibility
- **Performance Caching**: Memory-mapped AST caching for 60% faster analysis
- **Multi-Repository Support**: Transform code from local archives or remote repositories
- **Comprehensive Testing**: Automatic build and deployment validation

### `ploy webhooks` (planned)
```
ploy webhooks add <app> <url> [--events build.completed,deploy.failed] [--secret <secret>]
ploy webhooks list <app>
ploy webhooks remove <app> <webhook-id>
```
**ARF Integration**: Configure webhooks for ARF transformation events and external system integration.

## WebAssembly (WASM) Commands

### Usage Examples for Lane G
```bash
# Deploy Rust WASM application (auto-detected)
ploy push -a rust-wasm-app

# Deploy Go WASM application (auto-detected)  
ploy push -a go-wasm-app

# Deploy AssemblyScript application (auto-detected)
ploy push -a assemblyscript-app

# Force Lane G deployment
ploy push -a my-app -lane G

# Create WASM debug instance
ploy debug shell my-wasm-app -lane G

# Check WASM app status
ploy open my-wasm-app
```

### WASM-Specific Features
- **Automatic Detection**: WASM targets automatically routed to Lane G
- **Multi-Language Support**: Rust, Go, C/C++, AssemblyScript compilation
- **Runtime Features**: wazero runtime with WASI Preview 1 support
- **Security**: OPA policies with WASM-specific validation
- **Component Model**: Multi-module WASM applications supported
- **Performance**: 10-50ms boot times, 5-30MB footprint
