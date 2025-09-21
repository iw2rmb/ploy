# GraalVM Native Image Integration for Ploy

## Executive Summary

GraalVM native image compilation for Ploy requires strict detection of behavioral changes and systematic fallback to JVM when correctness cannot be guaranteed. This document defines the integration strategy using Ploy's existing transform/transflow patterns.

## Core Detection Rules

### 1. Non-Guaranteed Behavioral Equivalence (Requires Human Review)

**Detection Triggers:**
- Dynamic proxy creation with runtime-determined interfaces
- Reflection usage with computed class/method names
- Resource loading with dynamic path construction
- Static initializers accessing external resources (files, network, environment)
- Third-party libraries without known GraalVM metadata
- Custom classloaders or runtime bytecode generation

**Action:** Flag for human review with automated transformation suggestions via transflow.

**Implementation:**
```go
type BehavioralRisk struct {
    RequiresReview   bool
    RiskPatterns     []string  // e.g., "Class.forName with variable", "Proxy.newProxyInstance"
    TransformOptions []Transform
    Confidence       float64   // 0.0-1.0 scale
}
```

### 2. Guaranteed Behavioral Incompatibility (Automatic JVM Fallback)

**Detection Triggers:**
- JVMTI usage (debugging/profiling agents)
- Runtime class generation (cglib, javassist without AOT)
- Security managers or custom class loaders
- AWT/Swing UI components
- Incompatible JNI without native library config
- Known incompatible libraries (maintained blacklist)

**Action:** Immediate fallback to JVM with clear explanation.

**Fallback Reasons:**
```yaml
incompatibility_catalog:
  jvmti_detected: "Application uses JVMTI for runtime instrumentation"
  runtime_codegen: "Library {name} generates bytecode at runtime"
  security_manager: "Security manager requires JVM runtime features"
  ui_toolkit: "AWT/Swing components require full JVM"
```

## Native Image Shared Libraries

### Mandatory Shared Library Architecture

**Requirements:**
- Framework cores (Spring, Quarkus, Micronaut) compiled as shared libraries
- Common dependencies deduplicated across deployments
- Layer caching for incremental builds

**Implementation:**
```bash
# Shared library compilation
native-image --shared \
  -H:Name=libspring-core \
  -H:ConfigurationFileDirectories=shared-configs/ \
  --initialize-at-build-time=org.springframework

# Application linking
native-image --link-at-build-time \
  -H:+UseSharedLibrary=libspring-core \
  -H:+UseAuxiliaryEngineCaching
```

**Benefits:**
- 60-80% reduction in build times
- 40-50% reduction in deployment size
- Shared memory pages across containers

## Transform/Transflow Integration

### Reusing Ploy's Transformation Pipeline

**Phase 1: Detection (Existing Lane Detector)**
```go
// Extend existing lane detector
func detectGraalCompatibility(root string) LaneAssignment {
    detector := &GraalDetector{
        BaseDetector: ploy.NewDetector(root),
        Transflow:    ploy.NewTransflow(),
    }
    return detector.Analyze()
}
```

**Phase 2: Transformation (Transflow Reuse)**
```go
// Leverage existing transflow patterns
type GraalTransform struct {
    ploy.BaseTransform
    Recipe string // OpenRewrite recipe
    Risk   BehavioralRisk
}

func (t *GraalTransform) Apply(ctx *TransformContext) error {
    // Use existing OpenRewrite integration
    return t.Transflow.Execute(t.Recipe, ctx)
}
```

**Phase 3: Validation Pipeline**
```yaml
validation_stages:
  - compile_check: Verify transformed code compiles
  - agent_run: Collect reflection metadata via native-image-agent
  - behavior_test: Run smoke tests comparing JVM vs native
  - performance_check: Verify startup/memory improvements
```

## Decision Matrix

| Pattern | Detection Method | Action | Human Review |
|---------|-----------------|--------|--------------|
| Framework-managed reflection | AOT hints present | Auto-transform | No |
| Simple Class.forName("literal") | Static analysis | Add config | No |
| Class.forName(variable) | Data flow analysis | Transform + config | **Yes** |
| Proxy.newProxyInstance | AST detection | Generate proxy config | Conditional |
| ServiceLoader usage | Classpath scan | Include implementations | No |
| MethodHandle usage | Bytecode analysis | **Fallback to JVM** | No |
| JVMTI/Instrumentation | Manifest check | **Fallback to JVM** | No |

## Lane Assignment Strategy

```go
const (
    LaneF = "native-optimized"  // Full native with shared libs
    LaneC = "jvm-standard"      // Traditional JVM deployment
)

func assignLane(compat GraalCompatibility) string {
    if compat.GuaranteedIncompatible {
        return LaneC // Immediate fallback
    }
    if compat.RequiresReview && !compat.HumanApproved {
        return LaneC // Conservative fallback
    }
    if compat.CanCompile && compat.Confidence > 0.8 {
        return LaneF // Native deployment
    }
    return LaneC // Default to JVM
}
```

## Operational Commands

```bash
# Analyze without building
ploy graal analyze <app> --report

# Build with human review checkpoint
ploy push <app> --graal-native --require-approval

# Force shared library usage
ploy push <app> --graal-shared --cache-layer

# Debug mode with agent
ploy push <app> --graal-agent --trace-reflection
```

## Critical Success Metrics

- **Behavioral Correctness**: 100% - No silent failures or behavior changes
- **Detection Accuracy**: 95%+ for known patterns
- **Build Success Rate**: 85% for framework apps (Spring 3+, Quarkus, Micronaut)
- **Shared Library Hit Rate**: 70%+ for common dependencies
- **Fallback Latency**: <10s to switch from native attempt to JVM

## Failure Recovery Protocol

1. **Build Failure**: Log detailed error, fallback to JVM, notify developer
2. **Runtime Failure**: Health check detection, automatic rollback to JVM
3. **Performance Regression**: Monitor metrics, revert if startup/throughput degrades
4. **Review Timeout**: If human review not received within SLA, deploy JVM version

## Implementation Priority

1. **Phase 1**: Framework detection + basic transformations (Spring AOT, Quarkus native)
2. **Phase 2**: Shared library infrastructure for frameworks
3. **Phase 3**: Advanced pattern detection with transflow integration
4. **Phase 4**: Human review workflow with approval gates

## Non-Negotiable Requirements

- Zero behavioral changes without explicit approval
- All transformations must be reversible
- Shared libraries for all framework cores
- Full reuse of Ploy's existing transform/transflow system
- Complete audit trail of all automated decisions