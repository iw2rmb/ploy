# Ployman CLI Module

## Purpose
Comprehensive CLI tool for Ploy platform management with specialized LLM model registry operations for transflow healing workflows.

## Architecture Overview
Ployman provides a unified command-line interface to the Ploy platform, with dedicated model management functionality for the LLM registry system. The CLI communicates with the API server for all operations and supports multiple output formats (table, JSON, YAML) with comprehensive error handling.

## Module Structure
- `main.go` - CLI entry point and command routing
- `models.go:1-490` - Complete LLM model management commands with CRUD operations
- `models_test.go` - Unit tests for model commands

## Key Components
### Main Application (main.go)
- Command-line argument parsing and routing
- Integration with existing Ploy CLI commands
- Global configuration and API endpoint management

### Model Management (models.go)
- `ModelsCmd:18-60` - Main command dispatcher for model operations
- `handleModelsList:90-174` - List models with filtering and pagination
- `handleModelsGet:176-232` - Retrieve individual model details
- `handleModelsAdd:234-294` - Create new models from JSON/YAML files
- `handleModelsUpdate:296-362` - Update existing model configurations
- `handleModelsDelete:364-393` - Delete models with confirmation prompts
- `handleModelsStats:395-452` - Display model usage statistics
- `makeHTTPRequest:455-489` - HTTP client for API communication

## Command Interface
### Available Commands
- `ployman models list [--provider openai] [--capability code] [--output json]`
- `ployman models get <model-id> [--output yaml]`
- `ployman models add --file model.json`
- `ployman models update <model-id> --file updated.yaml`
- `ployman models delete <model-id> [--force]`
- `ployman models stats <model-id>`

### Output Formats
- **table**: Human-readable tabular display (default)
- **json**: Structured JSON output for automation
- **yaml**: YAML format for configuration management

## Dependencies
- External: gopkg.in/yaml.v3 for YAML parsing
- Internal: internal/arf/models for model structures
- API: Communicates with /v1/llms/models/* endpoints
- HTTP: 30-second timeout with proper error handling

## Configuration
- API endpoint configured via controllerURL variable
- HTTP client with 30-second timeouts
- Support for JSON and YAML model definition files
- Interactive confirmation for destructive operations

## Integration Points
### Consumes
- API Server: /v1/llms/models endpoints for all operations
- File System: Model definition files (JSON/YAML)
- User Input: Interactive confirmations and command arguments

### Provides
- CLI Interface: Complete model management workflow
- File Format Support: JSON and YAML model definitions
- Error Handling: User-friendly error messages and API error translation
- Automation Support: JSON/YAML output for scripting integration

## Testing
- Test directory: `models_test.go`
- Unit tests for command parsing and HTTP operations
- Mock API responses for testing edge cases

## Patterns & Conventions
- Command-based architecture with action dispatchers
- Consistent flag parsing across all subcommands
- Graceful error handling with user-friendly messages
- Multi-format output support for different use cases
- Interactive confirmation for destructive operations

## Related Documentation
- `../../api/llms/` - REST API endpoints for model operations
- `../../internal/storage/CLAUDE.md` - Storage layer for model persistence
- `../../internal/arf/models/` - Core model data structures
- `../../internal/validation/` - Model validation logic