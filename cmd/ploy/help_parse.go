package main

import (
	"errors"
	"flag"
)

// parseFlagSet handles shared FlagSet parse behavior for CLI handlers.
// It prints usage via printUsage on parse errors and treats --help/-h as success.
func parseFlagSet(fs *flag.FlagSet, args []string, printUsage func()) error {
	if err := fs.Parse(args); err != nil {
		printUsage()
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	return nil
}
