# API LLMs Module CLAUDE.md

## Purpose
REST API endpoints for LLM model registry management providing complete CRUD operations, filtering, and statistics for mods healing workflows.

## Architecture Overview
The LLMs API module provides HTTP endpoints for managing LLM models in the registry. Built on Fiber framework with comprehensive validation, error handling, and storage integration. Supports filtering by provider and capability, with plans for usage statistics tracking.

## Module Structure
- `handler.go:1-360` - Complete HTTP handlers for all model operations
- Test files for endpoint validation and error handling

## Key Components
### Handler Structure (handler.go)
- `Handler:17-28` - Main handler with storage and validation dependencies
- `NewHandler:23-28` - Handler constructor with dependency injection
- `RegisterRoutes:31-44` - Route registration under /v1/llms/models namespace

### API Endpoints
- `ListModels:47-114` - GET /v1/llms/models with provider/capability filtering
- `GetModel:117-151` - GET /v1/llms/models/{id} for individual model retrieval
- `CreateModel:154-208` - POST /v1/llms/models for model creation
- `UpdateModel:211-290` - PUT /v1/llms/models/{id} for model updates
- `DeleteModel:293-328` - DELETE /v1/llms/models/{id} for model deletion
- `GetModelStats:331-359` - GET /v1/llms/models/{id}/stats for usage statistics

## API Interface
### Endpoints
- `GET /v1/llms/models?provider=openai&capability=code&limit=20&offset=0`
- `GET /v1/llms/models/{id}`
- `POST /v1/llms/models` (Content-Type: application/json)
- `PUT /v1/llms/models/{id}` (Content-Type: application/json)
- `DELETE /v1/llms/models/{id}`
- `GET /v1/llms/models/{id}/stats`

### Response Formats
- **List**: `{"models": [...], "count": 5, "total": 15}`
- **Individual**: Model object with all fields
- **Create/Update**: `{"id": "model-id", "message": "success"}`
- **Stats**: Usage metrics including requests, success rate, costs

## Dependencies
- External: github.com/gofiber/fiber/v2 for HTTP framework
- Internal: internal/arf/models for model structures
- Internal: internal/storage for persistence layer
- Internal: internal/validation for model validation

## Configuration
- Route registration under /v1 API version
- JSON request/response content types
- Storage operations under llms/models/ namespace
- Comprehensive error handling with appropriate HTTP status codes

## Integration Points
### Consumes
- Storage Layer: internal/storage.Storage interface for persistence
- Validation Layer: internal/validation.LLMModelValidator for model validation
- Request Bodies: JSON-formatted model definitions

### Provides
- REST API: Complete HTTP interface for model management
- JSON Responses: Structured data for CLI and web interfaces
- Error Handling: HTTP status codes with descriptive error messages
- Filtering Support: Provider and capability-based model filtering

## Validation & Error Handling
- Request body parsing with detailed error messages
- Model validation using dedicated validator
- Existence checks for create/update operations
- ID consistency validation for updates
- Storage error translation to appropriate HTTP responses

## Testing
- Unit tests for all endpoint handlers
- Mock storage layer for isolated testing
- Error case coverage for all operations
- Integration tests with real storage backend

## Patterns & Conventions
- RESTful API design with standard HTTP methods
- Consistent error response format across all endpoints
- Dependency injection for testability
- Storage abstraction for backend flexibility
- Context-based request handling

## Related Documentation
- `../../cmd/ployman/README.md` - CLI client for these endpoints
- `../../internal/storage/CLAUDE.md` - Storage layer implementation
- `../../internal/arf/models/` - Core model data structures
- `../../internal/validation/` - Model validation logic
- `../server/` - Server registration and routing
