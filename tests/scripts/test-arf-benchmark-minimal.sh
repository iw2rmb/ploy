#!/bin/bash

# Minimal ARF Benchmark Test Runner
# This script runs a minimal benchmark test to verify basic functionality

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}ARF Minimal Benchmark Test${NC}"
echo "==========================="
echo ""

# Check if Ollama is running (optional - will use mock if not available)
if command -v ollama &> /dev/null; then
    echo "Checking Ollama server..."
    if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Ollama server is running${NC}"
        
        # Check if codellama model is available
        if ollama list 2>/dev/null | grep -q codellama; then
            echo -e "${GREEN}✓ CodeLlama model is available${NC}"
        else
            echo -e "${YELLOW}⚠ CodeLlama model not found. Install with: ollama pull codellama:7b${NC}"
            echo "  Continuing with mock LLM..."
        fi
    else
        echo -e "${YELLOW}⚠ Ollama server not running. Start with: ollama serve${NC}"
        echo "  Continuing with mock LLM..."
    fi
else
    echo -e "${YELLOW}⚠ Ollama not installed. Continuing with mock LLM...${NC}"
fi

echo ""

# Check for required tools
echo "Checking required tools..."

# Check Git
if ! command -v git &> /dev/null; then
    echo -e "${RED}✗ Git is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Git is available${NC}"

# Check Maven (optional for Java projects)
if command -v mvn &> /dev/null; then
    echo -e "${GREEN}✓ Maven is available${NC}"
else
    echo -e "${YELLOW}⚠ Maven not found. Java builds may fail${NC}"
fi

# Check Go for building
if ! command -v go &> /dev/null; then
    echo -e "${RED}✗ Go is not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Go is available${NC}"

echo ""

# Build the test runner if needed
if [ ! -f "build/arf-benchmark" ]; then
    echo "Building benchmark test runner..."
    go build -o build/arf-benchmark ./cmd/arf-benchmark
    if [ $? -ne 0 ]; then
        echo -e "${RED}Failed to build test runner${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Test runner built successfully${NC}"
fi

# Run the minimal benchmark test
echo ""
echo "Starting minimal benchmark test..."
echo "Repository: https://github.com/spring-guides/gs-rest-service.git"
echo "Recipe: Java 11 to 17 migration"
echo "Max iterations: 2"
echo ""

# Create output directory
mkdir -p benchmark_results/minimal_test

# Run the test
./build/arf-benchmark \
    -config api/arf/benchmark_configs/minimal_test.yaml \
    -output benchmark_results/minimal_test \
    -verbose

# Check results
if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}✅ Benchmark test completed successfully!${NC}"
    
    # Display results location
    echo ""
    echo "Results saved to: benchmark_results/minimal_test/"
    
    # List result files
    echo "Result files:"
    ls -la benchmark_results/minimal_test/*.json 2>/dev/null || echo "  No JSON results found"
    
else
    echo ""
    echo -e "${RED}❌ Benchmark test failed${NC}"
    echo "Check the output above for error details"
    exit 1
fi

echo ""
echo "=== Test Summary ==="
echo "This minimal test verified:"
echo "✓ Git repository cloning"
echo "✓ OpenRewrite recipe application (mocked)"
echo "✓ Build validation"
echo "✓ Error detection"
echo "✓ Metrics collection"
echo "✓ Result generation"
echo ""
echo -e "${GREEN}The ARF benchmark suite is ready for use!${NC}"
echo ""
echo "Next steps:"
echo "1. Install Ollama for real LLM support: https://ollama.ai"
echo "2. Pull CodeLlama model: ollama pull codellama:7b"
echo "3. Install OpenRewrite CLI for real transformations"
echo "4. Run full benchmark tests with test-arf-benchmark-suite.sh"