package arf

// HelpSystem provides comprehensive help and usage information
type HelpSystem struct {
	commands map[string]CommandHelp
}

// CommandHelp represents help information for a specific command
type CommandHelp struct {
	Name        string
	Synopsis    string
	Description string
	Usage       string
	Flags       []FlagHelp
	Examples    []ExampleHelp
	SeeAlso     []string
}

// FlagHelp represents help for a command flag
type FlagHelp struct {
	Long        string
	Short       string
	Description string
	Default     string
	Required    bool
}

// ExampleHelp represents a usage example
type ExampleHelp struct {
	Title       string
	Command     string
	Description string
}
