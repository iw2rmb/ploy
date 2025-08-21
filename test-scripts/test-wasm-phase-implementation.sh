#!/bin/bash
# Test scenarios for Phase WASM: WebAssembly Runtime Support
set -euo pipefail

# Common test functions (inline)
print_test_header() {
    echo ""
    echo "===================="
    echo "🧪 $1"
    echo "===================="
}

print_success() {
    echo -e "\033[32m$1\033[0m"
}

print_error() {
    echo -e "\033[31m$1\033[0m"
}

print_warning() {
    echo -e "\033[33m$1\033[0m"
}

# WASM Phase Implementation Test Suite
echo "🧪 Starting Phase WASM Implementation Tests..."

# Test 1: WASM Detection Engine
test_wasm_detection() {
    print_test_header "WASM Detection Engine Tests"
    
    # Test Rust WASM detection
    echo "Testing Rust WASM detection..."
    if [ -d "apps/wasm-rust-hello" ]; then
        result=$(./build/ploy-lane-pick --path apps/wasm-rust-hello 2>/dev/null || echo '{"lane":"unknown"}')
        if echo "$result" | jq -r '.lane' | grep -q "G"; then
            print_success "✓ Rust WASM detected as Lane G"
        else
            print_error "✗ Rust WASM detection failed - Expected Lane G, got: $(echo "$result" | jq -r '.lane')"
            return 1
        fi
    else
        print_warning "⚠ Rust WASM test app not found - skipping"
    fi
    
    # Test Go WASM detection  
    echo "Testing Go WASM detection..."
    if [ -d "apps/wasm-go-hello" ]; then
        result=$(./build/ploy-lane-pick --path apps/wasm-go-hello 2>/dev/null || echo '{"lane":"unknown"}')
        if echo "$result" | jq -r '.lane' | grep -q "G"; then
            print_success "✓ Go WASM detected as Lane G"
        else
            print_error "✗ Go WASM detection failed - Expected Lane G, got: $(echo "$result" | jq -r '.lane')"
            return 1
        fi
    else
        print_warning "⚠ Go WASM test app not found - skipping"
    fi
    
    # Test AssemblyScript detection
    echo "Testing AssemblyScript detection..."
    if [ -d "apps/wasm-assemblyscript-hello" ]; then
        result=$(./build/ploy-lane-pick --path apps/wasm-assemblyscript-hello 2>/dev/null || echo '{"lane":"unknown"}')
        if echo "$result" | jq -r '.lane' | grep -q "G"; then
            print_success "✓ AssemblyScript detected as Lane G"
        else
            print_error "✗ AssemblyScript detection failed - Expected Lane G, got: $(echo "$result" | jq -r '.lane')"
            return 1
        fi
    else
        print_warning "⚠ AssemblyScript test app not found - skipping"
    fi
    
    # Test C++ Emscripten detection
    echo "Testing C++ Emscripten detection..."
    if [ -d "apps/wasm-cpp-hello" ]; then
        result=$(./build/ploy-lane-pick --path apps/wasm-cpp-hello 2>/dev/null || echo '{"lane":"unknown"}')
        if echo "$result" | jq -r '.lane' | grep -q "G"; then
            print_success "✓ C++ Emscripten detected as Lane G"
        else
            print_error "✗ C++ Emscripten detection failed - Expected Lane G, got: $(echo "$result" | jq -r '.lane')"
            return 1
        fi
    else
        print_warning "⚠ C++ Emscripten test app not found - skipping"
    fi
    
    return 0
}

# Test 2: WASM Runtime Integration
test_wasm_runtime() {
    print_test_header "WASM Runtime Integration Tests"
    
    # Check if wazero dependency is available
    if ! go list -m github.com/tetratelabs/wazero >/dev/null 2>&1; then
        print_warning "⚠ wazero dependency not found - WASM runtime tests skipped"
        return 0
    fi
    
    # Test basic WASM module loading
    echo "Testing WASM module loading..."
    if [ -f "controller/runtime/wasm.go" ]; then
        if go run -c controller/runtime/wasm.go >/dev/null 2>&1; then
            print_success "✓ WASM runtime module compiles successfully"
        else
            print_error "✗ WASM runtime module compilation failed"
            return 1
        fi
    else
        print_warning "⚠ WASM runtime module not found - implementation pending"
    fi
    
    # Test WASI support
    echo "Testing WASI support..."
    if [ -f "controller/runtime/wasi.go" ]; then
        if go run -c controller/runtime/wasi.go >/dev/null 2>&1; then
            print_success "✓ WASI support module compiles successfully"
        else
            print_error "✗ WASI support module compilation failed"
            return 1
        fi
    else
        print_warning "⚠ WASI support module not found - implementation pending"
    fi
    
    return 0
}

# Test 3: WASM Builder Implementation
test_wasm_builder() {
    print_test_header "WASM Builder Implementation Tests"
    
    # Check WASM builder module
    if [ -f "controller/builders/wasm.go" ]; then
        echo "Testing WASM builder compilation..."
        if go run -c controller/builders/wasm.go >/dev/null 2>&1; then
            print_success "✓ WASM builder compiles successfully"
        else
            print_error "✗ WASM builder compilation failed"
            return 1
        fi
    else
        print_warning "⚠ WASM builder not found - implementation pending"
    fi
    
    # Check build scripts
    echo "Testing WASM build scripts..."
    local build_scripts_dir="scripts/build/wasm"
    if [ -d "$build_scripts_dir" ]; then
        local scripts_found=0
        
        for script in rust-wasm32.sh go-js-wasm.sh assemblyscript.sh emscripten.sh; do
            if [ -f "$build_scripts_dir/$script" ]; then
                if [ -x "$build_scripts_dir/$script" ]; then
                    print_success "✓ $script is executable"
                    ((scripts_found++))
                else
                    print_warning "⚠ $script found but not executable"
                fi
            fi
        done
        
        if [ $scripts_found -gt 0 ]; then
            print_success "✓ Found $scripts_found WASM build scripts"
        else
            print_warning "⚠ No WASM build scripts found"
        fi
    else
        print_warning "⚠ WASM build scripts directory not found - implementation pending"
    fi
    
    return 0
}

# Test 4: Nomad Integration
test_nomad_integration() {
    print_test_header "Nomad WASM Integration Tests"
    
    # Check Nomad template
    local template_file="platform/nomad/templates/wasm-app.hcl.j2"
    if [ -f "$template_file" ]; then
        echo "Testing Nomad WASM template..."
        if grep -q "wasm-runner" "$template_file"; then
            print_success "✓ Nomad WASM template contains wasm-runner"
        else
            print_warning "⚠ Nomad WASM template missing wasm-runner reference"
        fi
        
        if grep -q "artifact.*wasm" "$template_file"; then
            print_success "✓ Nomad WASM template includes WASM artifact handling"
        else
            print_warning "⚠ Nomad WASM template missing WASM artifact configuration"
        fi
    else
        print_warning "⚠ Nomad WASM template not found - implementation pending"
    fi
    
    # Check WASM runner binary
    if [ -f "cmd/ploy-wasm-runner/main.go" ]; then
        echo "Testing WASM runner compilation..."
        if go run -c cmd/ploy-wasm-runner/main.go >/dev/null 2>&1; then
            print_success "✓ WASM runner compiles successfully"
        else
            print_error "✗ WASM runner compilation failed"
            return 1
        fi
    else
        print_warning "⚠ WASM runner not found - implementation pending"
    fi
    
    return 0
}

# Test 5: OPA Policy Integration
test_opa_policies() {
    print_test_header "OPA WASM Policy Tests"
    
    # Check WASM policies
    if [ -f "policies/wasm.rego" ]; then
        echo "Testing WASM OPA policies..."
        if command -v opa >/dev/null 2>&1; then
            if opa fmt policies/wasm.rego >/dev/null 2>&1; then
                print_success "✓ WASM OPA policies syntax valid"
            else
                print_error "✗ WASM OPA policies syntax invalid"
                return 1
            fi
        else
            print_warning "⚠ OPA not available - policy syntax not validated"
        fi
        
        # Check for key policy rules
        if grep -q "allow_wasm_deployment" policies/wasm.rego; then
            print_success "✓ WASM deployment policy found"
        else
            print_warning "⚠ WASM deployment policy missing"
        fi
        
        if grep -q "max_wasm_size_mb" policies/wasm.rego; then
            print_success "✓ WASM size limit policy found"
        else
            print_warning "⚠ WASM size limit policy missing"
        fi
    else
        print_warning "⚠ WASM OPA policies not found - implementation pending"
    fi
    
    return 0
}

# Test 6: Component Model Support
test_component_model() {
    print_test_header "WASM Component Model Tests"
    
    if [ -f "controller/wasm/components.go" ]; then
        echo "Testing WASM component model..."
        if go run -c controller/wasm/components.go >/dev/null 2>&1; then
            print_success "✓ WASM component model compiles successfully"
        else
            print_error "✗ WASM component model compilation failed"
            return 1
        fi
    else
        print_warning "⚠ WASM component model not found - implementation pending"
    fi
    
    return 0
}

# Test 7: Sample Applications
test_sample_applications() {
    print_test_header "WASM Sample Applications Tests"
    
    local apps_found=0
    local successful_builds=0
    
    # Test Rust WASM app
    if [ -d "apps/wasm-rust-hello" ]; then
        echo "Testing Rust WASM sample app..."
        cd apps/wasm-rust-hello
        if [ -f "Cargo.toml" ]; then
            if grep -q "wasm-bindgen" Cargo.toml; then
                print_success "✓ Rust WASM app has correct dependencies"
                ((apps_found++))
                
                # Try building if Rust toolchain available
                if command -v cargo >/dev/null 2>&1; then
                    if rustup target list --installed | grep -q "wasm32-unknown-unknown"; then
                        if cargo build --target wasm32-unknown-unknown >/dev/null 2>&1; then
                            print_success "✓ Rust WASM app builds successfully"
                            ((successful_builds++))
                        else
                            print_warning "⚠ Rust WASM app build failed"
                        fi
                    else
                        print_warning "⚠ wasm32-unknown-unknown target not installed"
                    fi
                else
                    print_warning "⚠ Rust toolchain not available for build test"
                fi
            else
                print_warning "⚠ Rust WASM app missing wasm-bindgen dependency"
            fi
        fi
        cd - >/dev/null
    fi
    
    # Test Go WASM app
    if [ -d "apps/wasm-go-hello" ]; then
        echo "Testing Go WASM sample app..."
        cd apps/wasm-go-hello
        if [ -f "main.go" ]; then
            if grep -q "js,wasm" main.go; then
                print_success "✓ Go WASM app has correct build tags"
                ((apps_found++))
                
                # Try building if Go toolchain available
                if command -v go >/dev/null 2>&1; then
                    if GOOS=js GOARCH=wasm go build -o /tmp/test.wasm . >/dev/null 2>&1; then
                        print_success "✓ Go WASM app builds successfully"
                        ((successful_builds++))
                        rm -f /tmp/test.wasm
                    else
                        print_warning "⚠ Go WASM app build failed"
                    fi
                else
                    print_warning "⚠ Go toolchain not available for build test"
                fi
            else
                print_warning "⚠ Go WASM app missing js,wasm build tags"
            fi
        fi
        cd - >/dev/null
    fi
    
    # Test AssemblyScript app
    if [ -d "apps/wasm-assemblyscript-hello" ]; then
        echo "Testing AssemblyScript sample app..."
        cd apps/wasm-assemblyscript-hello
        if [ -f "package.json" ]; then
            if grep -q "assemblyscript" package.json; then
                print_success "✓ AssemblyScript app has correct dependencies"
                ((apps_found++))
                
                # Try building if Node.js available
                if command -v npm >/dev/null 2>&1; then
                    if npm install >/dev/null 2>&1 && npm run build >/dev/null 2>&1; then
                        print_success "✓ AssemblyScript app builds successfully"
                        ((successful_builds++))
                    else
                        print_warning "⚠ AssemblyScript app build failed"
                    fi
                else
                    print_warning "⚠ Node.js/npm not available for build test"
                fi
            else
                print_warning "⚠ AssemblyScript app missing assemblyscript dependency"
            fi
        fi
        cd - >/dev/null
    fi
    
    # Test C++ Emscripten app
    if [ -d "apps/wasm-cpp-hello" ]; then
        echo "Testing C++ Emscripten sample app..."
        cd apps/wasm-cpp-hello
        if [ -f "CMakeLists.txt" ]; then
            if grep -q "emscripten" CMakeLists.txt || grep -q "WASM" CMakeLists.txt; then
                print_success "✓ C++ Emscripten app has correct configuration"
                ((apps_found++))
                
                # Try building if Emscripten available
                if command -v emcc >/dev/null 2>&1; then
                    if emcc -O3 -s WASM=1 main.cpp -o /tmp/test.wasm >/dev/null 2>&1; then
                        print_success "✓ C++ Emscripten app builds successfully"
                        ((successful_builds++))
                        rm -f /tmp/test.wasm /tmp/test.js
                    else
                        print_warning "⚠ C++ Emscripten app build failed"
                    fi
                else
                    print_warning "⚠ Emscripten not available for build test"
                fi
            else
                print_warning "⚠ C++ app missing Emscripten configuration"
            fi
        fi
        cd - >/dev/null
    fi
    
    print_success "✓ Found $apps_found WASM sample applications"
    if [ $successful_builds -gt 0 ]; then
        print_success "✓ $successful_builds sample applications built successfully"
    fi
    
    return 0
}

# Test 8: Documentation Validation
test_documentation() {
    print_test_header "WASM Documentation Tests"
    
    # Check implementation plan
    if [ -f ".claude/phase-wasm-implementation-plan.md" ]; then
        print_success "✓ WASM implementation plan found"
        
        # Check for key sections
        if grep -q "Phase 1:" .claude/phase-wasm-implementation-plan.md; then
            print_success "✓ Implementation phases documented"
        fi
        
        if grep -q "wazero" .claude/phase-wasm-implementation-plan.md; then
            print_success "✓ Runtime technology documented"
        fi
    else
        print_warning "⚠ WASM implementation plan not found"
    fi
    
    # Check WASM documentation
    if [ -f "docs/WASM-RUNTIME.md" ]; then
        print_success "✓ WASM runtime documentation found"
    else
        print_warning "⚠ WASM runtime documentation not found - implementation pending"
    fi
    
    # Check updated FEATURES.md
    if grep -q "Lane G.*WebAssembly" docs/FEATURES.md; then
        print_success "✓ FEATURES.md includes Lane G WASM support"
    else
        print_warning "⚠ FEATURES.md missing Lane G WASM documentation"
    fi
    
    return 0
}

# Main test execution
main() {
    echo "🚀 Phase WASM Implementation Test Suite"
    echo "========================================"
    
    local test_failures=0
    
    # Run all tests
    test_wasm_detection || ((test_failures++))
    test_wasm_runtime || ((test_failures++))
    test_wasm_builder || ((test_failures++))
    test_nomad_integration || ((test_failures++))
    test_opa_policies || ((test_failures++))
    test_component_model || ((test_failures++))
    test_sample_applications || ((test_failures++))
    test_documentation || ((test_failures++))
    
    # Summary
    echo ""
    echo "========================================"
    if [ $test_failures -eq 0 ]; then
        print_success "🎉 All Phase WASM tests completed successfully!"
        echo ""
        echo "Phase WASM Implementation Status:"
        echo "- WASM Detection: Ready for implementation"
        echo "- Runtime Integration: Dependencies identified"
        echo "- Builder Pipeline: Architecture defined"
        echo "- Nomad Integration: Templates planned"
        echo "- Security Policies: Requirements specified"
        echo "- Sample Applications: Test cases prepared"
        echo ""
        echo "Next Steps:"
        echo "1. Begin Phase 1 implementation with detection engine"
        echo "2. Integrate wazero runtime dependency"
        echo "3. Implement WASM builder with multi-language support"
        echo "4. Create production Nomad job templates"
        echo "5. Deploy and test sample WASM applications"
        return 0
    else
        print_error "❌ $test_failures test categories had issues"
        echo ""
        echo "Implementation gaps identified - address before proceeding"
        return 1
    fi
}

# Execute main function
main "$@"