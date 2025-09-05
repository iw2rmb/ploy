---
task: m-setup-github-token-ansible
branch: feature/setup-github-token-ansible
status: pending
created: 2025-09-05
modules: [ansible, vps-config]
---

# Setup GitHub Token via Ansible Playbook on VPS

## Problem/Goal
Create an Ansible playbook to populate GITHUB_TOKEN environment variable on the VPS for transflow testing and future GitHub integration (Stream 3, Phase 2). This ensures the VPS environment has proper GitHub authentication for running transflow workflows that may need GitHub provider functionality.

## Success Criteria
- [ ] Create Ansible playbook for GITHUB_TOKEN environment variable setup
- [ ] Deploy to VPS via existing Ansible infrastructure
- [ ] Verify token is available in VPS environment for ploy user
- [ ] Test transflow integration with GitHub token on VPS
- [ ] Document token setup process in deployment docs

## Context Files
- @ansible playbook infrastructure for VPS configuration
- @internal/git/provider - Git provider interface ready for GitHub implementation
- @CLAUDE.md - VPS setup instructions and deployment patterns

## User Notes
- Global GITHUB_TOKEN env variable provided locally for tests
- Needed for VPS integration testing and future GitHub provider support
- Should follow existing Ansible patterns for environment variable management

## Work Log
- [2025-09-05] Created task for GitHub token Ansible setup