---
task: m-implement-model-registry-ployman
branch: feature/model-registry-ployman
status: pending
created: 2025-09-06
modules: [ployman, llm-models, storage, api]
---

# Model Registry Implementation for ployman CLI

## Problem/Goal
Implement the final remaining MVP requirement: Model registry in `ployman` CLI with schema validation stored under `llms` namespace. This will enable transflow healing workflows to reference and configure LLM models through a centralized registry system.

## Success Criteria
- [ ] Define LLM model schema with validation (ID, provider, capabilities, config, etc.)
- [ ] Implement REST API endpoints for model CRUD operations (`/v1/llms/models/*`)  
- [ ] Add storage layer integration with SeaweedFS under `llms/models/` namespace
- [ ] Create `ployman models` CLI commands (list, get, add, update, delete)
- [ ] Add comprehensive validation for model IDs, providers, and configurations
- [ ] Integrate with existing transflow healing workflow for model resolution
- [ ] Unit and integration test coverage for all components
- [ ] Documentation and usage examples for model registry operations

## Context Files
- @cmd/ployman/main.go - Existing CLI structure to extend
- @internal/arf/models/recipe.go - Recipe model pattern to follow
- @internal/cli/arf/recipes.go - Recipe CRUD operations pattern
- @api/arf/ - Existing ARF API structure to replicate for llms
- @internal/storage/ - Storage layer patterns
- @roadmap/transflow/MVP.md - Requirements specification

## User Notes
This completes the final requirement from the MVP roadmap. The implementation should:

### 1. Model Schema (`internal/arf/models/llm.go`)
```go
type LLMModel struct {
    ID           string            `json:"id"`           // e.g., "gpt-4o-mini@2024-08-06"
    Name         string            `json:"name"`         // Display name
    Provider     string            `json:"provider"`     // openai, anthropic, etc.
    Version      string            `json:"version"`      // Model version
    Capabilities []string          `json:"capabilities"` // ["code", "analysis", "planning"]
    Config       map[string]string `json:"config"`       // Provider-specific config
    MaxTokens    int              `json:"max_tokens"`   // Context window size
    CostPerToken float64          `json:"cost_per_token,omitempty"`
    Created      time.Time        `json:"created"`
    Updated      time.Time        `json:"updated"`
}
```

### 2. CLI Commands
```bash
ployman models list                    # List all models
ployman models get <id>                # Get model details  
ployman models add -f model.json       # Add from file
ployman models update <id> -f model.json # Update model
ployman models delete <id>             # Delete model
```

### 3. API Endpoints
- `GET /v1/llms/models` - List all models
- `GET /v1/llms/models/{id}` - Get specific model
- `POST /v1/llms/models` - Add new model
- `PUT /v1/llms/models/{id}` - Update model
- `DELETE /v1/llms/models/{id}` - Delete model

### 4. Storage Integration
- Follow existing SeaweedFS patterns from recipe storage
- Use `llms/models/` namespace for model persistence
- Implement proper locking and consistency

### 5. Validation Requirements
- Model ID format validation (provider@version pattern)
- Required fields: ID, name, provider, capabilities
- Numeric validation: max_tokens > 0, cost_per_token >= 0
- Provider whitelist: openai, anthropic, azure, local
- Capability validation against known types

### Implementation Files
1. **NEW** `internal/arf/models/llm.go` - Model schema
2. **NEW** `api/llms/handler.go` - API endpoints  
3. **NEW** `cmd/ployman/models.go` - CLI commands
4. **NEW** `internal/validation/llm.go` - Validation logic
5. **MODIFY** `cmd/ployman/main.go` - Add models command
6. **NEW** Test files for all components

This follows the same patterns established by the recipe management system but for LLM model registry.

## Work Log
- [2025-09-06] Created task based on MVP roadmap analysis