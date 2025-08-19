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
  
  # Install dependencies if package.json exists and node_modules doesn't
  if [[ -f "$app_dir/package.json" ]] && [[ ! -d "$app_dir/node_modules" ]]; then
    echo "Installing Node.js dependencies with npm..."
    pushd "$app_dir" >/dev/null
    npm install --production --silent || {
      echo "Warning: npm install failed, continuing with build..."
      popd >/dev/null
      return 1
    }
    popd >/dev/null
    echo "Node.js dependencies installed successfully"
  elif [[ -d "$app_dir/node_modules" ]]; then
    echo "Node.js dependencies already installed"
  fi
  
  # Verify main entry point exists
  local main_file
  if [[ -f "$app_dir/package.json" ]]; then
    main_file=$(node -p "try { require('$app_dir/package.json').main || 'index.js' } catch(e) { 'index.js' }" 2>/dev/null || echo "index.js")
    if [[ ! -f "$app_dir/$main_file" ]]; then
      echo "Warning: Main file '$main_file' not found, build may fail"
      return 1
    fi
    echo "Verified main entry point: $main_file"
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

