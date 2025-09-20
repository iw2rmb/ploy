# VPS Integration Testing Guide

## Purpose
VPS production environment integration testing infrastructure providing comprehensive validation of mods workflows, KB learning systems, and distributed service coordination in the real production environment (`$TARGET_HOST`). **MVP COMPLETE**: All production services validated and operational.

## Narrative Summary
The VPS integration testing module provides production environment validation for the complete mods system including distributed job orchestration, KB learning integration, and real-world service interactions. Framework validates service topology (Consul, Nomad, SeaweedFS), performance baselines, KB storage operations, and complete mods workflow execution in production environment.

**✅ Production Environment Validation** - Complete testing infrastructure for validating mods workflows in production VPS environment with real service interactions, distributed coordination, and performance benchmarking. System provides service health checks, topology validation, performance baselines, and comprehensive workflow testing with actual GitLab operations.

Core validation workflow: VPS service health → distributed service topology → KB storage performance → mods CLI availability → production workflow execution → GitLab integration → comprehensive result validation. Framework ensures production readiness with real-world service interactions and distributed system coordination.

## Key Files
- [`vps_client.go#L1`](./vps_client.go#L1) — VPS client implementation with SSH operations and service health checking
- [`vps_client.go#L15`](./vps_client.go#L15) — `VPSClient` structure with SSH connection management and command execution
- [`vps_client.go#L42`](./vps_client.go#L42) — Service health checking with Consul, Nomad, and SeaweedFS validation
- [`vps_client.go#L82`](./vps_client.go#L82) — Command execution framework with proper user context (ploy user)
- [`vps_integration_test.go#L1`](./vps_integration_test.go#L1) — Core VPS service validation and readiness testing
- [`vps_integration_test.go#L12`](./vps_integration_test.go#L12) — `TestVPSEnvironmentReadiness` with service health validation
- [`vps_integration_test.go#L42`](./vps_integration_test.go#L42) — `TestVPSKBStorageSetup` with SeaweedFS namespace validation
- [`production_validation_test.go#L1`](./production_validation_test.go#L1) — Production-grade validation testing with performance baselines
- [`production_validation_test.go#L13`](./production_validation_test.go#L13) — `TestVPSProductionReadiness` with service topology validation
- [`production_validation_test.go#L38`](./production_validation_test.go#L38) — Performance baseline testing with KB storage response time validation

### Production Service Validation
- Complete service health checking for Consul, Nomad, SeaweedFS-master, SeaweedFS-filer
- Distributed service topology validation with leader election and cluster coordination
- KB storage namespace setup and operational validation
- Mods CLI availability and command execution validation
- Performance baseline testing with response time measurement
- Production user context validation (ploy user with proper permissions)

## Integration Points

### Consumes (✅ Production Operational)
- **✅ VPS Environment**: Production server (`$TARGET_HOST`) with complete service stack deployment
- **✅ SSH Access**: Remote command execution with proper authentication and user context
- **✅ Consul Cluster**: Distributed coordination service with member validation and health checking
- **✅ Nomad Orchestration**: Job execution platform with leader election and cluster status validation
- **✅ SeaweedFS Storage**: Distributed storage backend with master/filer topology and performance validation
- **✅ Ploy CLI**: Production mods CLI installation with command availability validation
- **✅ System Services**: Service management and health monitoring with systemctl integration

### Provides (✅ MVP Complete)
- **✅ Production Validation**: Complete VPS environment readiness and service health validation
- **✅ Service Topology Testing**: Distributed service coordination and leader election validation
- **✅ Performance Baselines**: KB storage response time measurement and performance validation
- **✅ CLI Availability Testing**: Mods command execution and functionality validation
- **✅ Storage Setup Validation**: KB namespace creation and SeaweedFS operational testing
- **✅ User Context Validation**: Ploy user permissions and service access validation
- **✅ Integration Readiness**: Complete production environment preparation and validation
- **✅ Real-world Testing**: Actual service interactions with production performance validation

## Configuration

Environment variables:
- `TARGET_HOST=<production host>` - VPS production server for integration testing
- SSH authentication via SSH keys (production deployment pattern)
- Service endpoints validated: Consul (8500), Nomad (4646), SeaweedFS (9333, 8888)

VPS Service Stack (✅ Production Operational):
- **Consul**: Distributed coordination and service discovery (port 8500)
- **Nomad**: Job orchestration and container management (port 4646) 
- **SeaweedFS Master**: Storage coordination and metadata management (port 9333)
- **SeaweedFS Filer**: File system interface and HTTP API (port 8888)
- **Ploy CLI**: Mods command execution with KB integration (/opt/ploy/bin/ploy)

User Context:
- Tests execute as `ploy` user with proper service permissions
- Service access validated with appropriate file system and network permissions
- Production security model with limited privilege access

## Key Patterns

- VPS client abstraction with SSH command execution and error handling (see [`vps_client.go#L15`](./vps_client.go#L15))
- Service health validation with timeout management and comprehensive status checking (see [`vps_client.go#L42`](./vps_client.go#L42))
- Production user context with proper privilege escalation and permission validation (see [`vps_integration_test.go#L32`](./vps_integration_test.go#L32))
- Performance baseline testing with response time measurement and acceptance criteria (see [`production_validation_test.go#L39`](./production_validation_test.go#L39))
- Service topology validation with leader election and cluster coordination testing (see [`production_validation_test.go#L21`](./production_validation_test.go#L21))
- Comprehensive error handling with detailed test output and debugging information
- Production readiness validation with real service interactions and distributed system coordination
- Test skip patterns for environments without VPS access (graceful degradation)

## Production Status

**✅ MVP COMPLETE - All VPS Integration Components Operational:**
- **Service Stack**: Complete Consul + Nomad + SeaweedFS deployment validated and operational
- **VPS Environment**: Production server (`$TARGET_HOST`) with full service topology deployment
- **CLI Integration**: Ploy mods commands available and functional in production environment
- **KB Storage**: SeaweedFS backend operational with namespace setup and performance validation
- **Service Health**: All critical services (consul, nomad, seaweedfs-master, seaweedfs-filer) healthy
- **Performance Validation**: Response time baselines established and acceptance criteria met
- **User Security**: Ploy user context operational with proper service permissions
- **Integration Testing**: Complete production environment validation with real service interactions

**Production Service Status:**
- ✅ Consul cluster: Leader election and member coordination operational
- ✅ Nomad orchestration: Job submission and execution platform ready
- ✅ SeaweedFS storage: Master/filer topology with HTTP API operational  
- ✅ Mods CLI: Production installation with KB integration ready
- ✅ Network connectivity: Service endpoint validation and response time testing passed
- ✅ Performance baselines: KB storage <2s response time validated
- ✅ Security model: Ploy user permissions and service access validated

## Related Documentation
- [`../e2e/README.md`](../e2e/README.md) - E2E testing framework with VPS integration (✅ operational)
- [`../../internal/mods/README.md`](../../internal/mods/README.md) - Mods CLI with VPS deployment support (MVP complete)
- Root `Makefile` - VPS integration test targets and TARGET_HOST configuration
- Production deployment documentation for service stack configuration
