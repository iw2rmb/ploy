package config

// Option configures the Service during construction.
type Option func(*Service) error

// WithDefaults registers a static defaults source.
func WithDefaults(defaults *Config) Option {
    return func(s *Service) error {
        if s.loader == nil {
            s.loader = &CompositeLoader{}
        }
        s.loader.AddSource(&defaultsSource{defaults: defaults, priority: 10})
        return nil
    }
}

// WithEnvironment registers an environment source using the given prefix, e.g. "PLOY_".
// This minimal slice supports APP_NAME -> app.name.
func WithEnvironment(prefix string) Option {
    return func(s *Service) error {
        if s.loader == nil {
            s.loader = &CompositeLoader{}
        }
        s.loader.AddSource(&envSource{prefix: prefix, priority: 100})
        return nil
    }
}

// WithFile loads configuration from a YAML file at the given path.
func WithFile(path string) Option {
    return func(s *Service) error {
        if s.loader == nil {
            s.loader = &CompositeLoader{}
        }
        s.loader.AddSource(&fileSource{path: path, priority: 50})
        return nil
    }
}
