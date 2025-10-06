package lanescatalog

import "embed"

// Catalog embeds the workstation lane definitions bundled with Ploy.
//
//go:embed *.toml
var Catalog embed.FS
