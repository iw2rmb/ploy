package storage

// PutOptionsResolved is an exported, provider-friendly view of Put options.
type PutOptionsResolved struct {
	ContentType  string
	Metadata     map[string]string
	CacheControl string
}

// ResolvePutOptions applies functional PutOption values and returns a resolved struct.
func ResolvePutOptions(opts ...PutOption) PutOptionsResolved {
	var o putOptions
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return PutOptionsResolved(o)
}
