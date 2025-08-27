package parsers

import (
	"fmt"
	"strings"
	"sync"
)

// Parser defines the interface for output parsers
type Parser interface {
	// Name returns the unique name of the parser
	Name() string
	
	// ParseOutput parses the output from a tool and returns issues
	ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error)
	
	// SupportsFormat checks if the parser supports a given output format
	SupportsFormat(format string) bool
}

// ConfigurableParser extends Parser with configuration support
type ConfigurableParser interface {
	Parser
	Configure(options map[string]interface{}) error
	GetOption(key string) interface{}
}

// Registry manages available parsers
type Registry struct {
	mu      sync.RWMutex
	parsers map[string]Parser
}

// NewRegistry creates a new parser registry
func NewRegistry() *Registry {
	return &Registry{
		parsers: make(map[string]Parser),
	}
}

// Register adds a parser to the registry
func (r *Registry) Register(parser Parser) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	name := parser.Name()
	if _, exists := r.parsers[name]; exists {
		return fmt.Errorf("parser '%s' already registered", name)
	}
	
	r.parsers[name] = parser
	return nil
}

// Get retrieves a parser by name
func (r *Registry) Get(name string) (Parser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	parser, exists := r.parsers[name]
	if !exists {
		return nil, fmt.Errorf("parser '%s' not found", name)
	}
	
	return parser, nil
}

// List returns all registered parser names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	names := make([]string, 0, len(r.parsers))
	for name := range r.parsers {
		names = append(names, name)
	}
	return names
}

// AutoDetect attempts to find a suitable parser based on output content
func (r *Registry) AutoDetect(stdout, stderr string, exitCode int) (Parser, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	trimmedOutput := strings.TrimSpace(stdout)
	
	// Check for JSON format
	if strings.HasPrefix(trimmedOutput, "{") || strings.HasPrefix(trimmedOutput, "[") {
		// Look for JSON-capable parsers
		for _, parser := range r.parsers {
			if parser.SupportsFormat("json") || parser.SupportsFormat("application/json") {
				return parser, nil
			}
		}
	}
	
	// Check for XML format - only match XML-capable parsers
	if strings.HasPrefix(trimmedOutput, "<?xml") || 
	   (strings.HasPrefix(trimmedOutput, "<") && strings.Contains(trimmedOutput, ">")) {
		for _, parser := range r.parsers {
			if parser.SupportsFormat("xml") || parser.SupportsFormat("application/xml") {
				return parser, nil
			}
		}
		// If XML detected but no XML parser available, don't fall back to text
		return nil, fmt.Errorf("no XML parser found for XML output")
	}
	
	// For plain text output (not structured data), use text parser
	if trimmedOutput != "" && !strings.HasPrefix(trimmedOutput, "<") {
		for _, parser := range r.parsers {
			if parser.SupportsFormat("text") || parser.SupportsFormat("text/plain") {
				return parser, nil
			}
		}
	}
	
	return nil, fmt.Errorf("no suitable parser found for output")
}

// CompositeParser combines multiple parsers
type CompositeParser struct {
	name    string
	parsers []Parser
}

// NewCompositeParser creates a parser that combines results from multiple parsers
func NewCompositeParser(parsers ...Parser) *CompositeParser {
	names := make([]string, len(parsers))
	for i, p := range parsers {
		names[i] = p.Name()
	}
	
	return &CompositeParser{
		name:    "composite-" + strings.Join(names, "-"),
		parsers: parsers,
	}
}

// Name returns the composite parser name
func (c *CompositeParser) Name() string {
	return c.name
}

// ParseOutput runs all parsers and combines their results
func (c *CompositeParser) ParseOutput(stdout, stderr string, exitCode int) ([]Issue, error) {
	var allIssues []Issue
	var errors []error
	
	for _, parser := range c.parsers {
		issues, err := parser.ParseOutput(stdout, stderr, exitCode)
		if err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", parser.Name(), err))
			continue
		}
		allIssues = append(allIssues, issues...)
	}
	
	// If all parsers failed, return the combined error
	if len(errors) == len(c.parsers) && len(errors) > 0 {
		return nil, fmt.Errorf("all parsers failed: %v", errors)
	}
	
	return allIssues, nil
}

// SupportsFormat checks if any parser supports the format
func (c *CompositeParser) SupportsFormat(format string) bool {
	for _, parser := range c.parsers {
		if parser.SupportsFormat(format) {
			return true
		}
	}
	return false
}

// DefaultRegistry is the global parser registry
var DefaultRegistry = NewRegistry()

// Register adds a parser to the default registry
func Register(parser Parser) error {
	return DefaultRegistry.Register(parser)
}

// Get retrieves a parser from the default registry
func Get(name string) (Parser, error) {
	return DefaultRegistry.Get(name)
}

// List returns all parsers in the default registry
func List() []string {
	return DefaultRegistry.List()
}

// AutoDetect uses the default registry to auto-detect a parser
func AutoDetect(stdout, stderr string, exitCode int) (Parser, error) {
	return DefaultRegistry.AutoDetect(stdout, stderr, exitCode)
}