#!/bin/sh
echo "=== ARTIFACT DOWNLOAD TEST ==="
echo "Current working directory: $(pwd)"
echo "User: $(whoami)"
echo "Date: $(date)"
echo ""

echo "=== CHECKING /local DIRECTORY ==="
if [ -d "local" ]; then
  echo "✓ local/ directory exists"
  echo "Contents of local/:"
  ls -la local/
  echo ""
  echo "File count in local/: $(find local -type f | wc -l)"
  if [ -f "local/input.tar" ]; then
    echo "✓ input.tar found!"
    echo "Size: $(ls -lh local/input.tar | awk '{print $5}')"
    echo "First few files in tar:"
    tar -tf local/input.tar | head -10
    echo ""
    echo "Extracting tar to test integrity:"
    mkdir -p /tmp/extract-test
    if tar -xf local/input.tar -C /tmp/extract-test; then
      echo "✓ Tar extraction successful"
      echo "Extracted files:"
      find /tmp/extract-test -type f | head -10
    else
      echo "✗ Tar extraction failed"
    fi
  else
    echo "✗ input.tar NOT found"
  fi
else
  echo "✗ local/ directory does not exist"
fi

echo ""
echo "=== FULL DIRECTORY STRUCTURE ==="
find . -name "*.tar" 2>/dev/null || echo "No .tar files found anywhere"
echo ""
echo "All files and directories:"
ls -la .

echo ""
echo "=== ENVIRONMENT CHECK ==="
echo "PATH: $PATH"
echo "HOME: $HOME"
echo "PWD: $PWD"

echo ""
echo "=== NETWORK CONNECTIVITY TEST ==="
echo "Testing SeaweedFS connectivity:"
if command -v wget > /dev/null 2>&1; then
  wget -q --spider http://seaweedfs-filer.service.consul:8888/artifacts/ && echo "✓ SeaweedFS reachable" || echo "✗ SeaweedFS unreachable"
elif command -v curl > /dev/null 2>&1; then
  curl -s -f http://seaweedfs-filer.service.consul:8888/artifacts/ > /dev/null && echo "✓ SeaweedFS reachable" || echo "✗ SeaweedFS unreachable"  
else
  echo "No wget or curl available for connectivity test"
fi

echo ""
echo "=== TEST COMPLETE ==="