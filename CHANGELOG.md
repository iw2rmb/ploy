# CHANGELOG

## [2025-08-18] - CLI Commands Implementation

### Added
- **Domain Management Commands**
  - `ploy domains add <app> <domain>` - Register custom domain for app
  - `ploy domains list <app>` - List all domains associated with app  
  - `ploy domains remove <app> <domain>` - Remove domain registration

- **Certificate Management Commands**
  - `ploy certs issue <domain>` - Issue TLS certificate via ACME
  - `ploy certs list` - List all managed certificates with expiration dates

- **Debug Commands**
  - `ploy debug shell <app>` - Create debug instance with SSH access
  - `ploy debug shell <app> --lane <A-F>` - Debug with specific lane override

- **Rollback Commands**
  - `ploy rollback <app> <sha>` - Rollback app to previous SHA version

### API Endpoints Added
- `POST /v1/apps/:app/domains` - Add domain to app
- `GET /v1/apps/:app/domains` - List app domains
- `DELETE /v1/apps/:app/domains/:domain` - Remove domain from app
- `POST /v1/certs/issue` - Issue certificate for domain
- `GET /v1/certs` - List all certificates
- `POST /v1/apps/:app/debug` - Create debug instance
- `POST /v1/apps/:app/rollback` - Rollback app to previous version

### Technical Details
- Extended CLI router to handle new command categories
- Added comprehensive error handling and usage messages
- Implemented REST API handlers with proper JSON responses
- Added test scenarios for all new CLI commands (scenarios 79-88)
- All commands follow consistent CLI patterns and conventions

### Testing
-  CLI commands build successfully
-  Proper help messages display for all commands  
-  Error handling works for invalid arguments
-  Commands attempt proper API calls to controller
-  Test scenarios documented in TESTS.md

### Status
**COMPLETED** - Phase 1, Step 1 from PLAN.md: "Complete missing CLI commands: domains add, certs issue, debug shell, rollback"

All essential CLI operations are now implemented, providing users with complete domain, certificate, debugging, and rollback capabilities.