# Knowledge Base Learning System

The Knowledge Base (KB) learning system is Ploy's intelligent error pattern recognition and solution caching system. It automatically learns from transflow healing attempts to improve future success rates and reduce manual intervention.

## Overview

The KB system captures and analyzes every self-healing attempt during transflow workflows, building a comprehensive database of error patterns and their successful solutions. This enables the system to become more effective over time, automatically applying proven fixes for similar issues.

## Key Components

### Error Signature Canonicalization
- **Purpose**: Convert raw error messages into standardized signatures for pattern matching
- **Process**: 
  1. Extract error type, location, and key identifiers
  2. Normalize variable names and paths  
  3. Generate canonical signature hash
  4. Store signature with original error context

### Patch Fingerprinting
- **Purpose**: Identify and deduplicate similar solutions
- **Process**:
  1. Generate content hash of successful patches
  2. Extract semantic change patterns
  3. Store patch with metadata (success rate, context)
  4. Link patches to error signatures

### Confidence Scoring
- **Purpose**: Predict likelihood of healing success
- **Factors**:
  - Historical success rate for error signature
  - Patch complexity and risk assessment
  - Context similarity to previous cases
  - Time since last successful application

## Storage Architecture

### SeaweedFS Integration
The KB uses SeaweedFS for distributed storage under the `llms/` namespace:

```
/llms/kb/
├── errors/           # Error definitions by signature
├── cases/           # Individual healing attempts
├── summaries/       # Aggregated success patterns
└── patches/         # Deduplicated patch content
```

### Consul Locking
Distributed operations use Consul KV for coordination:
- Concurrent healing attempt recording
- Summary aggregation locks
- Configuration updates
- Cache invalidation

## Learning Workflow

### 1. Error Capture
When a build fails during transflow execution:
```go
errorSig := kb.CanonicalizeBuildError(buildError)
case := &kb.Case{
    ErrorSignature: errorSig,
    Context: extractContext(buildError),
    Timestamp: time.Now(),
}
```

### 2. Healing Attempt Recording
Each healing strategy records its attempt:
```go
attempt := &kb.HealingAttempt{
    Strategy: "llm-exec",
    Patch: generatedPatch,
    Success: buildPassed,
    Duration: healingTime,
}
case.Attempts = append(case.Attempts, attempt)
```

### 3. Success Pattern Aggregation
Successful cases contribute to pattern summaries:
```go
summary := kb.AggregateSuccessPatterns(errorSig)
summary.SuccessRate = calculateSuccessRate(allCases)
summary.BestStrategies = rankStrategiesBySuccess(allCases)
```

### 4. Future Application
When similar errors occur, KB suggests solutions:
```go
suggestions := kb.GetHealingSuggestions(errorSig)
for _, suggestion := range suggestions.OrderByConfidence() {
    if suggestion.Confidence > threshold {
        attemptHealing(suggestion.Patch)
    }
}
```

## Configuration

### Environment Variables
```bash
# Enable KB learning
export KB_ENABLED=true

# Storage configuration
export KB_STORAGE_URL=http://localhost:8888
export KB_STORAGE_TIMEOUT=10s

# Learning parameters
export KB_MIN_CONFIDENCE=0.7
export KB_MAX_CASES_PER_ERROR=100
export KB_SUMMARY_UPDATE_INTERVAL=1h
```

### Mods Integration
Enable KB learning in transflow configuration:
```yaml
self_heal:
  enabled: true
  kb_learning: true        # Enable KB recording
  kb_apply_suggestions: true  # Apply KB suggestions
  kb_confidence_threshold: 0.75
```

## API Endpoints

### Error Queries
```bash
# Get error signature information
GET /v1/llms/kb/errors/{signature}

# List all known error types
GET /v1/llms/kb/errors?limit=50&offset=0

# Search errors by pattern
GET /v1/llms/kb/errors/search?q=java-compilation&limit=20
```

### Case Management
```bash
# Get cases for specific error
GET /v1/llms/kb/errors/{signature}/cases

# Get specific healing case
GET /v1/llms/kb/cases/{case-id}

# Get case statistics
GET /v1/llms/kb/cases/stats?from=2025-01-01&to=2025-12-31
```

### Summaries and Analytics
```bash
# Get success patterns for error
GET /v1/llms/kb/summaries/{signature}

# Get overall KB statistics
GET /v1/llms/kb/stats

# Get learning trends
GET /v1/llms/kb/trends?period=30d&group_by=error_type
```

## Performance Characteristics

### Storage Operations
- **Error recording**: <50ms average
- **Patch storage**: <100ms average  
- **Similarity search**: <200ms average
- **Summary generation**: <500ms average

### Memory Usage
- **In-memory cache**: 100-200MB typical
- **Per-case storage**: 1-5KB average
- **Patch deduplication**: 70% storage reduction
- **Cache hit ratio**: 85%+ for common errors

### Background Processing
- **Summary updates**: Every 1 hour
- **Cache refresh**: Every 15 minutes
- **Cleanup old cases**: Daily at 2 AM
- **Patch deduplication**: Weekly

## Monitoring and Metrics

### Key Metrics
```bash
# KB learning statistics
curl http://localhost:8888/v1/llms/kb/stats

# Response example:
{
  "total_cases": 15420,
  "unique_errors": 342,
  "success_rate": 0.78,
  "avg_healing_time": "45s",
  "cache_hit_ratio": 0.89,
  "storage_usage": "2.1GB"
}
```

### Health Checks
```bash
# KB system health
curl http://localhost:8888/v1/llms/kb/health

# Response example:
{
  "status": "healthy",
  "storage_connection": "ok",
  "consul_connection": "ok",
  "cache_status": "ok",
  "last_update": "2025-01-09T10:30:00Z"
}
```

## Error Types and Patterns

### Java Compilation Errors
```
java-compilation-missing-symbol
java-compilation-type-mismatch
java-compilation-method-not-found
java-compilation-package-not-exists
```

### Build System Errors
```
gradle-dependency-resolution
maven-plugin-execution-failed
npm-dependency-conflict
go-mod-version-conflict
```

### Runtime Errors
```
java-runtime-classnotfound
java-runtime-nullpointer
python-import-error
node-module-not-found
```

## Best Practices

### Enabling KB Learning
1. **Start with high-confidence threshold** (0.8+) to avoid false positives
2. **Monitor learning metrics** to adjust thresholds over time
3. **Review failed cases** manually to improve canonicalization
4. **Clean up duplicate errors** periodically for better performance

### Performance Optimization
1. **Use background processing** for non-critical operations
2. **Implement caching** for frequently accessed patterns
3. **Set reasonable retention** policies for old cases
4. **Monitor storage usage** and compress old data

### Quality Assurance
1. **Validate patch quality** before applying suggestions
2. **Test in non-production** environments first
3. **Maintain human oversight** for critical fixes
4. **Regular audits** of learning patterns for accuracy

## Troubleshooting

### Common Issues

#### KB Not Recording Cases
```bash
# Check KB configuration
echo $KB_ENABLED $KB_STORAGE_URL

# Verify storage connectivity
curl http://localhost:8888/

# Check transflow configuration
grep -r "kb_learning" transflow-config.yaml
```

#### Low Success Rates
```bash
# Review error classification
curl http://localhost:8888/v1/llms/kb/errors | jq '.errors[] | select(.success_rate < 0.5)'

# Check patch quality
curl http://localhost:8888/v1/llms/kb/cases/failed | jq '.cases[0:10]'
```

#### Storage Performance Issues
```bash
# Check storage metrics
curl http://localhost:8888/v1/llms/kb/stats

# Monitor case volume
curl http://localhost:8888/v1/llms/kb/trends?period=7d&metric=case_count
```

### Debug Commands
```bash
# Enable detailed KB logging
export KB_LOG_LEVEL=debug

# Test error canonicalization
curl -X POST http://localhost:8888/v1/llms/kb/errors/canonicalize \
  -d '{"error": "cannot find symbol: class NonExistentClass"}'

# Validate patch application
curl -X POST http://localhost:8888/v1/llms/kb/patches/validate \
  -d @patch-file.json
```

## Future Enhancements

### Planned Features
- **Cross-repository learning**: Share patterns across different projects
- **Advanced similarity matching**: ML-based error pattern recognition
- **Automated patch generation**: AI-generated fixes based on patterns
- **Integration plugins**: IDE and CI/CD integration for real-time suggestions

### Research Areas
- **Semantic code analysis**: Understanding code intent beyond syntax
- **Context-aware suggestions**: Environment and project-specific recommendations  
- **Collaborative filtering**: Learning from team-wide healing patterns
- **Continuous model improvement**: Online learning from user feedback