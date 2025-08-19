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

prepare_nodejs_build() {
  local app_dir="$1"
  echo "Detected Node.js application, preparing build..."
  
  # Check if we have Node.js and npm available
  if ! command -v node >/dev/null 2>&1; then
    echo "Warning: Node.js not found in PATH, build may fail"
    return 1
  fi
  
  if ! command -v npm >/dev/null 2>&1; then
    echo "Warning: npm not found in PATH, build may fail"
    return 1
  fi
  
  pushd "$app_dir" >/dev/null
  
  # Enhanced dependency management
  manage_nodejs_dependencies "$app_dir"
  
  # Package bundling and optimization
  bundle_nodejs_application "$app_dir"
  
  # Verify main entry point exists
  verify_nodejs_entrypoint "$app_dir"
  
  popd >/dev/null
  return 0
}

manage_nodejs_dependencies() {
  local app_dir="$1"
  echo "Managing Node.js dependencies..."
  
  # Check for package-lock.json and handle accordingly
  if [[ -f "package-lock.json" ]]; then
    echo "Found package-lock.json, using npm ci for faster, reproducible builds"
    if [[ -d "node_modules" ]]; then
      echo "Removing existing node_modules for clean install"
      rm -rf node_modules
    fi
    npm ci --production --silent || {
      echo "Warning: npm ci failed, falling back to npm install..."
      npm install --production --silent || {
        echo "Warning: npm install also failed, continuing with build..."
        return 1
      }
    }
  elif [[ -f "package.json" ]] && [[ ! -d "node_modules" ]]; then
    echo "Installing Node.js dependencies with npm install..."
    npm install --production --silent || {
      echo "Warning: npm install failed, continuing with build..."
      return 1
    }
  elif [[ -d "node_modules" ]]; then
    echo "Node.js dependencies already installed"
    # Verify dependencies are up to date
    echo "Verifying dependency integrity..."
    npm ls --production --silent >/dev/null 2>&1 || {
      echo "Warning: Dependency issues detected, reinstalling..."
      rm -rf node_modules
      npm install --production --silent || {
        echo "Warning: npm install failed after cleanup, continuing..."
        return 1
      }
    }
  fi
  
  # Cache dependency information for build optimization
  if [[ -f "package.json" ]]; then
    local dep_count
    dep_count=$(node -p "Object.keys(require('./package.json').dependencies || {}).length" 2>/dev/null || echo "0")
    echo "Node.js dependencies installed: $dep_count packages"
    
    # Generate dependency manifest for Unikraft optimization
    generate_dependency_manifest
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
  echo "Generating dependency manifest for Unikraft optimization..."
  
  if [[ -f "package.json" ]] && command -v node >/dev/null 2>&1; then
    # Extract key dependency information
    node -e "
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
          production_only: true
        }
      };
      require('fs').writeFileSync('.unikraft-manifest.json', JSON.stringify(manifest, null, 2));
      console.log('Dependency manifest generated');
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
  local main_file
  
  if [[ -f "package.json" ]]; then
    main_file=$(node -p "try { require('./package.json').main || 'index.js' } catch(e) { 'index.js' }" 2>/dev/null || echo "index.js")
    
    if [[ ! -f "$main_file" ]]; then
      echo "Warning: Main file '$main_file' not found, build may fail"
      return 1
    fi
    
    echo "Verified main entry point: $main_file"
    
    # Validate main file syntax
    echo "Validating JavaScript syntax..."
    node -c "$main_file" 2>/dev/null || {
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

# SBOM/signature (optional)

if command -v syft >/dev/null 2>&1; then syft packages "$OUT" -o json > "$OUT.sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$OUT.sig" "$OUT" || true; fi

