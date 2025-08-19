# Ploy Ansible Playbook Improvements

## Overview
This document outlines the improvements made to the Ansible playbook in `playbooks/main.yml` to address PATH configuration issues and ensure proper tool installation.

## Issues Fixed

### 1. PATH Configuration Conflicts
**Problem**: Multiple conflicting PATH declarations were causing tools to be inaccessible to the ploy user:
- `/etc/environment` had duplicate PATH entries
- User's `.profile` was overriding system PATH
- Go installation was appending `$PATH` incorrectly

**Solution**: 
- Fixed system-wide PATH in `/etc/environment` using `regexp` to replace existing PATH
- Added cleanup tasks to remove conflicting entries from user profiles
- Removed redundant PATH configuration from user's `.bashrc`

### 2. Cosign Installation and Verification
**Problem**: Cosign was being installed but not verified
**Solution**: 
- Added verification task to test cosign installation
- Added debug output to display cosign version during playbook run
- Cosign was already properly configured in the original playbook

### 3. GitHub Environment Variables Conflicts
**Problem**: GitHub credentials were being set in both `/etc/environment` and user's `.bashrc`
**Solution**: 
- Added cleanup task to remove GitHub env vars from `/etc/environment`
- Kept GitHub credentials in user's `.bashrc` only
- Added proper ownership and permissions

## Changes Made

### Path Configuration
```yaml
- name: Configure system PATH with Go
  lineinfile:
    path: /etc/environment
    regexp: '^PATH='
    line: 'PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin"'
    create: true
```

### Cleanup Tasks
```yaml
- name: Clean up conflicting environment variables from /etc/environment
  lineinfile:
    path: /etc/environment
    regexp: '^GITHUB_PLOY_DEV_(USERNAME|PAT)='
    state: absent

- name: Clean up conflicting PATH entries from user profile
  lineinfile:
    path: /home/ploy/.profile
    regexp: '^export PATH="/usr/bin:/bin:\$PATH"'
    state: absent
```

### Cosign Verification
```yaml
- name: Verify Cosign installation
  command: /usr/local/bin/cosign version
  register: cosign_version
  changed_when: false

- name: Display Cosign version
  debug:
    msg: "Cosign installed successfully: {{ cosign_version.stdout_lines[0] if cosign_version.stdout_lines else 'Version info not available' }}"
```

## Tools Installed and Verified

### Build Tools
- **KraftKit**: v0.7.1 - Unikernel build tool
- **Cosign**: v2.2.2 - Container signing and verification
- **Syft**: v0.100.0 - SBOM generation
- **Grype**: v0.74.0 - Vulnerability scanning

### Development Tools
- **Go**: v1.22.0 - Programming language
- **Node.js**: v18.x - JavaScript runtime
- **Java**: OpenJDK 17 - Java development
- **Python**: 3.x with pip and venv

### Infrastructure
- **Docker**: Latest CE with Compose
- **MinIO**: Object storage
- **Firewall**: UFW configured

## PATH Resolution
The final PATH configuration ensures all tools are accessible:
```
/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin
```

## Testing
- Syntax check: ✅ `ansible-playbook --syntax-check playbooks/main.yml`
- Tool verification: All tools accessible via proper PATH
- User permissions: ploy user has access to all required tools

## Usage
Run the updated playbook:
```bash
cd iac/dev
ansible-playbook -i inventory/hosts.yml playbooks/main.yml -e target_host=$TARGET_HOST
```

## Benefits
1. **Reliability**: Eliminates PATH-related tool access issues
2. **Maintainability**: Single source of truth for PATH configuration
3. **Verification**: Confirms tools are properly installed during deployment
4. **Security**: Proper separation of system and user environment variables
