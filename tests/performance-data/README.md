# Performance Test Data

This directory contains test data projects for benchmarking CHTTP vs legacy static analysis performance.

## Test Project Sizes

### Small Projects (1-5 files, <1MB)
- **Purpose**: Test baseline performance with minimal projects
- **Files**: 2-5 Python files with basic Pylint issues
- **Target Response Time**: <2 seconds
- **Use Case**: Small scripts, utilities, simple applications

### Medium Projects (10-50 files, 1-10MB)
- **Purpose**: Test performance with typical project sizes
- **Files**: 15-30 Python files with moderate complexity
- **Target Response Time**: <5 seconds (roadmap target)
- **Use Case**: Standard applications, libraries, microservices

### Large Projects (100+ files, 10-50MB)
- **Purpose**: Test performance with enterprise-scale projects
- **Files**: 75+ Python files with complex interdependencies
- **Target Response Time**: <10 seconds (acceptable for large projects)
- **Use Case**: Large applications, frameworks, monolithic services

## Generated Test Data

The benchmark script automatically generates test data with:

- **Realistic Python Code**: Based on common patterns and structures
- **Pylint Issues**: Intentional code quality issues for analysis
- **Scalable Complexity**: Increasing complexity with project size
- **Consistent Structure**: Standardized format for reliable benchmarking

## Performance Targets (from roadmap)

- **Response Time**: <5 seconds for typical Python projects
- **Throughput**: 50+ concurrent analyses
- **Resource Usage**: <100MB per CHTTP service
- **Availability**: 99.9% uptime for CHTTP services

## Usage

Test data is automatically generated when running the performance benchmark:

```bash
./tests/scripts/benchmark-chttp-performance.sh
```

Manual generation (if needed):
```bash
TEST_DATA_DIR=./tests/performance-data ./tests/scripts/benchmark-chttp-performance.sh
```

## File Structure

After generation, this directory will contain:

```
performance-data/
├── README.md
├── python-small-project.json          # Legacy analysis payload  
├── python-small-project-chttp.json    # CHTTP analysis payload
├── python-medium-project.json         # Legacy analysis payload
├── python-medium-project-chttp.json   # CHTTP analysis payload  
├── python-large-project.json          # Legacy analysis payload
└── python-large-project-chttp.json    # CHTTP analysis payload
```

## JSON Payload Format

Each test payload follows this structure:

```json
{
    "repository": {
        "id": "perf-test-size-type",
        "name": "python-size-project",
        "url": "file:///path/to/test/project",
        "commit": "main"
    },
    "config": {
        "enabled": true,
        "mode": "legacy|chttp",
        "languages": {
            "python": {
                "pylint": true,
                "enabled": true,
                "service_url": "https://pylint.chttp.dev.ployd.app"
            }
        }
    },
    "metadata": {
        "size": "small|medium|large",
        "estimated_files": 5,
        "estimated_size_bytes": 1048576,
        "test_type": "legacy|chttp"
    }
}
```

## Integration with Analysis API

Test payloads are designed to work with:

- **Legacy Mode**: In-process Pylint analysis via existing engine
- **CHTTP Mode**: Distributed Pylint analysis via CHTTP microservice  
- **Analysis API**: `POST /v1/analysis/analyze` endpoint

## Performance Validation

The benchmark validates against success metrics:

- ✅ Response times within targets
- ✅ Concurrent analysis capacity
- ✅ Resource usage efficiency  
- ✅ Error rates and reliability