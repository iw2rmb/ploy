#!/bin/bash
# OpenRewrite Runner with SeaweedFS Maven Repository Caching
# This script manages recipe downloads and caching to avoid repeated Maven Central hits

set -ex  # Enable verbose output for debugging

# Arguments from Nomad job
INPUT_TAR="${1:-/workspace/input.tar}"
OUTPUT_TAR="${2:-/workspace/output.tar}"
RECIPE_CLASS="${3:-${RECIPE}}"

# Environment variables (set by Nomad job)
SEAWEEDFS_URL="${SEAWEEDFS_URL:-http://seaweedfs-filer.service.consul:8888}"
MAVEN_CACHE_PATH="${MAVEN_CACHE_PATH:-maven-repository}"
PLOY_API_URL="${PLOY_API_URL:-http://api.service.consul:8081}"
DISCOVER_RECIPE="${DISCOVER_RECIPE:-false}"

echo "[OpenRewrite] Starting transformation"
echo "[OpenRewrite] Recipe: ${RECIPE_CLASS}"
echo "[OpenRewrite] Discovery mode: ${DISCOVER_RECIPE}"
echo "[OpenRewrite] SeaweedFS: ${SEAWEEDFS_URL}"

# Check if we need to discover recipe coordinates
if [ "${DISCOVER_RECIPE}" = "true" ] || [ -z "${RECIPE_GROUP}" ]; then
    echo "[OpenRewrite] Dynamic recipe discovery enabled - OpenRewrite will find coordinates automatically"
    # OpenRewrite will discover the recipe's Maven coordinates from its built-in catalog
    RECIPE_COORDS=""
else
    # Use provided coordinates
    RECIPE_GROUP="${RECIPE_GROUP:-org.openrewrite.recipe}"
    RECIPE_ARTIFACT="${RECIPE_ARTIFACT:-rewrite-migrate-java}"
    RECIPE_VERSION="${RECIPE_VERSION:-2.11.0}"
    RECIPE_COORDS="${RECIPE_GROUP}:${RECIPE_ARTIFACT}:${RECIPE_VERSION}"
    echo "[OpenRewrite] Using provided coordinates: ${RECIPE_COORDS}"
fi

# Function to download from SeaweedFS cache
download_from_cache() {
    local group_path=$(echo "$1" | tr '.' '/')
    local artifact=$2
    local version=$3
    local file_type=$4
    local filename="${artifact}-${version}.${file_type}"
    local cache_path="${MAVEN_CACHE_PATH}/${group_path}/${artifact}/${version}/${filename}"
    local local_path="/workspace/.m2/repository/${group_path}/${artifact}/${version}"
    
    mkdir -p "${local_path}"
    
    # Try to download from SeaweedFS
    if curl -f -s -o "${local_path}/${filename}" "${SEAWEEDFS_URL}/${cache_path}" 2>/dev/null; then
        echo "[Cache] Found ${filename} in SeaweedFS cache"
        return 0
    else
        return 1
    fi
}

# Function to upload to SeaweedFS cache
upload_to_cache() {
    local file=$1
    # Remove the local repository prefix to get relative path
    local relative_path=${file#/workspace/.m2/repository/}
    local cache_path="${MAVEN_CACHE_PATH}/${relative_path}"
    
    # Upload to SeaweedFS (silent, best-effort)
    if curl -X PUT "${SEAWEEDFS_URL}/${cache_path}" \
           --data-binary "@${file}" \
           -H "Content-Type: application/octet-stream" \
           -s -o /dev/null 2>/dev/null; then
        echo "[Cache] Uploaded ${relative_path} to SeaweedFS"
    fi
}

# Step 1: Extract input tar
echo "[OpenRewrite] Extracting input archive..."
echo "[OpenRewrite] Current directory: $(pwd)"
echo "[OpenRewrite] Input tar location: ${INPUT_TAR}"
ls -la "${INPUT_TAR}" || echo "[Error] Input tar not found at ${INPUT_TAR}"

# Clean workspace to avoid mixing files from previous runs
echo "[OpenRewrite] Cleaning workspace..."
rm -rf /workspace/project
mkdir -p /workspace/project

cd /workspace/project
echo "[OpenRewrite] Changed to project directory: $(pwd)"

# Extract tar archive
echo "[OpenRewrite] Extracting tar archive..."
tar -xf "${INPUT_TAR}" || {
    echo "[Error] Failed to extract input tar"
    echo "[Error] Tar exit code: $?"
    ls -la /workspace/
    exit 1
}

echo "[OpenRewrite] Archive extracted successfully"
echo "[OpenRewrite] Current working directory after extraction: $(pwd)"
echo "[OpenRewrite] Project directory contents:"
ls -la
echo "[OpenRewrite] Directory tree (max depth 2):"
find . -maxdepth 2 -type d | sort
echo "[OpenRewrite] Total files extracted: $(find . -type f | wc -l)"

# Step 2: Detect project type and create minimal POM if needed
echo "[OpenRewrite] Checking for build files..."
if [ -f "pom.xml" ]; then
    echo "[OpenRewrite] Found pom.xml"
    head -20 pom.xml
elif [ -f "build.gradle" ]; then
    echo "[OpenRewrite] Found build.gradle"
    head -20 build.gradle
elif [ -f "build.gradle.kts" ]; then
    echo "[OpenRewrite] Found build.gradle.kts"
    head -20 build.gradle.kts
else
    echo "[OpenRewrite] No build file found, creating minimal pom.xml..."
    cat > pom.xml << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 
         http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    
    <groupId>org.example</groupId>
    <artifactId>openrewrite-project</artifactId>
    <version>1.0.0</version>
    
    <properties>
        <maven.compiler.source>11</maven.compiler.source>
        <maven.compiler.target>11</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
</project>
EOF
fi

# Step 3: Handle caching based on discovery mode
echo "[OpenRewrite] Checking cache strategy..."
CACHE_HIT=false

echo "[OpenRewrite] Recipe coordinates check:"
echo "  RECIPE_GROUP: ${RECIPE_GROUP}"
echo "  RECIPE_ARTIFACT: ${RECIPE_ARTIFACT}"
echo "  RECIPE_VERSION: ${RECIPE_VERSION}"
echo "  RECIPE_COORDS: ${RECIPE_COORDS}"
echo "  DISCOVER_RECIPE: ${DISCOVER_RECIPE}"

if [ "${DISCOVER_RECIPE}" = "true" ] || [ -z "${RECIPE_COORDS}" ]; then
    echo "[OpenRewrite] Dynamic discovery mode - OpenRewrite will handle recipe resolution"
    # In discovery mode, OpenRewrite will download recipes as needed
    # We'll cache them after the transformation completes
else
    # Try to get recipe from cache first
    echo "[OpenRewrite] Checking SeaweedFS cache for recipe artifacts..."
    
    if download_from_cache "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "jar" && \
       download_from_cache "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "pom"; then
        CACHE_HIT=true
        echo "[OpenRewrite] Recipe artifacts found in cache"
        
        # Register recipe metadata even for cached recipes (idempotent operation)
        JAR_PATH="${MAVEN_CACHE_PATH}/$(echo ${RECIPE_GROUP} | tr '.' '/')/${RECIPE_ARTIFACT}/${RECIPE_VERSION}/${RECIPE_ARTIFACT}-${RECIPE_VERSION}.jar"
        register_recipe_metadata "${RECIPE_CLASS}" "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "${JAR_PATH}"
    else
        echo "[OpenRewrite] Recipe not in cache, downloading from Maven Central..."
        
        # Mark timestamp before download for tracking new files
        touch /tmp/before_download
        
        # Download recipe and its dependencies
        mvn dependency:get \
            -DgroupId="${RECIPE_GROUP}" \
            -DartifactId="${RECIPE_ARTIFACT}" \
            -Dversion="${RECIPE_VERSION}" \
            -Dtransitive=true \
            -DremoteRepositories=https://repo.maven.apache.org/maven2 \
            || {
                echo "[Error] Failed to download recipe from Maven Central"
                exit 1
            }
        
        # Upload newly downloaded artifacts to SeaweedFS
        echo "[OpenRewrite] Caching downloaded artifacts to SeaweedFS..."
        find /workspace/.m2/repository -type f \( -name "*.jar" -o -name "*.pom" \) \
             -newer /tmp/before_download \
             -exec bash -c 'upload_to_cache() { 
                local file=$1
                local relative_path=${file#/workspace/.m2/repository/}
                curl -X PUT "'${SEAWEEDFS_URL}'/'${MAVEN_CACHE_PATH}'/${relative_path}" \
                     --data-binary "@${file}" \
                     -H "Content-Type: application/octet-stream" \
                     -s -o /dev/null 2>/dev/null && \
                echo "[Cache] Uploaded ${relative_path}"
             }; upload_to_cache "$1"' _ {} \;
        
        # Register the main recipe metadata with Ploy API
        JAR_PATH="${MAVEN_CACHE_PATH}/$(echo ${RECIPE_GROUP} | tr '.' '/')/${RECIPE_ARTIFACT}/${RECIPE_VERSION}/${RECIPE_ARTIFACT}-${RECIPE_VERSION}.jar"
        register_recipe_metadata "${RECIPE_CLASS}" "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "${JAR_PATH}"
    fi
fi

# Step 4: Run OpenRewrite transformation
echo "[OpenRewrite] Running transformation with recipe: ${RECIPE_CLASS}"

# Determine build tool
if [ -f "pom.xml" ]; then
    echo "[OpenRewrite] Using Maven for transformation..."
    
    # Run OpenRewrite
    if [ -n "${RECIPE_COORDS}" ]; then
        # Use explicit coordinates if provided
        echo "[OpenRewrite] Running Maven command with explicit coordinates..."
        echo "[OpenRewrite] Recipe class: ${RECIPE_CLASS}"
        echo "[OpenRewrite] Recipe coordinates: ${RECIPE_COORDS}"
        
        mvn -B org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run \
            -Drewrite.recipe="${RECIPE_CLASS}" \
            -Drewrite.recipeArtifactCoordinates="${RECIPE_COORDS}" \
            -Drewrite.activeRecipes="${RECIPE_CLASS}" \
            -DskipTests \
            -X 2>&1 | tee /tmp/transform.log || {
                echo "[Error] OpenRewrite transformation failed"
                echo "[Error] Exit code: $?"
                echo "[Error] Last 100 lines of output:"
                tail -100 /tmp/transform.log
                exit 1
            }
        
        echo "[OpenRewrite] Transformation completed successfully"
    else
        # Let OpenRewrite discover recipe from its catalog
        echo "[OpenRewrite] Running Maven command for dynamic discovery..."
        echo "[OpenRewrite] Recipe class: ${RECIPE_CLASS}"
        
        # First, try to discover available recipes
        echo "[OpenRewrite] Step 1: Discovering available recipes..."
        mvn -B org.openrewrite.maven:rewrite-maven-plugin:5.34.0:discover 2>&1 | tee /tmp/discover.log || {
            echo "[Error] Recipe discovery failed"
            echo "[Error] Discovery output:"
            cat /tmp/discover.log
        }
        
        # Now run the transformation
        echo "[OpenRewrite] Step 2: Running transformation..."
        mvn -B org.openrewrite.maven:rewrite-maven-plugin:5.34.0:run \
            -Drewrite.recipe="${RECIPE_CLASS}" \
            -Drewrite.activeRecipes="${RECIPE_CLASS}" \
            -DskipTests \
            -X 2>&1 | tee /tmp/transform.log || {
                echo "[Error] OpenRewrite transformation failed"
                echo "[Error] Exit code: $?"
                echo "[Error] Last 100 lines of output:"
                tail -100 /tmp/transform.log
                exit 1
            }
        
        echo "[OpenRewrite] Transformation completed successfully"
    fi
        
elif [ -f "build.gradle" ] || [ -f "build.gradle.kts" ]; then
    echo "[OpenRewrite] Using Gradle for transformation..."
    
    # Add OpenRewrite plugin to build.gradle if not present
    if ! grep -q "id.*org.openrewrite.rewrite" build.gradle* 2>/dev/null; then
        echo "[OpenRewrite] Adding OpenRewrite plugin to build.gradle..."
        if [ -f "build.gradle.kts" ]; then
            cat >> build.gradle.kts << EOF

plugins {
    id("org.openrewrite.rewrite") version "6.16.2"
}

rewrite {
    activeRecipe("${RECIPE_CLASS}")
}

dependencies {
    rewrite("${RECIPE_COORDS}")
}
EOF
        else
            cat >> build.gradle << EOF

plugins {
    id 'org.openrewrite.rewrite' version '6.16.2'
}

rewrite {
    activeRecipe '${RECIPE_CLASS}'
}

dependencies {
    rewrite '${RECIPE_COORDS}'
}
EOF
        fi
    fi
    
    # Run OpenRewrite
    gradle rewriteRun --no-daemon || {
        echo "[Error] OpenRewrite transformation failed"
        exit 1
    }
else
    echo "[Error] No supported build file found (pom.xml or build.gradle)"
    exit 1
fi

# Step 5: Create output tar
echo "[OpenRewrite] Creating output archive..."
echo "[OpenRewrite] Current working directory: $(pwd)"
echo "[OpenRewrite] Directory structure before tar:"
ls -la
echo "[OpenRewrite] Files to archive:"
find . -type f | head -20
echo "[OpenRewrite] Total files to archive: $(find . -type f | wc -l)"

echo "[OpenRewrite] Creating tar archive (excluding Maven cache and workspace files)..."
tar -cf "${OUTPUT_TAR}" --exclude='.m2' --exclude='target' --exclude='.gradle' --exclude='build' . || {
    echo "[Error] Failed to create output tar"
    echo "[Error] Exit code: $?"
    exit 1
}

echo "[OpenRewrite] Output tar created successfully"
ls -la "${OUTPUT_TAR}"

# Step 6: Upload output to SeaweedFS (if OUTPUT_KEY is provided)
if [ -n "${OUTPUT_KEY}" ]; then
    echo "[OpenRewrite] Uploading output to SeaweedFS..."
    echo "[OpenRewrite] Job ID: ${JOB_ID}"
    # Add artifacts/ prefix to OUTPUT_KEY since SeaweedFS base URL doesn't include it
    UPLOAD_URL="${SEAWEEDFS_URL}/artifacts/${OUTPUT_KEY}"
    echo "[OpenRewrite] Upload URL: ${UPLOAD_URL}"
    
    echo "[OpenRewrite] Attempting to upload $(ls -lh ${OUTPUT_TAR} | awk '{print $5}') file to SeaweedFS..."
    echo "[OpenRewrite] Testing network connectivity to SeaweedFS..."
    if ! curl -f -s --connect-timeout 10 "${SEAWEEDFS_URL}/status" >/dev/null; then
        echo "[OpenRewrite] WARNING: Cannot reach SeaweedFS at ${SEAWEEDFS_URL}"
    fi
    
    echo "[OpenRewrite] Uploading with verbose output..."
    UPLOAD_RESPONSE=$(curl -X PUT "${UPLOAD_URL}" \
           --data-binary "@${OUTPUT_TAR}" \
           -H "Content-Type: application/octet-stream" \
           --connect-timeout 30 \
           --max-time 300 \
           -w "HTTP_CODE:%{http_code} SIZE_UPLOAD:%{size_upload} TIME_TOTAL:%{time_total}" \
           2>&1)
    UPLOAD_EXIT_CODE=$?
    
    echo "[OpenRewrite] Upload response: ${UPLOAD_RESPONSE}"
    echo "[OpenRewrite] Upload exit code: ${UPLOAD_EXIT_CODE}"
    
    if [ $UPLOAD_EXIT_CODE -eq 0 ] && echo "$UPLOAD_RESPONSE" | grep -q "HTTP_CODE:2[0-9][0-9]"; then
        echo "[OpenRewrite] Output uploaded successfully to artifacts/${OUTPUT_KEY}"
    else
        echo "[OpenRewrite] ERROR: Failed to upload output to SeaweedFS"
        echo "[OpenRewrite] Exit code: ${UPLOAD_EXIT_CODE}"
        echo "[OpenRewrite] Response: ${UPLOAD_RESPONSE}"
        # Continue anyway - transformation was successful
    fi
else
    echo "[OpenRewrite] No OUTPUT_KEY provided, skipping upload"
fi

# Step 7: Generate transformation report
cat > /workspace/transformation-report.json << EOF
{
  "recipe": "${RECIPE_CLASS}",
  "coordinates": "${RECIPE_COORDS}",
  "cache_hit": ${CACHE_HIT},
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "success": true
}
EOF

echo "[OpenRewrite] Transformation completed successfully"
echo "[OpenRewrite] Output: ${OUTPUT_TAR}"
echo "[OpenRewrite] Cache status: $([ "$CACHE_HIT" = true ] && echo "HIT" || echo "MISS")"

# Function to register recipe metadata with Ploy API
register_recipe_metadata() {
    local recipe_class=$1
    local group=$2
    local artifact=$3
    local version=$4
    local jar_path=$5
    
    # Only register if we have the API endpoint configured
    if [ -n "${PLOY_API_URL}" ]; then
        echo "[Recipe] Registering recipe metadata with Ploy API..."
        
        # Create JSON payload for recipe registration
        cat > /tmp/recipe-registration.json << EOJSON
{
  "recipe_class": "${recipe_class}",
  "maven_coords": "${group}:${artifact}:${version}",
  "jar_path": "${jar_path}",
  "source": "openrewrite-jvm",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOJSON
        
        # Register with Ploy API
        curl -X POST "${PLOY_API_URL}/v1/arf/recipes/register" \
             -H "Content-Type: application/json" \
             -d @/tmp/recipe-registration.json \
             -s -o /dev/null 2>/dev/null && \
        echo "[Recipe] Recipe metadata registered successfully" || \
        echo "[Recipe] Failed to register recipe metadata (non-critical)"
    fi
}

# Export the upload function for use in exec calls
export -f upload_to_cache