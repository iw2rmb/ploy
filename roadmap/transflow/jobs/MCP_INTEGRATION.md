# MCP Integration for LLM-exec Jobs

## Overview

Model Context Protocol (MCP) integration enables LLM-exec jobs in the transflow healing infrastructure to access external tools and resources for enhanced context gathering and processing. This integration allows LLM jobs to use MCP tools for file system operations, web scraping, database access, and other specialized operations.

## Architecture

The MCP integration consists of several key components:

1. **Configuration System**: Parse MCP tools from transflow YAML configuration
2. **Environment Management**: Convert MCP config to environment variables for containerized jobs
3. **Context Prefetching**: Fetch context files and URLs before job execution
4. **HCL Template Integration**: Extend Nomad job templates with MCP environment variables
5. **Validation Framework**: Comprehensive validation of MCP configurations and endpoints

## Configuration

### Basic MCP Configuration in Transflow YAML

```yaml
# transflow.yaml
version: v1alpha1
id: my-workflow
target_repo: org/project
target_branch: refs/heads/main

steps:
  - type: llm-exec
    id: apply-fix-with-mcp
    model: gpt-4o-mini@2024-08-06
    prompts:
      - "Fix null pointer exceptions using proper error handling"
    mcp_tools:
      - name: file-system
        endpoint: mcp://fs
        config:
          max_file_size: "1MB"
      - name: search
        endpoint: mcp://rg
        config:
          max_results: "100"
      - name: web-scraper
        endpoint: https://api.example.com/mcp/web
        config:
          timeout: "30s"
    context:
      - "src/**/*.java"
      - "pom.xml"
      - "https://docs.example.com/api/v2"
    budgets:
      max_tokens: 8000
      max_cost: 10
      timeout: "20m"
```

### MCP Tool Types

#### File System Tools
```yaml
mcp_tools:
  - name: file-system
    endpoint: mcp://fs
    config:
      root_path: "/workspace"
      allowed_extensions: [".java", ".xml", ".json"]
      max_file_size: "1MB"
```

#### Search Tools
```yaml
mcp_tools:
  - name: search
    endpoint: mcp://rg
    config:
      max_results: "100"
      case_sensitive: "false"
      include_line_numbers: "true"
```

#### Web Tools
```yaml
mcp_tools:
  - name: web-scraper
    endpoint: https://api.example.com/mcp/web
    config:
      timeout: "30s"
      max_content_length: "10MB"
      allowed_domains: ["docs.example.com", "api.example.com"]
```

## Environment Variables

The MCP integration automatically generates the following environment variables for LLM-exec jobs:

### Core MCP Variables
- `MCP_TOOLS_JSON`: JSON array of available MCP tools with their configurations
- `MCP_CONTEXT_JSON`: JSON array of context items to be processed
- `MCP_ENDPOINTS_JSON`: JSON object mapping tool names to endpoint URLs
- `MCP_BUDGETS_JSON`: JSON object with resource limits and timeouts
- `MCP_PROMPTS_JSON`: JSON array of prompts for the LLM execution
- `MCP_TIMEOUT`: Default timeout for MCP tool operations (e.g., "30m")
- `MCP_SECURITY_MODE`: Security policy enforcement ("allowlist" or "denylist")

### Example Environment Variable Values

```bash
# MCP_TOOLS_JSON
[
  {
    "name": "file-system",
    "endpoint": "mcp://fs",
    "config": {
      "max_file_size": "1MB"
    }
  },
  {
    "name": "search", 
    "endpoint": "mcp://rg",
    "config": {
      "max_results": "100"
    }
  }
]

# MCP_CONTEXT_JSON
[
  "src/**/*.java",
  "pom.xml",
  "https://docs.example.com/api/v2"
]

# MCP_ENDPOINTS_JSON
{
  "file-system": "mcp://fs",
  "search": "mcp://rg",
  "web-scraper": "https://api.example.com/mcp/web"
}

# MCP_BUDGETS_JSON
{
  "max_tokens": 8000,
  "max_cost": 10,
  "timeout": "20m"
}
```

## Context Prefetching

The MCP integration includes a context prefetching system that prepares context files and URLs before job execution:

### File Pattern Processing
- File patterns like `src/**/*.java` are processed into manifest files
- Manifests describe what files should be available to the containerized job
- The actual file system access happens through MCP tools in the container

### URL Content Fetching
- HTTP/HTTPS URLs are fetched and saved to the context directory
- Content is preprocessed and made available as static files
- Future enhancement: Use MCP web tools for dynamic content fetching

### Context Manifest
A comprehensive manifest file is created at `/workspace/context/mcp_context_manifest.json`:

```json
{
  "mcp_config": {
    "tools": [...],
    "context": [...],
    "budgets": {...}
  },
  "context_items": ["src/**/*.java", "pom.xml"],
  "tools": [...],
  "workspace_dir": "/workspace",
  "context_dir": "/workspace/context",
  "prefetch_time": "2024-08-27T10:30:00Z"
}
```

## Nomad Job Template Integration

The LLM-exec HCL template has been extended with MCP environment variables:

```hcl
# roadmap/transflow/jobs/llm_exec.hcl
env = {
  # Core LLM configuration
  MODEL       = "${MODEL}"
  TOOLS       = "${TOOLS_JSON}"
  LIMITS      = "${LIMITS_JSON}"
  CONTEXT_DIR = "/workspace/context"
  OUTPUT_DIR  = "/workspace/out"
  RUN_ID      = "${RUN_ID}"
  
  # MCP integration configuration
  MCP_TOOLS_JSON      = "${MCP_TOOLS_JSON}"
  MCP_CONTEXT_JSON    = "${MCP_CONTEXT_JSON}"
  MCP_ENDPOINTS_JSON  = "${MCP_ENDPOINTS_JSON}"
  MCP_BUDGETS_JSON    = "${MCP_BUDGETS_JSON}"
  MCP_PROMPTS_JSON    = "${MCP_PROMPTS_JSON}"
  MCP_TIMEOUT         = "${MCP_TIMEOUT}"
  MCP_SECURITY_MODE   = "${MCP_SECURITY_MODE}"
}

network {
  # Allow outbound connections for MCP endpoints
  mode = "bridge"
  port "http" {
    to = 8080
  }
  dns {
    servers = ["8.8.8.8", "1.1.1.1"]
  }
}
```

## Security Considerations

### Endpoint Validation
- MCP endpoints must use `mcp://`, `http://`, or `https://` schemes
- Endpoint URLs are validated for basic format correctness
- Tool names must be non-empty and unique within a configuration

### Network Security
- MCP jobs run with network access to configured endpoints
- DNS resolution is configured for reliable endpoint access
- Future enhancement: Network policies for endpoint allowlisting

### Resource Limits
- Configurable timeouts prevent runaway MCP tool operations
- Budget limits control token usage and execution costs
- Context size limits prevent excessive memory usage

## Usage Examples

### Basic File Processing
```yaml
steps:
  - type: llm-exec
    id: fix-java-nulls
    model: gpt-4o-mini@2024-08-06
    mcp_tools:
      - name: file-system
        endpoint: mcp://fs
    context:
      - "src/**/*.java"
    prompts:
      - "Fix all null pointer exceptions in the Java files"
```

### Web Documentation Integration
```yaml
steps:
  - type: llm-exec
    id: update-with-docs
    model: gpt-4o@2024-08-06
    mcp_tools:
      - name: file-system
        endpoint: mcp://fs
      - name: web-scraper
        endpoint: https://api.example.com/mcp/web
    context:
      - "src/main/java/**/*.java"
      - "https://docs.spring.io/spring-boot/docs/current/reference/html/"
    prompts:
      - "Update the code to use the latest Spring Boot best practices"
```

### Search-Enhanced Analysis
```yaml
steps:
  - type: llm-exec
    id: analyze-with-search
    model: gpt-4o-mini@2024-08-06
    mcp_tools:
      - name: file-system
        endpoint: mcp://fs
      - name: search
        endpoint: mcp://rg
    context:
      - "src/**"
      - "test/**"
    prompts:
      - "Find and fix all TODO comments in the codebase"
    budgets:
      max_tokens: 12000
      timeout: "15m"
```

## Error Handling

### Configuration Errors
- Invalid MCP tool configurations are caught during YAML parsing
- Endpoint validation errors are reported with specific tool names
- Timeout format errors include suggested valid formats

### Runtime Errors
- MCP tool failures are logged but don't fail the entire job
- Context prefetching errors are handled gracefully
- Network connectivity issues are retried with exponential backoff

### Fallback Behavior
- Jobs can run without MCP tools if configuration is invalid
- Default context processing falls back to file system operations
- Environment variables default to safe values if MCP config is missing

## Performance Considerations

### Context Prefetching Overhead
- File pattern processing: ~100ms per pattern
- URL fetching: Depends on network latency and content size
- Manifest creation: ~10ms for typical configurations

### Memory Usage
- MCP tool configurations: ~1KB per tool
- Context manifests: ~10KB for typical workflows
- Prefetched content: Variable based on context items

### Network Impact
- MCP endpoint connections: Established per job execution
- Context URL fetching: One-time overhead during prefetch phase
- DNS resolution caching reduces repeated lookup overhead

## Troubleshooting

### Common Issues

#### MCP Tool Not Found
```
Error: MCP tool 'file-system' endpoint must start with mcp://, http://, or https://
```
**Solution**: Verify endpoint URL format in transflow.yaml

#### Context Prefetching Failed
```
Error: failed to process context item 'https://example.com': HTTP error 404
```
**Solution**: Verify URL accessibility and network connectivity

#### Timeout Errors
```
Error: MCP timeout must be positive
```
**Solution**: Use valid duration format like "30m", "1h", "90s"

### Debug Mode
Enable verbose logging by setting environment variable:
```bash
export TRANSFLOW_DEBUG=1
```

This provides detailed logs for:
- MCP configuration parsing
- Context prefetching operations
- Environment variable generation
- Job submission details

## Future Enhancements

### Planned Features
- **MCP Server Integration**: Direct integration with MCP server protocols
- **Dynamic Tool Discovery**: Automatic discovery of available MCP tools
- **Caching Layer**: Cache MCP tool responses for improved performance
- **Advanced Security**: Fine-grained permissions and endpoint allowlisting
- **Monitoring**: Metrics collection for MCP tool usage and performance

### Compatibility
- **Backward Compatibility**: Existing LLM-exec jobs continue to work without MCP configuration
- **Graceful Degradation**: Jobs function normally if MCP tools are unavailable
- **Progressive Enhancement**: MCP features can be adopted incrementally