package manifests

import "errors"

var (
	errManifestNotFound    = errors.New("manifest not found")
	errInvalidManifest     = errors.New("invalid manifest configuration")
	errRegistryUnavailable = errors.New("manifest registry unavailable")
)
