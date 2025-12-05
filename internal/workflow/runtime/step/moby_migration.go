//go:build moby_migration
// +build moby_migration

// Package step runtime: moby_migration.go
//
// This file imports github.com/moby/moby/client and github.com/moby/moby/api
// modules to keep them in go.mod during the incremental migration from
// github.com/docker/docker to the Engine v29 SDK. The moby_migration build tag
// prevents these imports from affecting normal builds.
//
// Migration plan (see ROADMAP.md "Dependency and SDK selection"):
//  1. This file adds moby modules to go.mod alongside docker/docker.
//  2. Subsequent tasks will migrate container_docker.go and health.go imports.
//  3. Once migration completes, remove this file and the docker/docker dependency.
//
// Build with tag to verify moby imports resolve:
//
//	go build -tags=moby_migration ./internal/workflow/runtime/step
package step

import (
	// Docker Engine v29 SDK modules — imported to retain them in go.mod during
	// migration from github.com/docker/docker. These provide equivalent APIs:
	//
	//   docker/docker/client        → moby/moby/client
	//   docker/docker/api/types/*   → moby/moby/api/types/*
	//
	// The moby client supports the same FromEnv, WithAPIVersionNegotiation
	// options and container lifecycle methods (Create, Start, Wait, Logs, Remove).
	_ "github.com/moby/moby/api/types/container"
	_ "github.com/moby/moby/api/types/image"
	_ "github.com/moby/moby/api/types/mount"
	_ "github.com/moby/moby/client"
)
