package main

import "embed"

// clusterDeployRuntimeFS contains runtime deploy assets copied to the deploy dir
// at runtime by `ploy cluster deploy`.
//
//go:embed assets/runtime/**
var clusterDeployRuntimeFS embed.FS
