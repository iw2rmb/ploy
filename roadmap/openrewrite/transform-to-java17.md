# OpenRewrite Java 17 Transformation Test Plan

## Unified ARF System Approach

**Strategy**: Use the unified ARF (Automated Remediation Framework) system with OpenRewrite integration through standard ARF endpoints and recipe management.

**Key Assumptions**:
1. **Recipe Auto-Download**: If recipe is not available locally, the openrewrite-jvm image will automatically download and store it during transformation
2. **Extended Timeouts**: All timeouts are set to generous values to ensure processes have sufficient time to complete correctly

## Phase 1: Direct OpenRewrite Transformation Testing

### Step 1: Execute ARF Transformation with OpenRewrite
1. **Transform Request**: Use `/v1/arf/transform` endpoint with OpenRewrite recipe
   - **Timeout**: 30 minutes (generous time for repository cloning, recipe download, and transformation)
   - **Extended Processing Time**: Allow for recipe download if not cached
2. **Target Repository**: https://github.com/winterbe/java8-tutorial.git
3. **Recipe**: `org.openrewrite.java.migrate.UpgradeToJava17` (unified Java 8→17 recipe)
4. **Request Format**:
   ```json
   {
     "repository_url": "https://github.com/winterbe/java8-tutorial.git",
     "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
     "recipe_type": "openrewrite",
     "branch": "master",
     "configuration": {
       "target_recipes": ["org.openrewrite.java.migrate.UpgradeToJava17"],
       "package_manager": "maven",
       "target_jdk": "17"
     }
   }
   ```

### Step 2: Monitor Transformation Execution
1. **Job Submission**: Receive transformation_id from transform request
   - **Initial Response Timeout**: 2 minutes
2. **Status Monitoring**: Poll `/v1/arf/transforms/{transformation_id}` endpoint
   - **Polling Interval**: 30 seconds
   - **Maximum Wait Time**: 30 minutes total (allows for recipe download + transformation)
   - **Status Check Timeout**: 10 seconds per poll
3. **Progress Tracking**: Monitor transformation job until completion
   - **Expected Phases**: Repository clone → Recipe download (if needed) → Transformation → Result packaging
4. **Result Retrieval**: Get transformation output/diff when available

### Step 3: Post-Transformation Recipe Verification
1. **Recipe Storage Check**: After successful transformation, verify that recipes are available
   - **Endpoint**: `/v1/arf/recipes?type=openrewrite` (should show OpenRewrite recipes)
   - **Expected**: Recipe cache populated with `org.openrewrite.java.migrate.UpgradeToJava17`
2. **Cache Verification**: Confirm that subsequent transformations are faster due to cached recipes
3. **Performance Comparison**: Second transformation should be significantly faster (no recipe download)

### Step 4: Test Multiple Repositories (if basic test succeeds)
1. **Simple Java Tutorial**: winterbe/java8-tutorial (already tested)
   - **Timeout**: 30 minutes (first run with recipe download)
2. **Baeldung Tutorials**: eugenp/tutorials (larger codebase)
   - **Timeout**: 60 minutes (larger codebase, recipes should be cached)
3. **Java Design Patterns**: iluwatar/java-design-patterns (well-structured)
   - **Timeout**: 45 minutes (medium complexity, cached recipes)

## Timeout Configuration Strategy

### API Endpoint Timeouts:
- **Recipe List/Validate**: 60 seconds (recipe resolution)
- **Transform Submission**: 2 minutes (transformation job creation and validation)
- **Status Polling**: 10 seconds per request
- **Total Job Wait**: 30 minutes maximum per transformation

### Job Execution Timeouts:
- **Repository Clone**: 5 minutes
- **Recipe Download**: 15 minutes (Maven Central + artifact resolution)
- **Transformation Execution**: 15 minutes (large codebases)
- **Result Packaging**: 5 minutes
- **Total Job Timeout**: 30 minutes per transformation job

### Retry and Circuit Breaker Configuration:
- **Status Poll Retries**: 3 attempts with 5-second backoff
- **Network Timeout**: 30 seconds for external repository access
- **Docker Image Pull**: 10 minutes (openrewrite-jvm image + dependencies)

## Expected Outcomes

### Success Criteria:
- ✅ Recipe listing works via `/v1/arf/recipes?type=openrewrite`
- ✅ Recipe validation succeeds for Java migration recipes via `/v1/arf/recipes/validate`
- ✅ Transform job submission returns transformation_id within 2 minutes
- ✅ Transformation status tracking provides progress updates every 30 seconds
- ✅ At least one repository transformation completes successfully within 60 minutes
- ✅ **Recipe Caching**: Recipes are downloaded and stored after first transformation
- ✅ **Performance Improvement**: Subsequent transformations are 50%+ faster with cached recipes

### Performance Targets (with generous timeouts):
- **First Transformation** (with recipe download):
  - Endpoint Response Time: <2 minutes for job submission
  - Job Completion: <30 minutes for simple repositories
  - Success Rate: >80% for Tier 1 (simple) repositories
- **Subsequent Transformations** (with cached recipes):
  - Endpoint Response Time: <30 seconds for job submission
  - Job Completion: <30 minutes for simple repositories
  - Success Rate: >95% for Tier 1 repositories

## Recipe Management Validation

### Pre-Transformation State:
- **Recipe Catalog**: Managed through unified ARF recipe system
- **Expected Behavior**: OpenRewrite recipes are available via ARF recipe endpoints

### During Transformation:
- **Recipe Download Phase**: Monitor job logs for Maven artifact downloads
- **Expected Downloads**: 
  - `org.openrewrite.recipe:rewrite-migrate-java:latest`
  - Dependencies and transitive dependencies
  - Recipe metadata and configuration

### Post-Transformation State:
- **Recipe Catalog**: Contains OpenRewrite recipes accessible via ARF endpoints
- **Verification Method**: Call `/v1/arf/recipes?type=openrewrite` to confirm available recipes
- **Storage Location**: Recipes stored in openrewrite-jvm image or persistent volume
- **Performance Impact**: Second transformation should skip download phase

## Advantages of This Approach:
1. **Component Independence**: Bypasses failed advanced components
2. **Direct Testing**: Tests core OpenRewrite functionality directly
3. **Simplified Debugging**: Easier to isolate issues to specific components
4. **Incremental Validation**: Can verify each step independently
5. **Production Pathway**: Uses the same underlying infrastructure as full ARF
6. **Recipe Auto-Management**: Validates automatic recipe download and caching
7. **Realistic Timeouts**: Generous timeouts ensure reliable completion

## Risk Mitigation:
- **Docker Dependencies**: OpenRewrite uses Docker images - verify Docker/registry access
- **Nomad Integration**: Transformation jobs run via Nomad - confirm job execution capability
- **Network Access**: Verify API can clone repositories and access Maven Central for recipe downloads
- **Resource Limits**: Monitor job resource consumption and timeout handling
- **Recipe Download Failures**: Handle cases where Maven Central or recipe repositories are unavailable
- **Storage Persistence**: Ensure recipe cache survives container restarts

## Monitoring and Debugging:
- **Job Log Access**: Monitor Nomad job logs for recipe download progress
- **Network Connectivity**: Verify access to GitHub (repositories) and Maven Central (recipes)
- **Storage Utilization**: Monitor disk usage during recipe downloads
- **Performance Metrics**: Track transformation times before/after recipe caching

## API Endpoint Reference

Based on `/api/README.md`, OpenRewrite integration uses the unified ARF system:

### Unified ARF System for OpenRewrite
- `POST /v1/arf/transform` — execute transformation (including OpenRewrite recipes)
- `GET /v1/arf/transforms/:id` — get transformation result

**Note**: OpenRewrite recipes are managed exclusively through the unified `/v1/arf/recipes/*` endpoints with `type: "openrewrite"`.

### General ARF Recipe Management (Lines 137-148)
- `GET /v1/arf/recipes` — list available transformation recipes
- `GET /v1/arf/recipes/:id` — get detailed recipe information
- `POST /v1/arf/recipes` — create new transformation recipe
- `PUT /v1/arf/recipes/:id` — update existing recipe
- `DELETE /v1/arf/recipes/:id` — delete recipe from catalog
- `GET /v1/arf/recipes/search` — search recipes by name or tags
- `POST /v1/arf/recipes/upload` — upload recipe
- `POST /v1/arf/recipes/validate` — validate recipe
- `GET /v1/arf/recipes/:id/download` — download recipe
- `GET /v1/arf/recipes/:id/metadata` — get recipe metadata
- `GET /v1/arf/recipes/:id/stats` — get recipe usage statistics
- `POST /v1/arf/recipes/register` — register recipe from runner

**Important**: The API documentation shows that OpenRewrite uses a unified recipe management system through `/v1/arf/recipes/*` endpoints with `type: "openrewrite"` parameter, rather than dedicated OpenRewrite recipe endpoints.

## Test Execution Commands

### Step 1: Execute Transformation via Unified ARF
```bash
# Using unified ARF endpoint
curl -X POST "${PLOY_CONTROLLER%/v1}/v1/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "repository_url": "https://github.com/winterbe/java8-tutorial.git",
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "recipe_type": "openrewrite",
    "branch": "master",
    "configuration": {
      "target_recipes": ["org.openrewrite.java.migrate.UpgradeToJava17"],
      "package_manager": "maven",
      "target_jdk": "17"
    }
  }' \
  --max-time 120  # 2 minute timeout for transformation submission
```

### Step 2: Monitor Transformation Status (with 60-minute maximum wait)
```bash
TRANSFORM_ID="<transformation_id_from_step_1>"
timeout 3600 bash -c '
  while true; do
    STATUS=$(curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/transforms/'$TRANSFORM_ID'" | jq -r ".status")
    echo "$(date): Transformation status: $STATUS"
    if [[ "$STATUS" == "completed" || "$STATUS" == "failed" ]]; then
      break
    fi
    sleep 30
  done
'
```

### Step 3: Verify Recipe Availability
```bash
# Check OpenRewrite recipes using unified ARF recipe system
curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/recipes?type=openrewrite" | jq '.recipes | length'
# Should show available OpenRewrite recipes

# List all OpenRewrite recipes with details
curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/recipes?type=openrewrite" | jq '.recipes[]'
```

### Recipe Validation (Optional)
```bash
# Validate OpenRewrite recipe before transformation
curl -X POST "${PLOY_CONTROLLER%/v1}/v1/arf/recipes/validate" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "configuration": {
      "target_recipes": ["org.openrewrite.java.migrate.UpgradeToJava17"],
      "package_manager": "maven"
    }
  }'
```

## Documentation Updates:
- Update main roadmap with direct OpenRewrite testing results
- Document working endpoints vs. failed advanced components
- Create alternative testing guide for basic OpenRewrite functionality
- Document recipe caching behavior and performance improvements

This focused approach should provide a clear validation of core OpenRewrite capabilities while ensuring sufficient time for recipe downloads and transformations to complete successfully.