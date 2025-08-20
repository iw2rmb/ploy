#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --app <name> --app-dir <path> --lane A|B --sha <sha> --out-dir <dir>"; exit 1; }
APP=""; APPDIR=""; LANE="A"; SHA="dev"; OUTDIR="."
while [[ $# -gt 0 ]]; do
  case "$1" in
    --app) APP="$2"; shift 2;;
    --app-dir) APPDIR="$2"; shift 2;;
    --lane) LANE="$2"; shift 2;;
    --sha) SHA="$2"; shift 2;;
    --out-dir) OUTDIR="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APP" || -z "$APPDIR" ]] && usage
OUT="$OUTDIR/${APP}-${SHA}.img"
./build/kraft/gen_kraft_yaml.sh --lane "$LANE" --app-dir "$APPDIR" >/dev/null 2>&1 || true

# Node.js detection and build steps
has_nodejs() {
  [[ -f "$1/package.json" ]]
}

# Extract Node.js version from package.json engines field
get_nodejs_version_from_package() {
  local app_dir="$1"
  local package_json="$app_dir/package.json"
  
  if [[ ! -f "$package_json" ]]; then
    echo "18"  # Default version
    return 0
  fi
  
  # Extract engines.node field using node to parse JSON safely
  local node_version
  # Convert to absolute path for require()
  local abs_package_json
  abs_package_json=$(realpath "$package_json" 2>/dev/null || echo "$package_json")
  
  node_version=$(node -p "
    try {
      const pkg = require('$abs_package_json');
      const engines = pkg.engines || {};
      const nodeVersion = engines.node || '';
      // Handle version ranges like '^18.0.0', '>=16.0.0', '18.x'
      // Extract major version number
      const match = nodeVersion.match(/(\d+)/);
      match ? match[1] : '18';
    } catch (e) {
      '18';
    }
  " 2>/dev/null || echo "18")
  
  echo "$node_version"
}

# Download and extract Node.js binary for Unikraft build
download_nodejs_for_unikraft() {
  local version="$1"
  local app_dir="$2"
  local node_dir="$app_dir/.unikraft-node"
  
  echo "Downloading Node.js v$version for Unikraft build..."
  
  # Create directory for Node.js binary
  mkdir -p "$node_dir"
  
  # Determine architecture and platform
  local arch
  case "$(uname -m)" in
    x86_64) arch="x64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) arch="x64" ;;  # Default to x64
  esac
  
  local platform
  case "$(uname -s)" in
    Linux) platform="linux" ;;
    Darwin) platform="darwin" ;;
    *) platform="linux" ;;  # Default to linux for Unikraft
  esac
  
  local node_filename="node-v$version-$platform-$arch"
  local node_url="https://nodejs.org/dist/v$version/$node_filename.tar.xz"
  local node_archive="$node_dir/$node_filename.tar.xz"
  
  # Download Node.js if not already cached
  if [[ ! -f "$node_dir/bin/node" ]]; then
    echo "Downloading Node.js from: $node_url"
    
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "$node_url" -o "$node_archive" || {
        echo "Warning: Failed to download Node.js v$version, using system node"
        return 1
      }
    elif command -v wget >/dev/null 2>&1; then
      wget -q "$node_url" -O "$node_archive" || {
        echo "Warning: Failed to download Node.js v$version, using system node"
        return 1
      }
    else
      echo "Warning: Neither curl nor wget available, using system node"
      return 1
    fi
    
    # Extract Node.js
    echo "Extracting Node.js v$version..."
    tar -xf "$node_archive" -C "$node_dir" --strip-components=1 || {
      echo "Warning: Failed to extract Node.js, using system node"
      rm -f "$node_archive"
      return 1
    }
    
    # Cleanup archive
    rm -f "$node_archive"
    
    # Verify extraction
    if [[ -f "$node_dir/bin/node" ]]; then
      echo "✓ Node.js v$version downloaded and ready for Unikraft build"
      "$node_dir/bin/node" --version
    else
      echo "Warning: Node.js extraction failed, using system node"
      return 1
    fi
  else
    echo "✓ Node.js v$version already available for Unikraft build"
    "$node_dir/bin/node" --version
  fi
  
  return 0
}

# Setup Node.js version for Unikraft build
setup_nodejs_for_unikraft() {
  local app_dir="$1"
  local required_version
  
  echo "Setting up Node.js for Unikraft build..."
  
  required_version=$(get_nodejs_version_from_package "$app_dir")
  echo "Required Node.js version: $required_version"
  
  # Try to download specific version for Unikraft
  if download_nodejs_for_unikraft "$required_version" "$app_dir"; then
    # Export path to use downloaded Node.js for build
    export UNIKRAFT_NODE_PATH="$app_dir/.unikraft-node/bin"
    echo "✓ Using Node.js v$required_version for Unikraft build"
  else
    echo "⚠ Falling back to system Node.js"
    export UNIKRAFT_NODE_PATH=""
  fi
}

prepare_nodejs_build() {
  local app_dir="$1"
  echo "Detected Node.js application, preparing build..."
  
  # Setup Node.js version for Unikraft build
  setup_nodejs_for_unikraft "$app_dir"
  
  # Check if we have Node.js and npm available (system or downloaded)
  local node_cmd="node"
  local npm_cmd="npm"
  
  if [[ -n "$UNIKRAFT_NODE_PATH" ]]; then
    node_cmd="$UNIKRAFT_NODE_PATH/node"
    npm_cmd="$UNIKRAFT_NODE_PATH/npm"
    echo "Using downloaded Node.js: $node_cmd"
  fi
  
  if ! command -v "$node_cmd" >/dev/null 2>&1; then
    echo "Warning: Node.js not found, build may fail"
    return 1
  fi
  
  if ! command -v "$npm_cmd" >/dev/null 2>&1; then
    echo "Warning: npm not found, build may fail"
    return 1
  fi
  
  pushd "$app_dir" >/dev/null
  
  # Enhanced dependency management with specific Node.js version
  manage_nodejs_dependencies "$app_dir" "$node_cmd" "$npm_cmd"
  
  # Package bundling and optimization
  bundle_nodejs_application "$app_dir"
  
  # Verify main entry point exists
  verify_nodejs_entrypoint "$app_dir" "$node_cmd"
  
  popd >/dev/null
  return 0
}

manage_nodejs_dependencies() {
  local app_dir="$1"
  local node_cmd="${2:-node}"
  local npm_cmd="${3:-npm}"
  echo "Managing Node.js dependencies with $node_cmd..."
  
  # Check for package-lock.json and handle accordingly
  if [[ -f "package-lock.json" ]]; then
    echo "Found package-lock.json, using npm ci for faster, reproducible builds"
    if [[ -d "node_modules" ]]; then
      echo "Removing existing node_modules for clean install"
      rm -rf node_modules
    fi
    "$npm_cmd" ci --production --silent || {
      echo "Warning: npm ci failed, falling back to npm install..."
      "$npm_cmd" install --production --silent || {
        echo "Warning: npm install also failed, continuing with build..."
        return 1
      }
    }
  elif [[ -f "package.json" ]] && [[ ! -d "node_modules" ]]; then
    echo "Installing Node.js dependencies with npm install..."
    "$npm_cmd" install --production --silent || {
      echo "Warning: npm install failed, continuing with build..."
      return 1
    }
  elif [[ -d "node_modules" ]]; then
    echo "Node.js dependencies already installed"
    # Verify dependencies are up to date
    echo "Verifying dependency integrity..."
    "$npm_cmd" ls --production --silent >/dev/null 2>&1 || {
      echo "Warning: Dependency issues detected, reinstalling..."
      rm -rf node_modules
      "$npm_cmd" install --production --silent || {
        echo "Warning: npm install failed after cleanup, continuing..."
        return 1
      }
    }
  fi
  
  # Cache dependency information for build optimization
  if [[ -f "package.json" ]]; then
    local dep_count
    dep_count=$("$node_cmd" -p "Object.keys(require('./package.json').dependencies || {}).length" 2>/dev/null || echo "0")
    echo "Node.js dependencies installed: $dep_count packages"
    
    # Generate dependency manifest for Unikraft optimization
    generate_dependency_manifest "$node_cmd"
  fi
  
  echo "Node.js dependency management completed"
}

bundle_nodejs_application() {
  local app_dir="$1"
  echo "Optimizing Node.js application bundle..."
  
  # Create optimized application structure
  local bundle_dir=".unikraft-bundle"
  
  if [[ -d "$bundle_dir" ]]; then
    rm -rf "$bundle_dir"
  fi
  mkdir -p "$bundle_dir"
  
  # Copy essential application files
  echo "Bundling application files..."
  
  # Copy main application files (exclude development artifacts)
  find . -name "*.js" -not -path "./node_modules/*" -not -path "./.unikraft*" -not -path "./test/*" -not -path "./tests/*" | \
    xargs -I {} cp {} "$bundle_dir/" 2>/dev/null || true
  
  # Copy package.json and lock files
  [[ -f "package.json" ]] && cp "package.json" "$bundle_dir/"
  [[ -f "package-lock.json" ]] && cp "package-lock.json" "$bundle_dir/"
  
  # Copy only production node_modules
  if [[ -d "node_modules" ]]; then
    echo "Bundling production dependencies..."
    cp -r "node_modules" "$bundle_dir/"
    
    # Remove development-only packages from bundle
    if command -v npm >/dev/null 2>&1; then
      pushd "$bundle_dir" >/dev/null
      # Remove dev dependencies that might have been installed
      npm prune --production --silent 2>/dev/null || true
      popd >/dev/null
    fi
  fi
  
  # Copy additional runtime files (if they exist)
  for file in ".env.production" "config.json" "public" "views" "static"; do
    [[ -e "$file" ]] && cp -r "$file" "$bundle_dir/" 2>/dev/null || true
  done
  
  # Create startup script for Unikraft
  create_unikraft_startup_script "$bundle_dir"
  
  echo "Application bundle created in $bundle_dir"
}

generate_dependency_manifest() {
  local node_cmd="${1:-node}"
  echo "Generating dependency manifest for Unikraft optimization..."
  
  if [[ -f "package.json" ]] && command -v "$node_cmd" >/dev/null 2>&1; then
    # Extract key dependency information
    "$node_cmd" -e "
      const pkg = require('./package.json');
      const manifest = {
        name: pkg.name || 'nodejs-app',
        version: pkg.version || '1.0.0',
        main: pkg.main || 'index.js',
        scripts: pkg.scripts || {},
        dependencies: Object.keys(pkg.dependencies || {}),
        engines: pkg.engines || {},
        unikraft: {
          optimized: true,
          bundle_created: new Date().toISOString(),
          production_only: true,
          node_version: process.version
        }
      };
      require('fs').writeFileSync('.unikraft-manifest.json', JSON.stringify(manifest, null, 2));
      console.log('Dependency manifest generated with Node.js ' + process.version);
    " 2>/dev/null || echo "Warning: Could not generate dependency manifest"
  fi
}

create_unikraft_startup_script() {
  local bundle_dir="$1"
  local main_file
  
  # Determine main entry point
  if [[ -f "package.json" ]]; then
    main_file=$(node -p "try { require('./package.json').main || 'index.js' } catch(e) { 'index.js' }" 2>/dev/null || echo "index.js")
  else
    main_file="index.js"
  fi
  
  # Create optimized startup script
  cat > "$bundle_dir/start.js" << EOF
#!/usr/bin/env node
// Unikraft optimized startup script
// Generated by Ploy build system

// Set production environment
process.env.NODE_ENV = process.env.NODE_ENV || 'production';

// Optimize memory usage for unikernel
if (global.gc) {
  global.gc();
}

// Load main application
try {
  require('./$main_file');
} catch (error) {
  console.error('Failed to start application:', error);
  process.exit(1);
}
EOF
  
  chmod +x "$bundle_dir/start.js"
  echo "Startup script created: start.js"
}

verify_nodejs_entrypoint() {
  local app_dir="$1"
  local node_cmd="${2:-node}"
  local main_file
  
  if [[ -f "package.json" ]]; then
    main_file=$("$node_cmd" -p "try { require('./package.json').main || 'index.js' } catch(e) { 'index.js' }" 2>/dev/null || echo "index.js")
    
    if [[ ! -f "$main_file" ]]; then
      echo "Warning: Main file '$main_file' not found, build may fail"
      return 1
    fi
    
    echo "Verified main entry point: $main_file"
    
    # Validate main file syntax
    echo "Validating JavaScript syntax with $node_cmd..."
    "$node_cmd" -c "$main_file" 2>/dev/null || {
      echo "Warning: Syntax errors detected in $main_file"
      return 1
    }
    
    echo "JavaScript syntax validation passed"
  fi
  
  return 0
}

pushd "$APPDIR" >/dev/null

# Lane B specific Node.js handling
if [[ "$LANE" == "B" ]] && has_nodejs "$APPDIR"; then
  prepare_nodejs_build "$APPDIR"
fi

if command -v kraft >/dev/null 2>&1; then
  echo "Running kraft build..."
  kraft build -j 4 2>&1 | tee kraft_build.log || {
    echo "Kraft build failed, creating placeholder image"
    echo "Build log:"
    cat kraft_build.log 2>/dev/null || echo "No build log available"
    dd if=/dev/zero of="$OUT" bs=1M count=4
    echo "Created placeholder image due to build failure: $OUT"
    popd >/dev/null
    echo "$OUT"
    exit 0
  }
  
  if [[ -f build/image_qemu ]]; then 
    cp build/image_qemu "$OUT"
    echo "Unikraft build completed successfully: $OUT"
  elif [[ -f .unikraft/build/image_qemu ]]; then
    cp .unikraft/build/image_qemu "$OUT"
    echo "Unikraft build completed (alternate path): $OUT"
  else 
    echo "Build completed but no image found, creating placeholder"
    dd if=/dev/zero of="$OUT" bs=1M count=4
    echo "Created placeholder image: $OUT"
  fi
else
  dd if=/dev/zero of="$OUT" bs=1M count=4
  echo "kraft not available, created placeholder image: $OUT"
fi
popd >/dev/null
echo "$OUT"

# Enhanced SBOM generation with comprehensive metadata
if command -v syft >/dev/null 2>&1; then
  echo "Generating comprehensive SBOM for $OUT..."
  # Generate SPDX-JSON format SBOM with full cataloger analysis
  syft scan "$OUT" \
    -o spdx-json \
    --file "$OUT.sbom.json" || true
  
  # Also generate source code dependencies SBOM if in a source directory
  if [ -n "${APP_DIR:-}" ] && [ -d "${APP_DIR:-}" ]; then
    echo "Generating source dependencies SBOM..."
    syft scan "${APP_DIR}" \
      -o spdx-json \
      --file "${APP_DIR}/.sbom.json" || true
  fi
else
  echo "Warning: syft not found, skipping comprehensive SBOM generation"
fi

# Enhanced keyless OIDC artifact signing
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$SCRIPT_DIR/../common/signing.sh" ]]; then
  source "$SCRIPT_DIR/../common/signing.sh"
  
  echo "🔐 Enhanced Signing Configuration:"
  print_signing_config
  
  echo "🖊️  Signing artifact with keyless OIDC..."
  if sign_ploy_artifact "$OUT" "artifact"; then
    echo "✅ Artifact signed successfully: $OUT"
  else
    echo "⚠️  Artifact signing failed, but continuing build"
  fi
else
  # Fallback to basic signing if common functions not available
  echo "⚠️  Common signing functions not found, using basic signing"
  if command -v cosign >/dev/null 2>&1; then 
    cosign sign-blob --yes --output-signature "$OUT.sig" "$OUT" || true
  fi
fi

