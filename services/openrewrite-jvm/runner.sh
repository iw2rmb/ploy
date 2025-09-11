#!/bin/bash
# OpenRewrite Runner with SeaweedFS Maven Repository Caching
# This script manages recipe downloads and caching to avoid repeated Maven Central hits

set -eo pipefail
set -x  # Enable verbose output for debugging

# --- Event push helpers (best-effort) ---
post_event() {
  local level="$1"; shift
  local phase="$1"; shift
  local step="$1"; shift
  local msg="$1"
  if [ -n "${CONTROLLER_URL}" ] && [ -n "${TRANSFLOW_EXECUTION_ID}" ]; then
    # Avoid failing the run due to telemetry issues
    curl -sS -X POST "${CONTROLLER_URL}/transflow/event" \
      -H "Content-Type: application/json" \
      -d "{\"execution_id\":\"${TRANSFLOW_EXECUTION_ID}\",\"phase\":\"${phase}\",\"step\":\"${step}\",\"level\":\"${level}\",\"message\":\"${msg}\"}" \
      -o /dev/null || true
  fi
}

on_exit() {
  code=$?
  if [ "$code" -eq 0 ]; then
    post_event "info" "apply" "orw-apply" "Completed orw-apply"
  else
    post_event "error" "apply" "orw-apply" "Failed orw-apply"
  fi
}

trap on_exit EXIT
post_event "info" "apply" "orw-apply" "Started orw-apply"

# Helper: write an error message to an artifact that the controller can collect
write_error() {
  local msg="$*"
  local dir="${OUTPUT_DIR:-/workspace/out}"
  mkdir -p "$dir" 2>/dev/null || true
  echo "$msg" > "$dir/error.log" 2>/dev/null || true
}

# Define recipe metadata registration early so calls are safe
register_recipe_metadata() {
    local recipe_class=$1
    local group=$2
    local artifact=$3
    local version=$4
    local jar_path=$5

    if [ -n "${PLOY_API_URL}" ]; then
        echo "[Recipe] Registering recipe metadata with Ploy API..."
        cat > /tmp/recipe-registration.json << EOJSON
{
  "recipe_class": "${recipe_class}",
  "maven_coords": "${group}:${artifact}:${version}",
  "jar_path": "${jar_path}",
  "source": "openrewrite-jvm",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOJSON
        curl -sS -X POST "${PLOY_API_URL}/v1/arf/recipes/register" \
             -H "Content-Type: application/json" \
             -d @/tmp/recipe-registration.json \
             -o /dev/null || true
    fi
}

# Arguments from Nomad job (SeaweedFS-only IO: no OUTPUT_TAR)
INPUT_TAR="${1:-/workspace/input.tar}"
RECIPE_CLASS="${2:-${RECIPE}}"

# Environment variables (set by Nomad job)
SEAWEEDFS_URL="${SEAWEEDFS_URL:-http://seaweedfs-filer.service.consul:8888}"
MAVEN_CACHE_PATH="${MAVEN_CACHE_PATH:-maven-repository}"
PLOY_API_URL="${PLOY_API_URL:-http://api.service.consul:8081}"

echo "[OpenRewrite] Starting transformation"
echo "[OpenRewrite] Environment variables:"
echo "[OpenRewrite]   JOB_ID: ${JOB_ID:-}"
echo "[OpenRewrite]   TRANSFORMATION_ID: ${TRANSFORMATION_ID:-}"
echo "[OpenRewrite]   RECIPE: ${RECIPE_CLASS}"
echo "[OpenRewrite]   SEAWEEDFS_URL: ${SEAWEEDFS_URL}"
echo "[OpenRewrite]   INPUT_URL: ${INPUT_URL:-}"
echo "[OpenRewrite] SeaweedFS: ${SEAWEEDFS_URL}"

# Sanity check Java toolchain
echo "[OpenRewrite] Java toolchain diagnostics:"
if command -v java >/dev/null 2>&1; then
  java -version || true
else
  echo "[OpenRewrite] ERROR: 'java' not found in PATH" >&2
  write_error "java not found in PATH"
  fi
if command -v javac >/dev/null 2>&1; then
  javac -version || true
else
  echo "[OpenRewrite] ERROR: 'javac' (Java compiler) not found. A JDK is required, not a JRE." >&2
  echo "[OpenRewrite] Please ensure JAVA_HOME points to a JDK and PATH includes \"$JAVA_HOME/bin\"." >&2
  write_error "javac not found; JDK required"
  exit 1
fi
if command -v mvn >/dev/null 2>&1; then
  mvn -version || true
else
  echo "[OpenRewrite] ERROR: 'mvn' (Maven) not found in PATH" >&2
  write_error "mvn (Maven) not found in PATH"
  exit 1
fi

# Require explicit coordinates from env (no discovery)
RECIPE_GROUP="${RECIPE_GROUP:-}"
RECIPE_ARTIFACT="${RECIPE_ARTIFACT:-}"
RECIPE_VERSION="${RECIPE_VERSION:-}"
if [ -z "$RECIPE_GROUP" ] || [ -z "$RECIPE_ARTIFACT" ] || [ -z "$RECIPE_VERSION" ]; then
  echo "[OpenRewrite] ERROR: Missing recipe coordinates (RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_VERSION)" >&2
  write_error "Missing recipe coordinates"
  exit 1
fi
RECIPE_COORDS="${RECIPE_GROUP}:${RECIPE_ARTIFACT}:${RECIPE_VERSION}"
echo "[OpenRewrite] Using coordinates: ${RECIPE_COORDS}"

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

# Prepare output directory for artifacts
OUTPUT_DIR="${OUTPUT_DIR:-/workspace/out}"
mkdir -p "${OUTPUT_DIR}"

# Step 1: Extract input tar
echo "[OpenRewrite] Extracting input archive..."
echo "[OpenRewrite] Current directory: $(pwd)"
echo "[OpenRewrite] Input tar location: ${INPUT_TAR}"

# If INPUT_URL is provided and the tar is missing, try to download it (best-effort)
if [ ! -s "${INPUT_TAR}" ] && [ -n "${INPUT_URL:-}" ]; then
  echo "[OpenRewrite] INPUT_TAR not found; attempting download from INPUT_URL=${INPUT_URL}"
  set +e
  RESP=$(curl -sSL --connect-timeout 30 --max-time 300 -w "HTTP_CODE:%{http_code}" -o "${INPUT_TAR}" "${INPUT_URL}")
  CURL_RC=$?
  set -e
  echo "[OpenRewrite] INPUT_URL download result: rc=${CURL_RC} ${RESP}"
  if [ $CURL_RC -ne 0 ] || ! echo "$RESP" | grep -q "HTTP_CODE:200"; then
    echo "[OpenRewrite] WARNING: failed to download INPUT_TAR from ${INPUT_URL}"
  else
    echo "[OpenRewrite] Downloaded INPUT_TAR from ${INPUT_URL}"
    ls -lh "${INPUT_TAR}" || true
  fi
fi

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
    write_error "Failed to extract input.tar"
    exit 1
}

echo "[OpenRewrite] Archive extracted successfully"
echo "[OpenRewrite] Current working directory after extraction: $(pwd)"
echo "[OpenRewrite] Project directory contents:"
ls -la
echo "[OpenRewrite] Directory tree (max depth 2):"
find . -maxdepth 2 -type d | sort
echo "[OpenRewrite] Total files extracted: $(find . -type f | wc -l)"

# Snapshot original tree for diff generation later
ORIG_SNAPSHOT="/workspace/original"
rm -rf "$ORIG_SNAPSHOT"
mkdir -p "$ORIG_SNAPSHOT"
cp -a . "$ORIG_SNAPSHOT/"

# Step 2: Detect project type - build file is REQUIRED
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
  echo "[OpenRewrite] ERROR: No build file found (pom.xml, build.gradle, or build.gradle.kts)"
  echo "[OpenRewrite] OpenRewrite requires a valid build configuration to run transformations"
  echo "[OpenRewrite] Please ensure your project has one of the following:"
  echo "[OpenRewrite]   - pom.xml for Maven projects"
  echo "[OpenRewrite]   - build.gradle or build.gradle.kts for Gradle projects"
  write_error "No build file found in project (pom.xml/build.gradle/build.gradle.kts)"
  exit 1
fi

# Step 3: Handle caching based on discovery mode
echo "[OpenRewrite] Checking cache strategy..."
CACHE_HIT=false

echo "[OpenRewrite] Recipe coordinates check:"
echo "  RECIPE_GROUP: ${RECIPE_GROUP}"
echo "  RECIPE_ARTIFACT: ${RECIPE_ARTIFACT}"
echo "  RECIPE_VERSION: ${RECIPE_VERSION}"
echo "  RECIPE_COORDS: ${RECIPE_COORDS}"

if [ -n "${RECIPE_COORDS}" ]; then
    # Try to get recipe from cache first
    echo "[OpenRewrite] Checking SeaweedFS cache for recipe artifacts..."
    
    if download_from_cache "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "jar" && \
       download_from_cache "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "pom"; then
        CACHE_HIT=true
        echo "[OpenRewrite] Recipe artifacts found in cache"
        
        # Register recipe metadata even for cached recipes (idempotent operation)
        JAR_PATH="${MAVEN_CACHE_PATH}/$(echo ${RECIPE_GROUP} | tr '.' '/')/${RECIPE_ARTIFACT}/${RECIPE_VERSION}/${RECIPE_ARTIFACT}-${RECIPE_VERSION}.jar"
        register_recipe_metadata "${RECIPE_CLASS}" "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "${JAR_PATH}" || true
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
        register_recipe_metadata "${RECIPE_CLASS}" "${RECIPE_GROUP}" "${RECIPE_ARTIFACT}" "${RECIPE_VERSION}" "${JAR_PATH}" || true
    fi
fi

# Step 4: Run OpenRewrite transformation
echo "[OpenRewrite] Running transformation with recipe: ${RECIPE_CLASS}"

# Determine build tool: Maven only
if [ -f "pom.xml" ]; then
    echo "[OpenRewrite] Using Maven for transformation..."

    # Allow overriding plugin version from env; default kept for backward-compat
    MAVEN_PLUGIN_VERSION_ENV="${MAVEN_PLUGIN_VERSION:-6.18.0}"

    if [ -n "${RECIPE_COORDS}" ]; then
        # Use explicit coordinates if provided
        echo "[OpenRewrite] Running Maven command with explicit coordinates..."
        echo "[OpenRewrite] Recipe class: ${RECIPE_CLASS}"
        echo "[OpenRewrite] Recipe coordinates: ${RECIPE_COORDS}"

        # Use configured plugin version
        mvn -B org.openrewrite.maven:rewrite-maven-plugin:${MAVEN_PLUGIN_VERSION_ENV}:run \
            -Drewrite.recipe="${RECIPE_CLASS}" \
            -Drewrite.recipeArtifactCoordinates="${RECIPE_COORDS}" \
            -Drewrite.activeRecipes="${RECIPE_CLASS}" \
            -DskipTests \
            -X 2>&1 | tee /tmp/transform.log
        status=${PIPESTATUS[0]}
        if [ "$status" -ne 0 ]; then
          echo "[Error] OpenRewrite transformation failed (plugin 6.18.0)"
          echo "[Error] Last 100 lines of output:"
          tail -100 /tmp/transform.log
          write_error "OpenRewrite transformation failed (see transform.log)"
          cp -f /tmp/transform.log "${OUTPUT_DIR}/transform.log" 2>/dev/null || true
          exit 1
        fi
        # Persist transform.log for diagnostics on success as well
        cp -f /tmp/transform.log "${OUTPUT_DIR}/transform.log" 2>/dev/null || true
        
        echo "[OpenRewrite] Transformation completed successfully"
else
    echo "[Error] No supported build file found (pom.xml)"
    write_error "No supported build file found (pom.xml)"
    exit 1
fi

# Step 5: Create output tar
echo "[OpenRewrite] Creating output archive..."
echo "[OpenRewrite] Current working directory: $(pwd)"
echo "[OpenRewrite] Directory structure before tar:"
ls -la
echo "[OpenRewrite] All directories in workspace:"
find . -type d | sort
echo "[OpenRewrite] Files to archive:"
{ find . -type f | head -20; } || true
echo "[OpenRewrite] Total files to archive: $(find . -type f | wc -l)"
echo "[OpenRewrite] Checking for project directory:"
if [ -d "project" ]; then
    echo "[OpenRewrite] WARNING: project/ directory exists!"
    echo "[OpenRewrite] Contents of project/:"
    ls -la project/ || echo "Empty"
else
    echo "[OpenRewrite] No project/ directory found (good)"
fi

# Debug: Show exactly where we are and what files exist before tar creation
echo "[OpenRewrite] Pre-tar debugging:"
echo "[OpenRewrite] Current directory: $(pwd)"
echo "[OpenRewrite] Contents of current directory:"
{ ls -la . | head -10; } || true
echo "[OpenRewrite] Contents of /workspace:"
{ ls -la /workspace | head -10; } || true

# Step 5: Generate diff.patch artifact for transflow using git diff (preferred)
echo "[OpenRewrite] Generating unified diff patch..."
rm -f "${OUTPUT_DIR}/diff.patch" || true
if [ -d .git ]; then
  # Use git diff against HEAD to capture working tree changes (no color, unified)
  set +e
  git diff -U3 --no-color > "${OUTPUT_DIR}/diff.patch"
  DIFF_RC=$?
  set -e
  # git diff returns 1 when differences are present; treat 0 or 1 as success
  if [ "$DIFF_RC" -ne 0 ] && [ "$DIFF_RC" -ne 1 ]; then
    echo "[OpenRewrite] WARNING: git diff returned rc=$DIFF_RC; falling back to diff -ruN"
    diff -ruN "/workspace/original" "/workspace/project" > "${OUTPUT_DIR}/diff.patch" 2>/dev/null || true
  fi
else
  echo "[OpenRewrite] .git not found; using diff -ruN fallback"
  diff -ruN "/workspace/original" "/workspace/project" > "${OUTPUT_DIR}/diff.patch" 2>/dev/null || true
fi

# Log diff size and preview
if [ -f "${OUTPUT_DIR}/diff.patch" ]; then
  echo "[OpenRewrite] diff size: $(wc -c < \"${OUTPUT_DIR}/diff.patch\") bytes"
  echo "[OpenRewrite] diff head preview:"; head -n 20 "${OUTPUT_DIR}/diff.patch" || true
else
  echo "[OpenRewrite] WARNING: diff.patch was not created"
  touch "${OUTPUT_DIR}/diff.patch" || true
fi

# Step 8: Generate transformation report
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
echo "[OpenRewrite] Cache status: $([ "$CACHE_HIT" = true ] && echo "HIT" || echo "MISS")"

# Optional upload of diff.patch to SeaweedFS if DIFF_URL or DIFF_KEY provided (best-effort)
if [ -n "${DIFF_URL}" ] || { [ -n "${SEAWEEDFS_URL}" ] && [ -n "${DIFF_KEY}" ]; }; then
  TARGET_URL="${DIFF_URL}"
  if [ -z "$TARGET_URL" ]; then
    # Compose URL; avoid double /artifacts/
    KEY_PATH="${DIFF_KEY}"
    case "$KEY_PATH" in
      artifacts/*) TARGET_URL="${SEAWEEDFS_URL%/}/${KEY_PATH}" ;;
      *)           TARGET_URL="${SEAWEEDFS_URL%/}/artifacts/${KEY_PATH}" ;;
    esac
  fi
  echo "[OpenRewrite] Uploading diff.patch to $TARGET_URL"
  set +e
  UPLOAD_RESPONSE=$(curl -X PUT "$TARGET_URL" \
         --data-binary "@${OUTPUT_DIR}/diff.patch" \
         -H "Content-Type: text/plain" \
         --connect-timeout 30 \
         --max-time 300 \
         -w "HTTP_CODE:%{http_code} SIZE_UPLOAD:%{size_upload} TIME_TOTAL:%{time_total}" \
         2>&1) || true
  echo "[OpenRewrite] diff.patch upload response: ${UPLOAD_RESPONSE}"
  set -e
  # Also upload transform.log alongside diff for debugging
  if [ -s "${OUTPUT_DIR}/transform.log" ]; then
    LOG_URL="${TARGET_URL%/diff.patch}/transform.log"
    echo "[OpenRewrite] Uploading transform.log to $LOG_URL"
    set +e
    curl -X PUT "$LOG_URL" \
         --data-binary "@${OUTPUT_DIR}/transform.log" \
         -H "Content-Type: text/plain" \
         --connect-timeout 30 \
         --max-time 300 \
         -w "HTTP_CODE:%{http_code} SIZE_UPLOAD:%{size_upload} TIME_TOTAL:%{time_total}" \
         -s 2>&1 | sed -n '1,2p'
    set -e
  fi
fi

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
