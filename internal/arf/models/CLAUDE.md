# ARF Models Module CLAUDE.md

## Purpose
Core data models for the ARF (Application Recipe Framework) including recipes, transforms, and LLM model registry structures.

## Architecture Overview
The models package defines the fundamental data structures used throughout the Ploy platform. It includes recipe definitions for deployment configurations, transformation models for data processing, and comprehensive LLM model structures for the registry system with validation and lifecycle management.

## Module Structure
- `recipe.go` - Application deployment recipe models
- `transform.go` - Data transformation model definitions
- `llm.go:1-132` - Complete LLM model structure with validation and lifecycle methods
- Test files providing comprehensive validation and edge case coverage

## Key Components
### LLM Model (llm.go)
- `LLMModel:11-22` - Core model structure with metadata, capabilities, and configuration
- `ValidProviders:25` - Supported model providers (openai, anthropic, azure, local)
- `ValidCapabilities:28` - Available model capabilities (code, analysis, planning, reasoning, multimodal, function_calling)
- `ModelIDPattern:31` - Regex pattern for valid model ID format
- `Validate:34-75` - Comprehensive model validation with field and format checks
- `SetSystemFields:78-84` - Timestamp management for created/updated fields
- `GetProviderFromID:107-121` - Provider extraction from model ID
- `HasCapability:124-131` - Capability presence checking

### Model Structure
- **Identification**: ID, Name, Provider, Version for unique model identification
- **Capabilities**: Array of supported capabilities with provider validation
- **Configuration**: Provider-specific config map for API endpoints, parameters
- **Limits**: MaxTokens for context window, optional CostPerToken for billing
- **Timestamps**: Created and Updated timestamps with custom Time type

### Recipe and Transform Models
- Recipe models for application deployment configurations
- Transform models for data processing and manipulation workflows
- Integration with ARF system for deployment automation

## Data Validation
### LLM Model Validation
- **Required Fields**: ID, Name, Provider, Capabilities must be present
- **ID Format**: Must match ModelIDPattern (alphanumeric with @, /, -, _ allowed)
- **Provider Validation**: Must be from ValidProviders list
- **Capability Validation**: Must be from ValidCapabilities list
- **Token Limits**: MaxTokens > 0, CostPerToken >= 0
- **Comprehensive Error Messages**: Detailed validation failure descriptions

### Lifecycle Management
- **Creation**: SetSystemFields populates Created timestamp
- **Updates**: SetSystemFields updates Updated timestamp while preserving Created
- **Provider Extraction**: Smart parsing of provider from ID format
- **Capability Queries**: Efficient capability presence checking

## Dependencies
- Standard: fmt, regexp, strings, time for validation and processing
- No external dependencies - pure Go standard library
- Custom Time type for JSON serialization consistency

## Integration Points
### Consumes
- JSON/YAML unmarshaling from API requests and file inputs
- Time values from system for timestamp management

### Provides
- Model Structures: Used by validation, storage, and API layers
- Validation Logic: Built-in validation methods for data integrity
- Helper Methods: Provider extraction and capability checking
- JSON Serialization: Proper JSON tags for API communication

## Usage Patterns
### Model Creation
- Instantiate LLMModel with required fields
- Call Validate() to ensure data integrity
- Use SetSystemFields() to populate timestamps

### Model Queries
- Use HasCapability() to check for specific capabilities
- Use GetProviderFromID() to extract provider information
- Validate updates before persistence

## Testing
- Comprehensive test coverage for all validation scenarios
- Edge case testing for ID formats and provider validation
- Capability validation testing across all supported providers
- Timestamp management and lifecycle testing

## Patterns & Conventions
- Embedded validation in model structures
- Immutable field identification for updates
- Provider-specific capability management
- Consistent error messaging across validation methods
- Time type consistency for JSON serialization

## Related Documentation
- `../../validation/CLAUDE.md` - Extended validation logic using these models
- `../../storage/CLAUDE.md` - Persistence layer for model storage
- `../../../api/llms/CLAUDE.md` - API endpoints consuming these models
- `../../../cmd/ployman/CLAUDE.md` - CLI operations with these models