# OpenRewrite Java 17 Transformation Test Plan

## Alternative Approach: Direct OpenRewrite Endpoints

**Strategy**: Bypass the advanced ARF components (learning_system, llm_generator, hybrid_pipeline, catalog) and use dedicated OpenRewrite endpoints that work with available components (recipe_executor: true, sandbox_mgr: true).

**Key Assumptions**:
1. **Recipe Auto-Download**: If recipe is not available locally, the openrewrite-jvm image will automatically download and store it during transformation
2. **Extended Timeouts**: All timeouts are set to generous values to ensure processes have sufficient time to complete correctly

## Phase 1: Direct OpenRewrite Transformation Testing

### Step 1: Execute Direct OpenRewrite Transformation
1. **Transform Request**: Use `/v1/arf/openrewrite/transform` endpoint directly
   - **Timeout**: 30 minutes (generous time for repository cloning, recipe download, and transformation)
   - **Extended Processing Time**: Allow for recipe download if not cached
2. **Target Repository**: https://github.com/winterbe/java8-tutorial.git
3. **Recipe**: `org.openrewrite.java.migrate.Java11toJava17` (or appropriate Java 8→17 recipe)
4. **Request Format**:
   ```json
   {
     "project_url": "https://github.com/winterbe/java8-tutorial.git",
     "recipes": ["org.openrewrite.java.migrate.Java11toJava17"],
     "package_manager": "maven",
     "base_jdk": "17",
     "branch": "master"
   }
   ```

### Step 2: Monitor Job Execution with Extended Timeouts
1. **Job Submission**: Receive job_id from transform request
   - **Initial Response Timeout**: 2 minutes
2. **Status Monitoring**: Poll `/v1/arf/openrewrite/status/{job_id}` endpoint
   - **Polling Interval**: 30 seconds
   - **Maximum Wait Time**: 45 minutes total (allows for recipe download + transformation)
   - **Status Check Timeout**: 10 seconds per poll
3. **Progress Tracking**: Monitor transformation job until completion
   - **Expected Phases**: Repository clone → Recipe download (if needed) → Transformation → Result packaging
4. **Result Retrieval**: Get transformation output/diff when available

### Step 3: Post-Transformation Recipe Verification
1. **Recipe Storage Check**: After successful transformation, verify that recipes are now cached
   - **Endpoint**: `/v1/arf/openrewrite/recipes` (should now show downloaded recipes)
   - **Expected**: Recipe cache populated with `org.openrewrite.java.migrate.Java11toJava17`
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
- **Recipe List/Validate**: 60 seconds (allow recipe download)
- **Transform Submission**: 2 minutes (job creation and validation)
- **Status Polling**: 10 seconds per request
- **Total Job Wait**: 60 minutes maximum per transformation

### Job Execution Timeouts:
- **Repository Clone**: 5 minutes
- **Recipe Download**: 15 minutes (Maven Central + artifact resolution)
- **Transformation Execution**: 30 minutes (large codebases)
- **Result Packaging**: 5 minutes
- **Total Job Timeout**: 60 minutes per transformation job

### Retry and Circuit Breaker Configuration:
- **Status Poll Retries**: 3 attempts with 5-second backoff
- **Network Timeout**: 30 seconds for external repository access
- **Docker Image Pull**: 10 minutes (openrewrite-jvm image + dependencies)

## Expected Outcomes

### Success Criteria:
- ✅ Recipe listing works (empty initially, populated after first use)
- ✅ Recipe validation succeeds for Java migration recipes  
- ✅ Transform job submission returns job_id within 2 minutes
- ✅ Job status tracking provides progress updates every 30 seconds
- ✅ At least one repository transformation completes successfully within 60 minutes
- ✅ **Recipe Caching**: Recipes are downloaded and stored after first transformation
- ✅ **Performance Improvement**: Subsequent transformations are 50%+ faster with cached recipes

### Performance Targets (with generous timeouts):
- **First Transformation** (with recipe download):
  - Endpoint Response Time: <2 minutes for job submission
  - Job Completion: <60 minutes for simple repositories
  - Success Rate: >80% for Tier 1 (simple) repositories
- **Subsequent Transformations** (with cached recipes):
  - Endpoint Response Time: <30 seconds for job submission
  - Job Completion: <30 minutes for simple repositories
  - Success Rate: >95% for Tier 1 repositories

## Recipe Management Validation

### Pre-Transformation State:
- **Recipe Catalog**: May be empty or minimal
- **Expected Behavior**: OpenRewrite will download required recipes on first use

### During Transformation:
- **Recipe Download Phase**: Monitor job logs for Maven artifact downloads
- **Expected Downloads**: 
  - `org.openrewrite.recipe:rewrite-migrate-java:latest`
  - Dependencies and transitive dependencies
  - Recipe metadata and configuration

### Post-Transformation State:
- **Recipe Catalog**: Should now contain downloaded recipes
- **Verification Method**: Call `/v1/arf/openrewrite/recipes` to confirm cache population
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

Based on `/api/README.md`, the following OpenRewrite endpoints are available:

### Dedicated OpenRewrite Endpoints (Recommended)
- `POST /v1/arf/openrewrite/validate` — validate recipes  
- `GET /v1/arf/openrewrite/recipes` — list available recipes
- `POST /v1/arf/openrewrite/transform` — execute transformation
- `GET /v1/arf/openrewrite/status/:jobId` — get transformation job status

### General ARF Endpoints (Alternative)
- `GET /v1/arf/recipes` — list available transformation recipes (more general)
- `POST /v1/arf/transform` — execute code transformation (more general)

**Note**: Using dedicated OpenRewrite endpoints (`/v1/arf/openrewrite/*`) provides better compatibility with available components since they bypass advanced ARF features that may be unavailable.

## Test Execution Commands

### Step 1: Execute Transformation with Extended Timeout
```bash
# Using dedicated OpenRewrite endpoint (recommended)
curl -X POST "${PLOY_CONTROLLER%/v1}/v1/arf/openrewrite/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "project_url": "https://github.com/winterbe/java8-tutorial.git",
    "recipes": ["org.openrewrite.java.migrate.Java8toJava11"],
    "package_manager": "maven",
    "base_jdk": "11",
    "branch": "master"
  }' \
  --max-time 120  # 2 minute timeout for job submission
```

### Step 2: Monitor Job Status (with 60-minute maximum wait)
```bash
JOB_ID="<job_id_from_step_1>"
timeout 3600 bash -c '
  while true; do
    STATUS=$(curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/openrewrite/status/'$JOB_ID'" | jq -r ".status")
    echo "$(date): Job status: $STATUS"
    if [[ "$STATUS" == "completed" || "$STATUS" == "failed" ]]; then
      break
    fi
    sleep 30
  done
'
```

### Step 3: Verify Recipe Caching
```bash
# Check if recipes are now cached using dedicated endpoint
curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/openrewrite/recipes" | jq '.recipes | length'
# Should show recipes downloaded during transformation

# Alternative: Check general ARF recipe catalog
curl -s "${PLOY_CONTROLLER%/v1}/v1/arf/recipes" | jq '.recipes | length'
```

### Recipe Validation (Optional)
```bash
# Validate recipe before transformation
curl -X POST "${PLOY_CONTROLLER%/v1}/v1/arf/openrewrite/validate" \
  -H "Content-Type: application/json" \
  -d '{
    "recipes": ["org.openrewrite.java.migrate.Java8toJava11"]
  }'
```

## Documentation Updates:
- Update main roadmap with direct OpenRewrite testing results
- Document working endpoints vs. failed advanced components
- Create alternative testing guide for basic OpenRewrite functionality
- Document recipe caching behavior and performance improvements

This focused approach should provide a clear validation of core OpenRewrite capabilities while ensuring sufficient time for recipe downloads and transformations to complete successfully.