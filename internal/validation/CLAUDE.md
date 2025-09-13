# Validation Module CLAUDE.md

## Purpose
Comprehensive validation logic for application names, environment variables, resources, and LLM models across the Ploy platform.

## Architecture Overview
The validation module provides specialized validators for different types of platform entities. Each validator implements domain-specific rules and constraints, with the LLM model validator providing comprehensive provider-specific validation for the model registry system.

**E2E Validation Infrastructure** - Complete end-to-end validation framework with VPS testing capabilities supporting comprehensive workflow validation. Provides validation infrastructure for mods workflows including Java migration testing, self-healing validation, KB learning verification, and GitLab integration testing. Framework enables production environment testing with real repository operations and distributed job orchestration.

## Module Structure
- `app_name.go` - Application name validation with platform constraints
- `env_vars.go` - Environment variable validation and sanitization
- `resources.go` - Resource specification validation
- `llm.go:1-277` - Comprehensive LLM model validation with provider-specific rules
- Test files providing comprehensive coverage for all validators

## Key Components
### LLM Model Validation (llm.go)
- `LLMModelValidator:12-17` - Main validator structure
- `ValidateLLMModel:20-48` - Complete model validation orchestration
- `validateModelID:51-73` - Model ID format and constraint validation
- `validateProviderSpecificConfig:76-90` - Provider-specific configuration validation
- `validateOpenAIConfig:93-111` - OpenAI model configuration rules
- `validateAnthropicConfig:114-130` - Anthropic model configuration rules
- `validateAzureConfig:133-155` - Azure OpenAI configuration requirements
- `validateLocalConfig:158-174` - Local model configuration validation
- `validateCapabilitiesForProvider:177-206` - Provider capability compatibility checks
- `validateTokenLimits:209-237` - Token limit validation with provider constraints
- `ValidateModelUpdate:240-260` - Update-specific validation with immutable field checks

### Other Validators
- Application name validation for platform naming conventions
- Environment variable validation for deployment configurations
- Resource specification validation for infrastructure requirements

## Validation Rules
### LLM Models
- **ID Format**: 3-100 characters, alphanumeric with allowed special chars (-, _, @, /, .)
- **Provider Constraints**: openai, anthropic, azure, local with specific config requirements
- **Token Limits**: 1000-2000000 general range with provider-specific maximums
- **Capabilities**: Provider-specific capability validation (e.g., function_calling for OpenAI/Azure only)
- **Immutable Fields**: ID and provider cannot be changed during updates
- **Config Validation**: Provider-specific required and optional configuration keys

### Provider-Specific Rules
- **OpenAI**: Max 1M tokens, supports all capabilities
- **Anthropic**: Max 200K tokens, no function_calling capability
- **Azure**: Requires deployment_name, inherits OpenAI capabilities
- **Local**: Max 100K tokens (memory considerations), basic capabilities only

## Dependencies
- Internal: internal/arf/models for LLM model structures
- Standard: regexp for pattern matching validation
- Standard: strings for text processing and validation

## Integration Points
### Consumes
- Model Structures: internal/arf/models.LLMModel for validation
- Configuration Maps: Provider-specific config validation

### Provides
- Validation Services: Used by API handlers and CLI operations
- Error Messages: Detailed validation failure descriptions
- Format Validation: Standalone ID format validation functions
- Update Validation: Specialized validation for model updates
- E2E Validation Framework: Comprehensive workflow testing infrastructure with VPS support
- Production Testing: Real environment validation for mods and healing workflows

## Testing
- Comprehensive test coverage for all validation scenarios
- Provider-specific configuration testing
- Edge case validation for limits and constraints
- Update validation testing with immutable field checks

## Patterns & Conventions
- Structured validation with specific error messages
- Provider-specific validation delegation
- Immutable field enforcement for updates
- Comprehensive constraint checking with reasonable defaults
- Helper functions for common validation patterns

## Related Documentation
- `../../api/llms/CLAUDE.md` - API handlers using validation services
- `../arf/models/` - Model structures being validated
- `../../cmd/ployman/README.md` - CLI operations with validation
- `../storage/CLAUDE.md` - Storage operations requiring valid models
