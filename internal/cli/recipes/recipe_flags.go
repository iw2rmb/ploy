package recipes

// parseCommonFlags parses common command flags
func parseCommonFlags(args []string) CommandFlags {
	flags := CommandFlags{
		OutputFormat: "table", // Default output format
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run", "-n":
			flags.DryRun = true
		case "--force", "-f":
			flags.Force = true
		case "--verbose", "-v":
			flags.Verbose = true
		case "--strict", "-s":
			flags.Strict = true
		case "--interactive", "-i":
			flags.Interactive = true
		case "--output", "-o":
			if i+1 < len(args) {
				flags.OutputFormat = args[i+1]
				i++
			}
		case "--name":
			if i+1 < len(args) {
				flags.Name = args[i+1]
				i++
			}
		case "--template", "-t":
			if i+1 < len(args) {
				flags.Template = args[i+1]
				i++
			}
		case "--file":
			if i+1 < len(args) {
				flags.OutputFile = args[i+1]
				i++
			}
		}
	}
	return flags
}

// parseUploadFlags creates UploadFlags from common flags (legacy support)
func parseUploadFlags(args []string) UploadFlags {
	flags := parseCommonFlags(args)
	return UploadFlags{
		DryRun: flags.DryRun,
		Force:  flags.Force,
		Name:   flags.Name,
	}
}

// parseVerboseFlag extracts verbose flag from args (legacy support)
func parseVerboseFlag(args []string) bool {
	flags := parseCommonFlags(args)
	return flags.Verbose
}

// parseForceFlag extracts force flag from args (legacy support)
func parseForceFlag(args []string) bool {
	flags := parseCommonFlags(args)
	return flags.Force
}

// parseStrictFlag extracts strict flag from args (legacy support)
func parseStrictFlag(args []string) bool {
	flags := parseCommonFlags(args)
	return flags.Strict
}

// parseOutputFile extracts output file from args (legacy support)
func parseOutputFile(args []string) string {
	flags := parseCommonFlags(args)
	return flags.OutputFile
}
