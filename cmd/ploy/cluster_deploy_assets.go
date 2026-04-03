package main

import "embed"

// clusterDeployRuntimeFS contains runtime deploy assets copied to the deploy dir
// at runtime by `ploy cluster deploy`.
//
//go:embed assets/runtime/**
var clusterDeployRuntimeFS embed.FS

// clusterDeployConfigSchema contains the JSON schema copied to
// $PLOY_CONFIG_HOME/config.schema.json during cluster deploy.
//
//go:embed assets/config.schema.json
var clusterDeployConfigSchema []byte
