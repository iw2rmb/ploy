# OpenRewrite Recipes Guide

This guide covers how to discover, manage, and run OpenRewrite recipes using the Ploy platform.

## Table of Contents
- [Quick Start](#quick-start)
- [Recipe Discovery](#recipe-discovery)
- [Running Transformations](#running-transformations)
- [Recipe Management](#recipe-management)
- [API Reference](#api-reference)
- [Common Recipes](#common-recipes)

## Quick Start

### List available recipes
```bash
ploy arf recipes list
```

### Search for recipes
```bash
ploy arf recipes search "java migration"
```

### Run a transformation
```bash
ploy arf transform --recipe org.openrewrite.java.RemoveUnusedImports \
  --repo https://github.com/yourorg/yourrepo \
  --branch main
```

## Recipe Discovery

The Ploy platform provides comprehensive recipe discovery features through both CLI and API.

### CLI Commands

#### List All Recipes
```bash
# List all recipes in table format (default)
ploy arf recipes list

# List with JSON output
ploy arf recipes list --output json

# Filter by language
ploy arf recipes list --language java

# Filter by category
ploy arf recipes list --category migration

# Filter by pack
ploy arf recipes list --pack rewrite-spring

# Filter by version
ploy arf recipes list --version 5.0.0

# Combine pack and version filters
ploy arf recipes list --pack rewrite-java --version 8.1.0

# Pagination
ploy arf recipes list --limit 20 --offset 40
```

#### Search Recipes
```bash
# Search by keyword
ploy arf recipes search "spring boot"

# Search with limit
ploy arf recipes search "unused imports" --limit 5

# Search with verbose output
ploy arf recipes search "java 17" --verbose
```

#### Show Recipe Details
```bash
# Display recipe details
ploy arf recipes show org.openrewrite.java.RemoveUnusedImports

# Show in YAML format
ploy arf recipes show org.openrewrite.java.RemoveUnusedImports --output yaml
```

#### Get Recipe Statistics
```bash
# View usage statistics
ploy arf recipes stats org.openrewrite.java.RemoveUnusedImports
```

### Catalog Mode

Enable lightweight catalog mode for faster searches:
```bash
export PLOY_RECIPES_CATALOG=true
ploy arf recipes list
```

Server routes can be enabled with:
```bash
export PLOY_ENABLE_RECIPES_CATALOG=true
```

## Running Transformations

### Basic Transform Command
```bash
ploy arf transform \
  --recipe org.openrewrite.java.RemoveUnusedImports \
  --repo https://github.com/example/myproject \
  --branch main
```

### Advanced Options
```bash
ploy arf transform \
  --recipe org.openrewrite.java.migrate.UpgradeToJava17 \
  --repo https://github.com/example/myproject \
  --branch develop \
  --output-dir ./transformed \
  --iterations 3 \
  --report
```

### Transform with Auto-Healing
The platform automatically attempts to fix build/test failures:
```bash
ploy arf transform \
  --recipe org.openrewrite.spring.UpgradeSpringBoot_3_2 \
  --repo https://github.com/example/spring-app \
  --auto-heal
```

### Recipe Validation
If you specify an invalid recipe ID, the system will suggest alternatives:
```bash
$ ploy arf transform --recipe org.openrewrite.java.RemoveUnusedImport
```

The API returns a 400 error with suggestions:
```json
{
  "error": "invalid recipe_id",
  "message": "Recipe not found in catalog",
  "recipe_id": "org.openrewrite.java.RemoveUnusedImport",
  "suggestions": [
    "org.openrewrite.java.RemoveUnusedImports",
    "org.openrewrite.java.RemoveUnusedLocalVariables",
    "org.openrewrite.java.RemoveUnusedPrivateMethods"
  ]
}
```

## Recipe Management

### Upload Custom Recipe
```bash
# Upload a new recipe from YAML file
ploy arf recipes upload my-recipe.yaml

# Dry run to validate without uploading
ploy arf recipes upload my-recipe.yaml --dry-run

# Force upload even with warnings
ploy arf recipes upload my-recipe.yaml --force
```

### Download Recipe
```bash
# Download recipe to file
ploy arf recipes download org.openrewrite.java.RemoveUnusedImports

# Download to specific file
ploy arf recipes download org.openrewrite.java.RemoveUnusedImports \
  --output custom-name.yaml
```

### Validate Recipe File
```bash
# Basic validation
ploy arf recipes validate my-recipe.yaml

# Strict validation
ploy arf recipes validate my-recipe.yaml --strict
```

### Update Recipe
```bash
ploy arf recipes update org.custom.MyRecipe updated-recipe.yaml
```

### Delete Recipe
```bash
# Delete with confirmation
ploy arf recipes delete org.custom.MyRecipe

# Force delete without confirmation
ploy arf recipes delete org.custom.MyRecipe --force
```

## API Reference

### Endpoints

#### List Recipes
```http
GET /v1/arf/recipes
```

Query parameters:
- `page` - Page number (default: 1)
- `limit` - Items per page (default: 10)
- `category` - Filter by category
- `language` - Filter by language
- `query` - Search query
- `pack` - Filter by pack
- `version` - Filter by version

Example:
```bash
curl https://api.ployman.app/v1/arf/recipes?language=java&limit=20
```

#### Get Recipe Details
```http
GET /v1/arf/recipes/:id
```

Example:
```bash
curl https://api.ployman.app/v1/arf/recipes/org.openrewrite.java.RemoveUnusedImports
```

#### Search Recipes
```http
GET /v1/arf/recipes/search?q=<query>
```

Example:
```bash
curl "https://api.ployman.app/v1/arf/recipes/search?q=spring%20migration"
```

#### Refresh Catalog
```http
POST /v1/arf/recipes/refresh
```

Rebuilds the catalog by re-indexing recipe packs.

#### Execute Transformation
```http
// Removed: ARF transforms endpoint. Use Transflow instead.
```

Request body:
```json
{
  "recipe_id": "org.openrewrite.java.RemoveUnusedImports",
  "type": "openrewrite",
  "codebase": {
    "repository": "https://github.com/example/project",
    "branch": "main",
    "language": "java"
  }
}
```

Response includes transformation ID and status URL for monitoring:
```json
{
  "transformation_id": "abc-123-def",
  "status": "initiated",
  "status_url": "/v1/transflow/status/tf-abc123",
  "message": "Transformation started, use status_url to monitor progress"
}
```

#### Check Transformation Status
```http
GET /v1/transflow/status/:id
```

Returns current status, progress, and any healing attempts.

## Common Recipes

### Java Code Cleanup
- `org.openrewrite.java.RemoveUnusedImports` - Remove unused import statements
- `org.openrewrite.java.RemoveUnusedLocalVariables` - Remove unused local variables
- `org.openrewrite.java.RemoveUnusedPrivateMethods` - Remove unused private methods
- `org.openrewrite.java.RemoveUnusedPrivateFields` - Remove unused private fields
- `org.openrewrite.java.format.AutoFormat` - Format Java code

### Java Version Migration
- `org.openrewrite.java.migrate.Java8toJava11` - Migrate from Java 8 to Java 11
- `org.openrewrite.java.migrate.Java11toJava17` - Migrate from Java 11 to Java 17
- `org.openrewrite.java.migrate.UpgradeToJava17` - Direct upgrade to Java 17
- `org.openrewrite.java.migrate.UpgradeToJava21` - Upgrade to Java 21

### Spring Boot Migration
- `org.openrewrite.spring.UpgradeSpringBoot_2_7` - Upgrade to Spring Boot 2.7
- `org.openrewrite.spring.UpgradeSpringBoot_3_0` - Upgrade to Spring Boot 3.0
- `org.openrewrite.spring.UpgradeSpringBoot_3_1` - Upgrade to Spring Boot 3.1
- `org.openrewrite.spring.UpgradeSpringBoot_3_2` - Upgrade to Spring Boot 3.2

### Security Fixes
- `org.openrewrite.java.security.UseSecureRandom` - Replace insecure random with SecureRandom
- `org.openrewrite.java.security.RegularExpressionDenialOfService` - Fix ReDoS vulnerabilities
- `org.openrewrite.java.security.ZipSlip` - Fix Zip Slip vulnerabilities

## Recipe Composition

Chain multiple recipes together:
```bash
ploy arf recipes compose \
  org.openrewrite.java.RemoveUnusedImports \
  org.openrewrite.java.format.AutoFormat \
  --name "cleanup-and-format" \
  --repo https://github.com/example/project
```

## Transform-Time Validation

The API validates `recipe_id` when a catalog is available:

1. On transflow run, the plan can include OpenRewrite recipes which are checked against the catalog
2. If not found, returns 400 with suggestions based on fuzzy matching
3. The CLI surfaces these suggestions to help users correct typos

With `PLOY_RECIPES_CATALOG=true`, the CLI will preflight validate recipe IDs before execution.

## Best Practices

### 1. Test in Non-Production First
Always test recipes on a development branch before applying to production code.

### 2. Use Specific Recipe Versions
When possible, specify the exact recipe version for reproducible transformations.

### 3. Review Changes
Always review the changes made by recipes before merging:
```bash
git diff  # Review changes
git add -p  # Selectively stage changes
```

### 4. Enable Auto-Healing
For complex migrations, enable auto-healing to automatically fix compilation and test failures:
```bash
ploy arf transform --recipe <recipe-id> --auto-heal --iterations 3
```

### 5. Monitor Transformation Status
For long-running transformations, monitor progress:
```bash
# Start transformation
EXEC_ID=$(ploy transflow run -f config.yaml | jq -r '.execution_id')

# Check status
ploy arf transforms status $TRANSFORM_ID --watch
```

## Troubleshooting

### Recipe Not Found
If a recipe is not found, the system provides suggestions based on fuzzy matching:
- Check for typos in the recipe ID
- Use `ploy arf recipes search` to find the correct recipe
- Verify the recipe pack is available in your environment

### Transformation Failures
If a transformation fails:
1. Check the transformation status for error details
2. Review the build/test logs
3. Consider enabling auto-healing for automatic fixes
4. Verify the recipe is compatible with your codebase version

### Performance Issues
For large repositories:
- Use `--parallel` flag for concurrent processing
- Increase memory limits if needed
- Consider breaking the transformation into smaller chunks

## Environment Variables

- `PLOY_RECIPES_CATALOG` - Enable lightweight catalog mode for CLI
- `PLOY_ENABLE_RECIPES_CATALOG` - Enable catalog routes on server
- `PLOY_CONTROLLER` - API endpoint override
- `PLOY_APPS_DOMAIN` - Domain for API access
- `PLOY_ENVIRONMENT` - Environment setting (dev/prod)

## Test Repositories

The following repositories are verified to work with OpenRewrite recipes:

- `ploy-orw-test-java` - Basic Java project
  - Verified: `org.openrewrite.java.RemoveUnusedImports`
  
- `ploy-orw-test-legacy` - Legacy Java codebase
  - Recommended: `org.openrewrite.java.migrate.Java8toJava11` → `org.openrewrite.java.migrate.UpgradeToJava17`
  
- `ploy-orw-test-spring` - Spring Boot application
  - Use version-appropriate Boot migration recipes

## Further Resources

- [OpenRewrite Documentation](https://docs.openrewrite.org)
- [Recipe Catalog](https://docs.openrewrite.org/recipes)
- [Ploy Platform Documentation](https://docs.ployman.app)
- [GitHub Repository](https://github.com/iw2rmb/ploy)
