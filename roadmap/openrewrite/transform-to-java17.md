# OpenRewrite Java 17 Transformation Test Plan

## Unified ARF System with Async Execution

**Strategy**: Use the unified ARF (Automated Remediation Framework) system with asynchronous transformation execution and Consul KV persistence.

**Implementation Status**:
✅ **Phase 1 Completed**: Async transformation with Consul KV storage
✅ **Transform Route Enhancement**: Returns immediate status link instead of waiting
✅ **Background Execution**: Goroutine-based transformation execution
✅ **Persistent Storage**: Consul KV replaces in-memory storage

**Key Features**:
1. **Asynchronous Execution**: Transformations run in background, return status URL immediately
2. **Consul KV Persistence**: Transformation status survives API restarts
3. **Recipe Auto-Download**: OpenRewrite image automatically downloads and stores recipes during transformation
4. **Nested Healing Support**: Full healing workflow with recursive child transformations (Phase 2 ready)

## Phase 1: Async OpenRewrite Transformation Testing

### Step 1: Execute ARF Transformation with OpenRewrite (Async)
1. **Transform Request**: Use `/v1/arf/transform` endpoint with OpenRewrite recipe
   - **Response Time**: <1 second (returns status URL immediately)
   - **Background Processing**: Transformation runs asynchronously
   - **Consul Storage**: Status persisted to Consul KV immediately
2. **Target Repository**: https://github.com/winterbe/java8-tutorial.git
3. **Recipe**: `org.openrewrite.java.migrate.UpgradeToJava17` (unified Java 8→17 recipe)
4. **Request Format**:
   ```json
   {
     "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
     "type": "openrewrite",
     "codebase": {
       "repository": "https://github.com/winterbe/java8-tutorial.git",
       "branch": "master",
       "language": "java",
       "build_tool": "maven"
     }
   }
   ```
5. **Expected Response** (immediate):
   ```json
   {
     "transformation_id": "uuid-1234-5678",
     "status": "initiated",
     "status_url": "/v1/arf/transforms/uuid-1234-5678/status",
     "message": "Transformation started, use status_url to monitor progress"
   }
   ```

### Step 2: Monitor Transformation Execution
1. **Immediate Status URL**: Use the `status_url` from initial response
2. **Status Monitoring**: Poll `/v1/arf/transforms/{transformation_id}/status` endpoint
   - **Polling Interval**: 30 seconds
   - **Maximum Wait Time**: 30 minutes total (allows for recipe download + transformation)
   - **Status Check Timeout**: 10 seconds per poll
3. **Consul KV Status**: Transformation status persisted in:
   - Key: `ploy/arf/transforms/{transform_id}/status`
   - Updates in real-time as transformation progresses
4. **Progress Tracking**: Monitor workflow stages:
   - `initiated` → `openrewrite` → `build` → `test` → `heal` (if needed) → `completed`
5. **Enhanced Status Response**:
   ```json
   {
     "transformation_id": "uuid-1234-5678",
     "workflow_stage": "openrewrite",
     "status": "in_progress",
     "start_time": "2025-01-15T10:00:00Z",
     "children": [],  // Populated if healing workflow triggered
     "active_healing_count": 0,
     "total_healing_attempts": 0
   }
   ```

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

## Timeout Configuration Strategy (Async Model)

### API Endpoint Timeouts:
- **Transform Submission**: <1 second (immediate response with status URL)
- **Status Polling**: 10 seconds per request
- **Recipe List/Validate**: 60 seconds (recipe resolution)
- **Background Job Execution**: No HTTP timeout (runs independently)

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

## Container Implementation

### OpenRewrite Docker Container (`services/openrewrite-jvm/`)

The OpenRewrite transformations run in a custom Docker container with two-stage execution:

1. **Setup Stage** (`setup-workspace.sh` - entrypoint):
   - Detects Nomad artifact locations (`local/`, `artifacts/`, or current directory)
   - Handles both tar file artifacts and extracted directory structures
   - Creates `/workspace/input.tar` for the runner script
   - Manages workspace permissions and file structure

2. **Execution Stage** (`runner.sh`):
   - Extracts input tar to `/workspace/project/`
   - Detects build system (Maven/Gradle) or creates minimal POM
   - Handles recipe discovery and Maven Central caching
   - Executes OpenRewrite transformation
   - Creates output tar with transformed code
   - Registers recipe metadata with Ploy API

## Expected Outcomes

### Success Criteria:
- ✅ **Async Execution**: Transform endpoint returns status URL within <1 second
- ✅ **Consul Persistence**: Transformation status survives API restarts
- ✅ **Background Processing**: Transformations run independently of HTTP connections
- ✅ Recipe listing works via `/v1/arf/recipes?type=openrewrite`
- ✅ Recipe validation succeeds for Java migration recipes via `/v1/arf/recipes/validate`
- ✅ Transformation status tracking provides progress updates from Consul KV
- ✅ At least one repository transformation completes successfully within 60 minutes
- ✅ **Recipe Caching**: Recipes are downloaded and stored after first transformation
- ✅ **Performance Improvement**: Subsequent transformations are 50%+ faster with cached recipes
- ✅ **Healing Ready**: Data structures support nested healing workflows (Phase 2)

### Performance Targets (Async Model):
- **All Transformations**:
  - Endpoint Response Time: <1 second (returns status URL immediately)
  - Consul Status Update: Within 100ms of state changes
  - HTTP Connection Duration: <1 second per request
- **First Transformation** (with recipe download):
  - Background Job Completion: <30 minutes for simple repositories
  - Success Rate: >80% for Tier 1 (simple) repositories
- **Subsequent Transformations** (with cached recipes):
  - Background Job Completion: <15 minutes for simple repositories
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

## Advantages of Async Implementation:
1. **No Long-lived Connections**: HTTP connections released immediately
2. **Persistent State**: Transformation status survives API restarts via Consul KV
3. **Scalability**: Can handle many concurrent transformations without connection limits
4. **Background Processing**: Transformations continue even if client disconnects
5. **Healing Workflow Ready**: Data structures support nested healing transformations
6. **Recipe Auto-Management**: Validates automatic recipe download and caching
7. **Real-time Status**: Consul KV provides instant status updates
8. **Production Ready**: Async pattern suitable for production deployments

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

## API Endpoint Reference (Async Implementation)

Based on Phase 1 implementation, OpenRewrite uses async ARF system:

### Unified ARF System for OpenRewrite (Async)
- `POST /v1/arf/transform` — initiate async transformation, returns status URL immediately
- `GET /v1/arf/transforms/:id/status` — get transformation status from Consul KV
- `GET /v1/arf/transforms/:id` — (deprecated) legacy endpoint for compatibility

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

### Step 1: Execute Async Transformation via Unified ARF
```bash
# Initiate async transformation (returns immediately)
RESPONSE=$(curl -X POST "${PLOY_CONTROLLER%/v1}/v1/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/winterbe/java8-tutorial.git",
      "branch": "master",
      "language": "java",
      "build_tool": "maven"
    }
  }')

# Extract transformation_id and status_url from immediate response
TRANSFORM_ID=$(echo "$RESPONSE" | jq -r '.transformation_id')
STATUS_URL=$(echo "$RESPONSE" | jq -r '.status_url')
echo "Transformation initiated: $TRANSFORM_ID"
echo "Monitor at: $STATUS_URL"
```

### Step 2: Monitor Transformation Status (Async)
```bash
# Poll status endpoint (data from Consul KV)
timeout 3600 bash -c '
  while true; do
    STATUS_RESPONSE=$(curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/transforms/'$TRANSFORM_ID'/status")
    STATUS=$(echo "$STATUS_RESPONSE" | jq -r ".status")
    STAGE=$(echo "$STATUS_RESPONSE" | jq -r ".workflow_stage")
    echo "$(date): Stage: $STAGE, Status: $STATUS"
    
    # Check for healing workflows
    HEALING_COUNT=$(echo "$STATUS_RESPONSE" | jq -r ".active_healing_count // 0")
    if [ "$HEALING_COUNT" -gt 0 ]; then
      echo "  Active healing attempts: $HEALING_COUNT"
    fi
    
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
    "type": "openrewrite"
  }'
```

## Implementation Phases Completed:

### ✅ Phase 1: Async Transformation & Consul KV (COMPLETED)
- Transform route returns status URL immediately (<1 second)
- Background goroutine executes transformation
- Consul KV stores transformation status persistently
- Breaking change: Clients must use async pattern

### ✅ Phase 2: Enhanced Data Structures (COMPLETED)
- Nested `HealingAttempt` structure implemented
- `HealingTree` management logic ready
- `TransformationResult` extended with healing fields
- Attempt path generation and management complete

### 🚧 Phase 3: Healing Workflow Logic (IN PROGRESS)
- Recursive healing workflow execution implemented
- LLM error analysis integration pending
- Build/test validation in sandbox pending

## Consul KV Structure:
```
ploy/arf/transforms/{id}/status     # Main transformation status
ploy/arf/transforms/{id}/children   # Healing attempts hierarchy
ploy/arf/transforms/{id}/sandbox    # Sandbox deployment info
```

## Breaking Changes:
- `/v1/arf/transform` now returns status URL, not full result
- Clients must poll `/v1/arf/transforms/{id}/status` for results
- No backward compatibility with synchronous pattern

This async approach with Consul persistence provides production-ready OpenRewrite transformation capabilities with support for complex healing workflows.