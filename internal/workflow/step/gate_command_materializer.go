package step

// EnvMaterializer produces a shell preamble that materializes a special env
// key's value into runtime-specific behavior beyond plain env passthrough.
// Keys without a materializer use plain env passthrough (no preamble).
type EnvMaterializer func() string

// envMaterializerEntry pairs an env key with its materializer. The slice
// ordering is the concatenation order in envMaterializerPreamble.
type envMaterializerEntry struct {
	key          string
	materializer EnvMaterializer
}

// materializers is the single registration point for all special env key
// materializers. Add new entries here to extend the mechanism.
var materializers []envMaterializerEntry

// MaterializerForKey returns the materializer for a special env key, or nil
// for keys that use plain env passthrough.
func MaterializerForKey(key string) EnvMaterializer {
	for _, e := range materializers {
		if e.key == key {
			return e.materializer
		}
	}
	return nil
}

// envMaterializerPreamble returns the combined shell preamble for all
// registered env materializers. This is prepended to gate container commands.
func envMaterializerPreamble() string {
	var preamble string
	for _, e := range materializers {
		preamble += e.materializer()
	}
	return preamble
}

