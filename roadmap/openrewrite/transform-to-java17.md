# OpenRewrite Java 17 Transformation Test Plan

## Overview

This document provides a practical guide for testing OpenRewrite Java 8→17 transformations through the Ploy ARF system. The focus is on executing single transformations over a list of repositories and verifying the output artifacts.

## System Architecture

The OpenRewrite integration consists of:
1. **API Layer**: ARF handlers that accept transformation requests
2. **Nomad Dispatcher**: Orchestrates containerized transformations
3. **Container Execution**: OpenRewrite JVM container that performs the actual transformation
4. **Storage Layer**: SeaweedFS for artifact storage and caching

## Prerequisites

Before running transformations, ensure:
- Ploy API is running and accessible
- Nomad cluster is operational
- SeaweedFS is available for artifact storage
- Docker registry contains the `openrewrite-jvm` image

## API Endpoints

### Execute Transformation
```
POST /v1/arf/transform
```

### Get Transformation Status
```
GET /v1/arf/transforms/{transformation_id}
```

## Request Format

The transformation request must follow this exact structure:

```json
{
  "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
  "type": "openrewrite",
  "codebase": {
    "repository": "https://github.com/{owner}/{repo}.git",
    "branch": "master",
    "language": "java",
    "build_tool": "maven"
  }
}
```

### Field Descriptions:
- `recipe_id`: The OpenRewrite recipe class name (required)
- `type`: Must be "openrewrite" for OpenRewrite transformations
- `codebase.repository`: Full GitHub URL of the repository
- `codebase.branch`: Target branch (usually "master" or "main")
- `codebase.language`: Must be "java" for Java projects
- `codebase.build_tool`: Either "maven" or "gradle"

## Transformation Workflow

### 1. Submit Transformation Request

```bash
TRANSFORM_ID=$(curl -X POST "${PLOY_CONTROLLER}/arf/transform" \
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
  }' | jq -r '.transformation_id')

echo "Transformation ID: $TRANSFORM_ID"
```

### 2. Monitor Transformation Status

```bash
# Poll for status every 10 seconds
while true; do
  STATUS=$(curl -s "${PLOY_CONTROLLER}/arf/transforms/${TRANSFORM_ID}" | jq -r '.status')
  echo "$(date '+%Y-%m-%d %H:%M:%S') - Status: $STATUS"
  
  if [[ "$STATUS" == "completed" || "$STATUS" == "failed" ]]; then
    break
  fi
  
  sleep 10
done

# Get full result
curl -s "${PLOY_CONTROLLER}/arf/transforms/${TRANSFORM_ID}" | jq '.'
```

## Input and Output Artifacts

### Input Artifact (input.tar)
The system automatically creates an `input.tar` from the cloned repository containing:
- All source code files
- Build configuration (pom.xml, build.gradle)
- Resource files
- Project structure

### Output Artifact (output.tar)
After transformation, the system produces an `output.tar` containing:
- Transformed source code with Java 17 syntax
- Updated build configurations
- All original resources preserved
- Exclude: build artifacts (.m2, target, .gradle, build directories)

### Accessing Output Artifacts

The output tar is stored in SeaweedFS and can be retrieved via:
```bash
# The output location will be in the transformation result
OUTPUT_KEY=$(curl -s "${PLOY_CONTROLLER}/arf/transforms/${TRANSFORM_ID}" | jq -r '.output_key')

# Download from SeaweedFS
curl -O "${SEAWEEDFS_URL}/${OUTPUT_KEY}"
```

## Test Repository List

Execute transformations for each repository individually:

### 1. Simple Java Tutorial (Baseline Test)
```bash
curl -X POST "${PLOY_CONTROLLER}/arf/transform" \
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
  }'
```
**Expected Time**: 5-10 minutes  
**Characteristics**: Small codebase, clear Java 8 patterns

### 2. Spring Boot Sample
```bash
curl -X POST "${PLOY_CONTROLLER}/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/spring-guides/gs-spring-boot.git",
      "branch": "main",
      "language": "java",
      "build_tool": "maven"
    }
  }'
```
**Expected Time**: 5-15 minutes  
**Characteristics**: Spring framework usage, dependency management

### 3. Java Design Patterns
```bash
curl -X POST "${PLOY_CONTROLLER}/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/iluwatar/java-design-patterns.git",
      "branch": "master",
      "language": "java",
      "build_tool": "maven"
    }
  }'
```
**Expected Time**: 20-30 minutes  
**Characteristics**: Large codebase, multiple modules

### 4. RealWorld Example App
```bash
curl -X POST "${PLOY_CONTROLLER}/arf/transform" \
  -H "Content-Type: application/json" \
  -d '{
    "recipe_id": "org.openrewrite.java.migrate.UpgradeToJava17",
    "type": "openrewrite",
    "codebase": {
      "repository": "https://github.com/gothinkster/spring-boot-realworld-example-app.git",
      "branch": "master",
      "language": "java",
      "build_tool": "gradle"
    }
  }'
```
**Expected Time**: 10-20 minutes  
**Characteristics**: Gradle build, real-world application patterns

## Logging and Monitoring

### 1. API Logs
Monitor API logs for request processing and dispatcher activity:
```bash
# On the API server
tail -f /var/log/ploy/api.log

# Or via systemd/docker
journalctl -u ploy-api -f
docker logs ploy-api -f
```

### 2. Nomad Job Logs
Access transformation container logs:
```bash
# Get the Nomad job ID from transformation result
JOB_ID=$(curl -s "${PLOY_CONTROLLER}/arf/transforms/${TRANSFORM_ID}" | jq -r '.job_id')

# View logs
nomad logs -f ${JOB_ID}

# Or get all allocations for the job
nomad job status ${JOB_ID}
nomad alloc logs {allocation_id}
```

### 3. Container Debug Output
The OpenRewrite container produces detailed logs including:
- `[SETUP]` - Workspace preparation
- `[OpenRewrite]` - Transformation progress
- `[Cache]` - Recipe caching operations
- `[Error]` - Failure details

Key log markers to watch for:
```
[OpenRewrite] Extracting input archive...
[OpenRewrite] Running transformation with recipe: org.openrewrite.java.migrate.UpgradeToJava17
[OpenRewrite] Transformation completed successfully
[OpenRewrite] Output tar created successfully
```

### 4. SeaweedFS Storage Logs
Monitor artifact storage operations:
```bash
# Check SeaweedFS filer logs on VPS
curl "${SEAWEEDFS_URL}/status"
```

## Validation Steps

### 1. Extract and Inspect Output
```bash
# Download output tar
curl -o output.tar "${SEAWEEDFS_URL}/${OUTPUT_KEY}"

# Extract and examine
mkdir output-inspection
tar -xf output.tar -C output-inspection
cd output-inspection

# Check for Java 17 features
grep -r "var " --include="*.java" .  # Local variable type inference
grep -r "record " --include="*.java" .  # Record classes
grep -r "sealed " --include="*.java" .  # Sealed classes

# Verify POM updates
grep "<maven.compiler.source>17" pom.xml
grep "<maven.compiler.target>17" pom.xml
```

### 2. Build Verification (Optional)
```bash
cd output-inspection
mvn clean compile  # Should compile with Java 17
```

## Success Criteria

A transformation is considered successful when:

1. **API Response**: Returns `status: "completed"` with a valid transformation_id
2. **Output Artifact**: `output.tar` is created and accessible
3. **Content Validation**: 
   - Source files are present in output.tar
   - Java version updated in build configuration
   - Code compiles with Java 17 (if tested)

## Troubleshooting

### Common Issues and Solutions

#### 1. Transformation Stuck in "pending"
**Cause**: Nomad job not starting  
**Solution**: Check Nomad cluster health
```bash
nomad node status
nomad job status
```

#### 2. "Repository not found" Error
**Cause**: Invalid GitHub URL or private repository  
**Solution**: Verify repository URL is public and accessible

#### 3. Output tar is empty or missing files
**Cause**: Build detection failure  
**Solution**: Check container logs for `[SETUP]` and `[OpenRewrite]` errors

#### 4. Recipe not found
**Cause**: Maven Central connectivity issue  
**Sol ution**: Verify network access from Nomad nodes to Maven Central

#### 5. Transformation times out
**Cause**: Large repository or network issues  
**Solution**: Increase timeout values or retry with smaller repository

### Debug Commands

```bash
# Check API health
curl "${PLOY_CONTROLLER}/health"

# Verify Nomad connectivity
nomad server members
nomad node status

# Test SeaweedFS on VPS
curl "${SEAWEEDFS_URL}/status"
```

## Batch Execution Script

For testing multiple repositories sequentially:

```bash
#!/bin/bash
# batch-transform.sh

REPOS=(
  "winterbe/java8-tutorial:master:maven"
  "spring-guides/gs-spring-boot:main:maven"
  "iluwatar/java-design-patterns:master:maven"
  "gothinkster/spring-boot-realworld-example-app:master:gradle"
)

for repo_info in "${REPOS[@]}"; do
  IFS=':' read -r repo branch build_tool <<< "$repo_info"
  
  echo "Starting transformation for $repo..."
  
  TRANSFORM_ID=$(curl -s -X POST "${PLOY_CONTROLLER}/arf/transform" \
    -H "Content-Type: application/json" \
    -d "{
      \"recipe_id\": \"org.openrewrite.java.migrate.UpgradeToJava17\",
      \"type\": \"openrewrite\",
      \"codebase\": {
        \"repository\": \"https://github.com/${repo}.git\",
        \"branch\": \"${branch}\",
        \"language\": \"java\",
        \"build_tool\": \"${build_tool}\"
      }
    }" | jq -r '.transformation_id')
  
  echo "Transformation ID: $TRANSFORM_ID"
  
  # Wait for completion
  while true; do
    STATUS=$(curl -s "${PLOY_CONTROLLER}/arf/transforms/${TRANSFORM_ID}" | jq -r '.status')
    if [[ "$STATUS" == "completed" || "$STATUS" == "failed" ]]; then
      echo "Transformation $STATUS for $repo"
      break
    fi
    sleep 10
  done
done
```

## Summary

This test plan provides a practical approach to validating OpenRewrite Java 17 transformations through the Ploy ARF system. Focus on:

1. **Single transformations** - Test each repository individually
2. **Monitor progress** - Use status endpoint and logs
3. **Validate output** - Verify output.tar contains transformed code
4. **Track metrics** - Record execution times and success rates

The system handles recipe downloads, caching, and transformation execution automatically through the containerized OpenRewrite engine.