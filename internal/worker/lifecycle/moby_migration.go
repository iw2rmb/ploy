//go:build moby_migration
// +build moby_migration

// Package lifecycle: moby_migration.go
//
// This file imports github.com/moby/moby/client and github.com/moby/moby/api
// modules to keep them in go.mod during the incremental migration from
// github.com/docker/docker to the Engine v29 SDK. The moby_migration build tag
// prevents these imports from affecting normal builds.
//
// Migration plan (see ROADMAP.md "Dependency and SDK selection"):
//  1. This file adds moby modules to go.mod alongside docker/docker.
//  2. Subsequent tasks will migrate health.go imports for Ping/Info types.
//  3. Once migration completes, remove this file and the docker/docker dependency.
//
// Build with tag to verify moby imports resolve:
//
//	go build -tags=moby_migration ./internal/worker/lifecycle
package lifecycle

import (
	// Docker Engine v29 SDK modules — imported to retain them in go.mod during
	// migration from github.com/docker/docker. These provide equivalent APIs:
	//
	//   docker/docker/api/types        → moby/moby/api/types (Ping response)
	//   docker/docker/api/types/system → moby/moby/api/types/system (Info response)
	//   docker/docker/client           → moby/moby/client
	//
	// The moby client supports the same Ping() and Info() methods used by
	// DockerChecker for health reporting.
	_ "github.com/moby/moby/api/types"
	_ "github.com/moby/moby/api/types/system"
	_ "github.com/moby/moby/client"
)
