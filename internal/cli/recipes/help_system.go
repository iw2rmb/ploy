package recipes

import (
	"fmt"
)

// NewHelpSystem creates a new help system with all command documentation
func NewHelpSystem() *HelpSystem {
	hs := &HelpSystem{
		commands: make(map[string]CommandHelp),
	}
	hs.initializeCommands()
	return hs
}

// ShowHelp displays help for a specific command or general help
func (hs *HelpSystem) ShowHelp(command string) error {
	if command == "" {
		return hs.showGeneralHelp()
	}

	cmdHelp, exists := hs.commands[command]
	if !exists {
		return NewCLIError(fmt.Sprintf("No help available for command '%s'", command), 1).
			WithSuggestion("Use 'ploy recipe --help' to see all available commands")
	}

	return hs.showCommandHelp(cmdHelp)
}

// GetAvailableHelp returns list of available help topics
func (hs *HelpSystem) GetAvailableHelp() []string {
	var topics []string
	for cmd := range hs.commands {
		topics = append(topics, cmd)
	}
	return topics
}

// initializeCommands initializes all command help information
func (hs *HelpSystem) initializeCommands() {
	hs.initializeBasicCommands()
	hs.initializeAdditionalCommands()
}
