# Aster Integration Roadmap

**Status**: Planned  
**Estimated Time**: 2-3 weeks  
**Priority**: High  
**Dependencies**: CLLM Phase 2 completion, Aster MCP availability

## Overview

Integration of Aster's advanced AST-based code analysis capabilities with Ploy's CLLM service to enhance error analysis and context building. This hybrid approach combines Aster's semantic analysis with ARF-specific optimizations for superior code transformation workflows.

## Strategic Rationale

### Current State Analysis
- **CLLM**: Custom context building with 3000-token budget, ARF-specific patterns, ~51ms performance
- **Aster**: Comprehensive AST analysis, 7 language support, 30-97% token reduction, proven 83.4% success rate

### Integration Benefits
- **50% iteration reduction** (proven by Aster metrics in similar workflows)
- **Multi-language support** expansion from Java-only to 7 languages
- **Smart token optimization** with automatic format selection
- **Enhanced semantic understanding** via Tree-sitter AST parsing
- **Preserve ARF specialization** while gaining general semantic analysis

### Strategic Decision: Hybrid Enhancement
**Rationale**: Rather than replacement, enhance CLLM with Aster's capabilities while maintaining ARF-specific optimizations and workflow integration.

## Technical Architecture

### Integration Architecture
```
Enhanced CLLM with Aster Integration:
┌─────────────────────────────────┐
│   ARF Workflow Integration      │ ← PRESERVE: ARF-specific patterns & optimization
└─────────────────────────────────┘
                 │
┌─────────────────────────────────┐
│   CLLM Service (Enhanced)       │ ← ENHANCE: Add Aster MCP client
│   - ARF Error Analysis          │
│   - Aster MCP Integration       │
│   - Hybrid Context Building     │
└─────────────────────────────────┘
                 │
┌─────────────────┬───────────────┐
│  Current CLLM   │  Aster MCP    │
│  - ARF Patterns │  - AST Analysis│ ← COMBINE: Best of both approaches
│  - Token Budget │  - Smart Format│
│  - Priority     │  - Multi-Lang  │
│    Sections     │  - Dataflow   │
└─────────────────┴───────────────┘
```

### Component Enhancement Strategy
```
services/cllm/internal/
├── arf/
│   ├── analyzer.go              # ENHANCE: Add Aster semantic analysis
│   ├── context_builder.go       # ENHANCE: Integrate Aster format selection
│   ├── patterns.go             # PRESERVE: ARF-specific patterns
│   └── aster_client.go         # NEW: Aster MCP client integration
├── integration/
│   ├── aster_mcp.go           # NEW: MCP protocol client
│   ├── format_selection.go     # NEW: Smart format selection logic
│   └── hybrid_context.go       # NEW: Combine CLLM + Aster contexts
└── monitoring/
    └── aster_metrics.go        # NEW: Integration performance metrics
```

## Implementation Phases

### Phase 1: Foundation & MCP Client (Week 1)
**Priority**: Critical  
**Estimated Time**: 4-5 days

#### Phase 1 Tasks
- [ ] **1.1 Aster MCP Client Implementation**
  - Create MCP client for Aster service communication
  - Implement `aster_context_slice` integration for semantic analysis
  - Add connection management and error handling
  - Integration with existing CLLM configuration patterns

- [ ] **1.2 Basic Format Selection Integration**
  - Implement Aster's smart format selection logic
  - Add complexity scoring for automatic format selection
  - Integrate with existing token budgeting system
  - Preserve backwards compatibility with current ARF workflows

#### Phase 1 Acceptance Criteria
- Aster MCP client successfully connects and communicates
- Basic format selection works with existing CLLM endpoints
- All current CLLM functionality remains unchanged
- Performance baseline established for comparison

### Phase 2: Hybrid Context Building (Week 1-2)
**Priority**: Critical  
**Estimated Time**: 5-6 days

#### Phase 2 Tasks
- [ ] **2.1 Enhanced Semantic Context**
  - Integrate Aster's AST analysis with existing error context building
  - Combine Aster's dataflow analysis with ARF-specific pattern recognition
  - Implement multi-language support using Aster's Tree-sitter parsers
  - Smart context merging to avoid duplication

- [ ] **2.2 Optimized Token Management**
  - Dynamic token budgeting using Aster's complexity scoring
  - Intelligent section prioritization combining CLLM + Aster insights
  - Automatic format selection based on error complexity and context size
  - Token efficiency improvements while maintaining analysis quality

#### Phase 2 Technical Implementation
```go
// Enhanced analyzer with Aster integration
type EnhancedAnalyzer struct {
    baseAnalyzer   *Analyzer        // Existing CLLM analyzer
    asterClient    *AsterMCPClient  // New Aster MCP client
    formatSelector *FormatSelector  // Smart format selection
}

// Hybrid context building
func (ea *EnhancedAnalyzer) buildEnhancedContext(req *ARFAnalysisRequest) (*AnalysisContext, error) {
    // Get semantic context from Aster
    asterContext := ea.asterClient.GetContextSlice(ContextSliceRequest{
        Query: ea.buildSemanticQuery(req.Errors),
        TaskType: "refactoring",
        Level: ea.calculateOptimalLevel(req),
        MaxTokens: ea.calculateAsterBudget(req),
    })
    
    // Get ARF-specific patterns from existing system
    arfContext := ea.baseAnalyzer.buildAnalysisContext(req)
    
    // Merge contexts intelligently
    return ea.mergeContexts(asterContext, arfContext, req)
}
```

#### Phase 2 Acceptance Criteria
- Hybrid context building provides richer semantic analysis
- Token efficiency improves while maintaining or improving analysis quality
- Multi-language support works for at least Java, Go, and Python
- Performance remains within ARF workflow requirements (<3s)

### Phase 3: Advanced Integration & Optimization (Week 2-3)
**Priority**: High  
**Estimated Time**: 5-7 days

#### Phase 3 Tasks
- [ ] **3.1 Advanced Pattern Recognition**
  - Integrate Aster's semantic patterns with ARF-specific transformations
  - Cross-language pattern recognition for polyglot projects
  - Enhanced confidence scoring combining both systems
  - Dependency-aware analysis using Aster's relationship tracking

- [ ] **3.2 Performance & Quality Optimization**
  - Response quality metrics and feedback loops
  - Caching strategies for frequently analyzed code patterns
  - Progressive context enhancement (start minimal, expand as needed)
  - A/B testing framework for comparing CLLM vs enhanced approaches

#### Phase 3 Advanced Features
```go
// Progressive context enhancement
type ProgressiveAnalyzer struct {
    enhancedAnalyzer *EnhancedAnalyzer
    qualityThreshold float64
    maxIterations    int
}

func (pa *ProgressiveAnalyzer) analyzeWithProgression(req *ARFAnalysisRequest) (*ARFAnalysisResponse, error) {
    // Start with minimal context
    context := pa.buildMinimalContext(req)
    response := pa.analyzeContext(context, req)
    
    // Progressively enhance if quality threshold not met
    for i := 0; i < pa.maxIterations && response.Confidence < pa.qualityThreshold; i++ {
        context = pa.enhanceContext(context, response, req)
        response = pa.analyzeContext(context, req)
    }
    
    return response, nil
}
```

#### Phase 3 Acceptance Criteria
- Advanced pattern recognition shows measurable improvement in analysis quality
- Performance optimization maintains <3s ARF response time target
- Quality metrics show improvement over baseline CLLM implementation
- Caching reduces repeated analysis overhead

### Phase 4: Production Integration & Monitoring (Week 3)
**Priority**: Medium  
**Estimated Time**: 3-4 days

#### Phase 4 Tasks
- [ ] **4.1 Production Configuration**
  - Production-ready Aster service configuration
  - Nomad job definitions for Aster service deployment
  - Service discovery and health check integration
  - Configuration management following existing Ploy patterns

- [ ] **4.2 Enhanced Monitoring & Observability**
  - Aster integration metrics and dashboards
  - Performance comparison metrics (baseline vs enhanced)
  - Quality metrics tracking (confidence scores, success rates)
  - Alert configuration for integration health

#### Phase 4 Acceptance Criteria
- Aster service deploys successfully in production environment
- Comprehensive monitoring provides visibility into integration performance
- Alert system notifies of integration issues or degradation
- Documentation complete for operations team

## Configuration Specification

### Aster Integration Configuration
```yaml
# Extend existing CLLM configuration
aster_integration:
  enabled: true
  mcp_endpoint: "http://aster-service:8750"
  connection_timeout: "5s"
  request_timeout: "3s"
  
  # Format selection configuration
  format_selection:
    complexity_thresholds:
      simple: 5      # Use full format
      medium: 8      # Use compact format  
      complex: 15    # Use minimal format
    
    token_thresholds:
      large_result_set: 50   # Use summary format
      
  # Context building configuration
  context_building:
    hybrid_mode: true
    aster_budget_ratio: 0.6    # 60% budget for Aster context
    cllm_budget_ratio: 0.4     # 40% budget for ARF patterns
    merge_strategy: "intelligent"
    
  # Performance configuration
  performance:
    enable_caching: true
    cache_ttl: "1h"
    progressive_enhancement: true
    quality_threshold: 0.85
```

### Service Discovery Configuration
```hcl
# Nomad job for Aster service integration
job "aster-service" {
  datacenters = ["dc1"]
  type        = "service"
  
  group "aster" {
    count = 2
    
    network {
      port "mcp" {
        static = 8750
      }
    }
    
    service {
      name = "aster"
      port = "mcp"
      
      check {
        type     = "http"
        path     = "/health"
        interval = "30s"
        timeout  = "10s"
      }
    }
    
    task "aster-server" {
      driver = "docker"
      
      config {
        image = "aster:latest"
        ports = ["mcp"]
        args = ["serve", "--protocol", "websocket", "--port", "8750"]
      }
      
      resources {
        cpu    = 500
        memory = 1024
      }
    }
  }
}
```

## Testing Strategy

### Integration Testing Approach
- **Unit Tests**: Aster MCP client functionality, format selection logic
- **Integration Tests**: Hybrid context building, end-to-end ARF workflow
- **Performance Tests**: Response time validation, token efficiency measurement
- **Quality Tests**: Analysis quality comparison, confidence score validation

### Testing Phases
1. **Phase 1 Testing**: MCP client connectivity, basic format selection
2. **Phase 2 Testing**: Hybrid context quality, token optimization
3. **Phase 3 Testing**: Advanced patterns, performance optimization
4. **Phase 4 Testing**: Production deployment, monitoring validation

### Success Metrics
- **Performance**: Response time <3s maintained or improved
- **Quality**: Analysis confidence scores improved by >10%
- **Efficiency**: Token usage optimized by 20-40% through smart formatting
- **Coverage**: Multi-language support verified for Java, Go, Python minimum

## Risk Assessment & Mitigation

### Technical Risks
| Risk | Impact | Probability | Mitigation |
|------|---------|-------------|------------|
| Aster service availability | High | Low | Fallback to current CLLM implementation |
| Integration complexity | Medium | Medium | Phased implementation with rollback capability |
| Performance degradation | Medium | Low | Performance monitoring and optimization |
| Quality regression | High | Low | A/B testing and quality metrics |

### Operational Risks
| Risk | Impact | Probability | Mitigation |
|------|---------|-------------|------------|
| Deployment complexity | Medium | Low | Follow existing Ploy deployment patterns |
| Configuration drift | Low | Medium | Configuration management automation |
| Monitoring gaps | Low | Low | Extend existing monitoring infrastructure |

## Success Criteria

### Technical Success
- [x] **Hybrid Integration**: Successfully combine Aster AST analysis with CLLM ARF patterns
- [ ] **Multi-Language Support**: Expand beyond Java to at least 3 additional languages
- [ ] **Token Optimization**: Achieve 20-40% token efficiency improvement
- [ ] **Performance Maintenance**: Maintain <3s ARF response time requirement

### Quality Success
- [ ] **Analysis Quality**: >10% improvement in analysis confidence scores
- [ ] **Success Rate**: Maintain or improve current ARF workflow success rates
- [ ] **Pattern Recognition**: Enhanced pattern detection across multiple languages
- [ ] **Context Richness**: Richer semantic context without token budget explosion

### Operational Success
- [ ] **Seamless Integration**: No disruption to existing ARF workflows
- [ ] **Production Readiness**: Successful deployment with comprehensive monitoring
- [ ] **Documentation**: Complete documentation for development and operations teams
- [ ] **Knowledge Transfer**: Team trained on enhanced system capabilities

## Dependencies & Prerequisites

### External Dependencies
- **Aster Service**: Available and accessible via MCP protocol
- **Existing Infrastructure**: Nomad, Consul, monitoring stack
- **CLLM Phase 2**: Complete and stable before integration begins

### Technical Prerequisites
- **MCP Client Library**: Go-based MCP client implementation
- **Service Discovery**: Aster service registered in Consul
- **Configuration Management**: Aster configuration integrated with existing patterns

### Team Prerequisites
- **Knowledge Transfer**: Team familiar with Aster capabilities and MCP protocol
- **Testing Infrastructure**: Enhanced testing framework for hybrid system
- **Monitoring Setup**: Extended monitoring for integration health

## Next Steps After Completion

### Immediate Post-Integration (Week 4)
1. **Production Validation**: Monitor production metrics and user feedback
2. **Performance Tuning**: Optimize based on real-world usage patterns
3. **Quality Assessment**: Analyze improvement metrics and adjust thresholds

### Future Enhancements (Month 2+)
1. **Extended Language Support**: Add remaining Aster-supported languages
2. **Advanced Semantic Features**: Leverage Aster's full semantic analysis capabilities
3. **Workflow Integration**: Deeper integration with ARF automation frameworks

---

**Phase Owner**: CLLM Development Team  
**Reviewers**: Ploy Platform Team, ARF Integration Team  
**Dependencies**: Aster Project Team for MCP availability  
**Success Metrics Review**: Weekly during active development, monthly post-integration