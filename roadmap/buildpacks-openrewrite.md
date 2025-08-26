# Cloud Native Buildpacks Integration Roadmap

## Executive Summary

This roadmap outlines the integration of Cloud Native Buildpacks (CNB) into Ploy, focusing on two key objectives:
1. **Optional buildpacks support for Lane E** - Providing developers with a zero-configuration build option while preserving current optimizations
2. **OpenRewrite service migration** - Simplifying the OpenRewrite service deployment by leveraging buildpacks for dependency management

**Timeline**: 5-8 weeks total
**Priority**: Medium (enhances developer experience without disrupting core functionality)
**Risk**: Low (additive feature, preserves existing functionality)

## Value Proposition

### For Lane E
- **Developer Experience**: Zero-configuration builds for apps without Dockerfiles
- **Security**: Automated base image updates via `pack rebase`
- **Compatibility**: Industry-standard CNB support attracts more users
- **Flexibility**: Developers choose between optimization (current) and convenience (buildpacks)

### For OpenRewrite Service
- **Simplified Deployment**: Eliminate custom Docker builds
- **Dependency Management**: Automatic JVM, Maven, Git installation
- **Maintenance**: Leverage upstream buildpack updates
- **Consistency**: Deploy like any other Ploy app

## Architecture Overview

```
Current Lane E:
  src.tar → Dockerfile/Jib → OCI Image → Deploy

With Buildpacks:
  src.tar → Pack/Lifecycle → OCI Image → Deploy
              ↑
        Builder Selection
        (Paketo/Heroku)
```

## Phase 1: Lane E Optional Buildpacks (Weeks 1-3)

### Objectives
- Add buildpack support as opt-in feature
- Preserve existing Docker/Jib optimizations
- Maintain backward compatibility

### Implementation Tasks

#### Week 1: Core Integration
- [ ] Install Pack CLI binary on controller
- [ ] Add `controller/builders/buildpack.go`
- [ ] Implement builder selection logic
- [ ] Add `--buildpack` flag to CLI

#### Week 2: Smart Detection
- [ ] Extend `tools/lane-pick` for buildpack detection
- [ ] Create Lane E sub-modes (E-docker, E-jib, E-buildpack)
- [ ] Add builder selection heuristics
- [ ] Implement fallback logic

#### Week 3: Testing & Documentation
- [ ] Integration tests with sample apps
- [ ] Performance benchmarking vs current approach
- [ ] Update CLI documentation
- [ ] Add buildpack examples to `tests/apps/`

### Technical Design

```go
// controller/builders/buildpack.go
package builders

import (
    "github.com/buildpacks/pack"
    "github.com/buildpacks/pack/config"
)

type BuildpackBuilder struct {
    client       *pack.Client
    builderMap   map[string]string
}

func NewBuildpackBuilder() (*BuildpackBuilder, error) {
    client, err := pack.NewClient(
        pack.WithLogger(log.New()),
        pack.WithDockerClient(dockerClient),
    )
    
    return &BuildpackBuilder{
        client: client,
        builderMap: map[string]string{
            "java":   "paketobuildpacks/builder:base",
            "node":   "heroku/builder:24",
            "python": "heroku/builder:24",
            "go":     "paketobuildpacks/builder:tiny",
            "default": "paketobuildpacks/builder:full",
        },
    }, err
}

func (b *BuildpackBuilder) Build(req BuildpackRequest) (string, error) {
    imageName := fmt.Sprintf("harbor.local/ploy/%s:%s", req.App, req.SHA)
    
    builder := b.selectBuilder(req.SrcDir)
    if req.Builder != "auto" {
        builder = req.Builder
    }
    
    err := b.client.Build(context.Background(), pack.BuildOptions{
        Image:        imageName,
        Builder:      builder,
        AppPath:      req.SrcDir,
        ClearCache:   req.ClearCache,
        TrustBuilder: true,
        Env:          req.EnvVars,
        Publish:      true,
    })
    
    return imageName, err
}
```

### Lane Detection Updates

```go
// tools/lane-pick/main.go additions
func detectLaneE(root string) (string, []string) {
    reasons := []string{}
    
    // Existing Jib detection - highest priority
    if hasJibPlugin(root) {
        reasons = append(reasons, "Jib plugin detected - optimal containerless build")
        return "E-jib", reasons
    }
    
    // Existing Dockerfile - respect custom optimizations
    if exists(filepath.Join(root, "Dockerfile")) {
        dockerfile := readFile(filepath.Join(root, "Dockerfile"))
        if isOptimizedDockerfile(dockerfile) {
            reasons = append(reasons, "Optimized Dockerfile found")
            return "E-docker", reasons
        }
    }
    
    // New: Buildpack compatibility
    if isBuildpackCompatible(root) {
        reasons = append(reasons, "Buildpack-compatible project structure")
        return "E-buildpack", reasons
    }
    
    return "E-docker", reasons
}
```

## Phase 2: OpenRewrite Service Migration (Weeks 4-5)

### Objectives
- Migrate OpenRewrite service to buildpack-based deployment
- Handle Go service with Java/Maven dependencies
- Simplify deployment to standard `ploy push`

### Current Architecture
```dockerfile
# Current: Multi-stage Docker build
FROM golang:1.21 AS builder
# Build Go service

FROM maven:3.9-eclipse-temurin-17
# Install dependencies
# Copy Go binary
```

### New Architecture
```yaml
# project.toml for OpenRewrite service
[[build.buildpacks]]
uri = "paketo-buildpacks/go"

[[build.buildpacks]]
uri = "paketo-buildpacks/maven"

[[build.buildpacks]]
uri = "paketo-buildpacks/git"

[build.env]
BP_JVM_VERSION = "17"
BP_MAVEN_VERSION = "3.9"
```

### Implementation Tasks

#### Week 4: Service Preparation
- [ ] Create `project.toml` for OpenRewrite service
- [ ] Add buildpack detection markers
- [ ] Configure composite buildpack order
- [ ] Test local builds with pack CLI

#### Week 5: Deployment Migration
- [ ] Deploy OpenRewrite via `ploy push --buildpack`
- [ ] Validate Maven/Git functionality
- [ ] Performance testing vs current deployment
- [ ] Update deployment documentation

### Service Structure
```
openrewrite-service/
├── project.toml          # Buildpack configuration
├── go.mod               # Go dependencies (triggers Go buildpack)
├── .java-version        # Triggers JVM installation (content: "17")
├── .maven-version       # Triggers Maven installation (content: "3.9")
├── cmd/
│   └── server/
│       └── main.go     # Go service entry point
└── scripts/
    └── prepare.sh       # Pre-build script for OpenRewrite artifacts
```

## Phase 3: Custom Buildpack for Go+Java Services (Weeks 6-8)

### Objectives
- Create reusable buildpack for Go services needing JVM
- Package as "ploy/go-java" buildpack
- Enable other similar services to use same pattern

### Buildpack Structure
```
ploy-go-java-buildpack/
├── buildpack.toml
├── bin/
│   ├── detect          # Detects Go + Java markers
│   └── build           # Installs JVM, Maven, builds Go
└── layers/
    ├── jvm/            # Cached JVM installation
    ├── maven/          # Cached Maven installation
    └── openrewrite/    # Cached OpenRewrite artifacts
```

### Implementation

```bash
#!/usr/bin/env bash
# bin/detect
if [[ -f "$1/go.mod" ]] && [[ -f "$1/.java-version" ]]; then
    echo "Go + Java Composite"
    exit 0
fi
exit 1
```

```bash
#!/usr/bin/env bash
# bin/build
layers_dir=$1
platform_dir=$2
build_plan=$3

# Install JVM
if [[ ! -d "$layers_dir/jvm" ]]; then
    echo "Installing JVM 17..."
    mkdir -p "$layers_dir/jvm"
    curl -L "https://adoptium.net/..." | tar xz -C "$layers_dir/jvm"
    echo "launch = true" > "$layers_dir/jvm.toml"
fi

# Install Maven
if [[ ! -d "$layers_dir/maven" ]]; then
    echo "Installing Maven 3.9..."
    mkdir -p "$layers_dir/maven"
    curl -L "https://archive.apache.org/..." | tar xz -C "$layers_dir/maven"
    echo "build = true" > "$layers_dir/maven.toml"
fi

# Pre-cache OpenRewrite artifacts
echo "Pre-caching OpenRewrite dependencies..."
cat > /tmp/pom.xml <<EOF
<project>
    <dependencies>
        <dependency>
            <groupId>org.openrewrite</groupId>
            <artifactId>rewrite-maven-plugin</artifactId>
            <version>5.34.0</version>
        </dependency>
    </dependencies>
</project>
EOF
mvn -f /tmp/pom.xml dependency:go-offline

# Build Go application
echo "Building Go application..."
go build -o /layers/launch/app ./cmd/server
```

## Success Metrics

### Phase 1 (Lane E)
- [ ] 90% of apps without Dockerfiles build successfully
- [ ] Build time within 2x of current approach
- [ ] Image size within 3x of current approach
- [ ] Zero regression in existing Lane E builds

### Phase 2 (OpenRewrite)
- [ ] OpenRewrite service deploys via `ploy push`
- [ ] All transformation tests pass
- [ ] Deployment time < 2 minutes
- [ ] Service starts successfully with all dependencies

### Phase 3 (Custom Buildpack)
- [ ] Buildpack published to registry
- [ ] Successfully builds Go+Java services
- [ ] Layer caching reduces rebuild time by 50%
- [ ] Reusable by other services

## Risk Mitigation

### Performance Concerns
- **Risk**: Buildpack images 2-3x larger than current
- **Mitigation**: Keep as optional, document trade-offs
- **Fallback**: Maintain current optimized paths

### Compatibility Issues
- **Risk**: Some apps may not work with buildpacks
- **Mitigation**: Comprehensive testing, gradual rollout
- **Fallback**: Automatic fallback to Docker build

### Maintenance Burden
- **Risk**: Another build system to maintain
- **Mitigation**: Use upstream builders, minimal custom code
- **Fallback**: Can deprecate if adoption low

## Configuration Examples

### CLI Usage
```bash
# Opt-in to buildpacks
ploy push --buildpack

# Specify builder
ploy push --buildpack --builder heroku/builder:24

# Force specific lane mode
ploy push --lane E-buildpack
```

### App Configuration
```yaml
# .ploy.yaml in app repository
build:
  type: buildpack
  builder: paketobuildpacks/builder:base
  env:
    BP_JVM_VERSION: "17"
    BP_MAVEN_BUILD_ARGUMENTS: "-DskipTests"
```

## Rollout Strategy

1. **Alpha** (Week 3): Internal testing with test apps
2. **Beta** (Week 5): Opt-in for early adopters via `--buildpack` flag
3. **GA** (Week 8): Full documentation, production ready
4. **Future**: Consider buildpacks for other lanes if successful

## Future Enhancements

### Near Term (3-6 months)
- [ ] Buildpack builder caching on controller
- [ ] Custom Ploy builder with common dependencies
- [ ] Build-time SBOM integration
- [ ] Vulnerability scanning via buildpack metadata

### Long Term (6-12 months)
- [ ] Explore buildpacks for Lane B (Node/Python)
- [ ] Native buildpack support in lane-pick
- [ ] Buildpack-to-unikernel pipeline (experimental)
- [ ] Multi-architecture builds (ARM64 support)

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| TBD | Use Pack library over lifecycle | Faster implementation, proven stability |
| TBD | Paketo for Java, Heroku for others | Best language support per builder |
| TBD | Keep as optional feature | Preserves optimization philosophy |
| TBD | Start with OpenRewrite | Clear value prop, controlled testing |

## References

- [Cloud Native Buildpacks Spec](https://github.com/buildpacks/spec)
- [Pack CLI Documentation](https://buildpacks.io/docs/tools/pack/)
- [Paketo Buildpacks](https://paketo.io/)
- [Heroku Buildpacks](https://github.com/heroku/buildpacks)
- [Buildpack Author Guide](https://buildpacks.io/docs/buildpack-author-guide/)

## Appendix: Builder Comparison

| Builder | Image Size | Build Speed | Language Support | Caching | Production Ready |
|---------|------------|-------------|------------------|---------|------------------|
| Paketo Base | 822MB | Fast | Java, Node, Go, .NET | Excellent | Yes |
| Paketo Tiny | 95MB | Fastest | Go, Rust | Good | Yes |
| Paketo Full | 1.3GB | Slower | Everything | Excellent | Yes |
| Heroku 24 | 663MB | Fast | Most popular | Good | Yes |
| Google | 884MB | Fast | GCP optimized | Good | Yes |

## Summary

This roadmap provides a pragmatic path to adding buildpack support to Ploy while maintaining its core optimization philosophy. By making buildpacks optional and focusing on clear use cases (OpenRewrite service), we can enhance developer experience without compromising performance for users who need it.

**Key Principles**:
1. **Optional, not mandatory** - Developers choose optimization vs convenience
2. **Preserve existing optimizations** - Jib and custom Dockerfiles remain preferred
3. **Start small** - Prove value with OpenRewrite before broader adoption
4. **Measure everything** - Data-driven decisions on expansion

The implementation is designed to be reversible if buildpacks don't provide sufficient value, ensuring low risk to the project.