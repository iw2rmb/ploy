# Phase ARF-5.2: CLI Integration & User Interface

**Status**: ⏳ **PARTIALLY IMPLEMENTED**  
**Dependencies**: Phase ARF-5.1 (Recipe Data Model & Storage) ✅  
  
**Priority**: HIGH  

## Overview

Phase ARF-5.2 builds comprehensive CLI functionality for recipe management, integrating user-friendly commands with the Recipe storage system from Phase 5.1. This phase enables users to upload, manage, discover, and execute custom transformation recipes through intuitive `ploy arf recipe` commands.

## Objectives

1. ⏳ **Recipe Management Commands**: Complete CRUD operations for user recipes
2. ⏳ **Recipe Discovery Interface**: Search, filter, and browse available recipes  
3. ✅ **Benchmark Integration**: Seamless recipe execution through existing benchmark system
4. ⏳ **Import/Export Functionality**: Recipe sharing and backup capabilities
5. ⏳ **User Experience Optimization**: Intuitive workflows with comprehensive help and validation

## Technical Specifications

### CLI Command Structure

```bash
# Recipe Management Commands
ploy arf recipe upload <recipe-file>         # Upload new recipe
ploy arf recipe update <recipe-id> <file>    # Update existing recipe  
ploy arf recipe delete <recipe-id>           # Delete recipe
ploy arf recipe list [--filter]              # List available recipes
ploy arf recipe show <recipe-id>             # Display recipe details
ploy arf recipe download <recipe-id>         # Download recipe to file

# Recipe Discovery Commands  
ploy arf recipe search <query>               # Full-text recipe search
ploy arf recipe filter --tag <tag>           # Filter by tags
ploy arf recipe filter --language <lang>     # Filter by programming language
ploy arf recipe filter --category <cat>      # Filter by category

# Recipe Execution Commands
// ARF recipe execution removed; use Mods
ploy arf recipe compose <recipe-ids...>      # Chain multiple recipes
ploy arf recipe validate <recipe-file>       # Validate recipe without execution

# Recipe Import/Export Commands
ploy arf recipe export --filter <criteria>   # Export recipes to archive
ploy arf recipe import <archive-file>        # Import recipe archive
ploy arf recipe sync --source <url>          # Sync from remote repository

# Recipe Development Commands
ploy arf recipe init <template>               # Initialize new recipe from template
ploy arf recipe test <recipe-file> --repo    # Test recipe against repository
ploy arf recipe lint <recipe-file>           # Lint recipe for best practices
```

### CLI Implementation Architecture

```go
// RecipeCommand implements recipe management CLI interface
type RecipeCommand struct {
    storage      RecipeStorage      // From Phase 5.1
    indexStore   RecipeIndexStore   // From Phase 5.1
    validator    *RecipeValidator   // From Phase 5.1
    benchmarkSvc *BenchmarkService  // Existing benchmark system
    outputFormat OutputFormat       // JSON, YAML, table, etc.
}

// Command registration in cmd/ploy/main.go
func registerARFRecipeCommands(app *cli.App) {
    recipeCmd := &cli.Command{
        Name:  "recipe",
        Usage: "Manage ARF transformation recipes",
        Subcommands: []*cli.Command{
            uploadRecipeCommand(),
            updateRecipeCommand(),
            deleteRecipeCommand(),
            listRecipesCommand(),
            showRecipeCommand(),
            searchRecipesCommand(),
            runRecipeCommand(),
            validateRecipeCommand(),
            // ... additional commands
        },
    }
    
    // Add to existing ARF command tree
    arfCommand.Subcommands = append(arfCommand.Subcommands, recipeCmd)
}
```

### Recipe Upload & Management Commands

```go
// uploadRecipeCommand handles recipe upload with validation
func uploadRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:      "upload",
        Aliases:   []string{"u"},
        Usage:     "Upload a new transformation recipe",
        ArgsUsage: "<recipe-file>",
        Flags: []cli.Flag{
            &cli.BoolFlag{
                Name:    "dry-run",
                Aliases: []string{"n"},
                Usage:   "Validate recipe without uploading",
            },
            &cli.BoolFlag{
                Name:  "force",
                Usage: "Override validation warnings",
            },
            &cli.StringFlag{
                Name:  "name",
                Usage: "Override recipe name",
            },
        },
        Action: func(c *cli.Context) error {
            return handleRecipeUpload(c)
        },
    }
}

func handleRecipeUpload(c *cli.Context) error {
    recipePath := c.Args().First()
    if recipePath == "" {
        return cli.NewExitError("Recipe file path is required", 1)
    }
    
    // Load and parse recipe
    recipe, err := loadRecipeFromFile(recipePath)
    if err != nil {
        return cli.NewExitError(fmt.Sprintf("Failed to load recipe: %v", err), 1)
    }
    
    // Override name if specified
    if name := c.String("name"); name != "" {
        recipe.Metadata.Name = name
    }
    
    // Validate recipe
    if err := validateRecipe(recipe); err != nil {
        if !c.Bool("force") {
            return cli.NewExitError(fmt.Sprintf("Recipe validation failed: %v", err), 1)
        }
        fmt.Printf("Warning: %v (continuing due to --force)\n", err)
    }
    
    // Dry run mode
    if c.Bool("dry-run") {
        fmt.Printf("Recipe '%s' is valid and ready for upload\n", recipe.Metadata.Name)
        return nil
    }
    
    // Upload recipe
    if err := uploadRecipe(recipe); err != nil {
        return cli.NewExitError(fmt.Sprintf("Upload failed: %v", err), 1)
    }
    
    fmt.Printf("Recipe '%s' uploaded successfully (ID: %s)\n", 
        recipe.Metadata.Name, recipe.ID)
    return nil
}
```

### Recipe Discovery & Search Commands

```go
// listRecipesCommand provides flexible recipe listing
func listRecipesCommand() *cli.Command {
    return &cli.Command{
        Name:    "list",
        Aliases: []string{"ls"},
        Usage:   "List available recipes with optional filtering",
        Flags: []cli.Flag{
            &cli.StringSliceFlag{
                Name:    "tag",
                Aliases: []string{"t"},
                Usage:   "Filter by tags",
            },
            &cli.StringFlag{
                Name:    "language",
                Aliases: []string{"l"},
                Usage:   "Filter by programming language",
            },
            &cli.StringFlag{
                Name:    "category",
                Aliases: []string{"c"},
                Usage:   "Filter by category",
            },
            &cli.StringFlag{
                Name:    "author",
                Aliases: []string{"a"},
                Usage:   "Filter by author",
            },
            &cli.StringFlag{
                Name:    "output",
                Aliases: []string{"o"},
                Value:   "table",
                Usage:   "Output format: table, json, yaml",
            },
            &cli.IntFlag{
                Name:  "limit",
                Value: 20,
                Usage: "Maximum number of recipes to display",
            },
        },
        Action: handleRecipeList,
    }
}

func handleRecipeList(c *cli.Context) error {
    // Build filter from command flags
    filter := RecipeFilter{
        Tags:     c.StringSlice("tag"),
        Language: c.String("language"),
        Category: c.String("category"),
        Author:   c.String("author"),
        Limit:    c.Int("limit"),
    }
    
    // Query recipes
    recipes, err := queryRecipes(filter)
    if err != nil {
        return cli.NewExitError(fmt.Sprintf("Query failed: %v", err), 1)
    }
    
    // Display results based on output format
    outputFormat := c.String("output")
    return displayRecipes(recipes, outputFormat)
}

// searchRecipesCommand provides full-text search
func searchRecipesCommand() *cli.Command {
    return &cli.Command{
        Name:      "search",
        Usage:     "Search recipes by name, description, or content",
        ArgsUsage: "<search-query>",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:    "output",
                Aliases: []string{"o"},
                Value:   "table",
                Usage:   "Output format: table, json, yaml",
            },
            &cli.IntFlag{
                Name:  "limit",
                Value: 10,
                Usage: "Maximum number of results",
            },
        },
        Action: handleRecipeSearch,
    }
}
```

### Recipe Execution Integration

```go
// runRecipeCommand integrates with existing benchmark system
func runRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:      "run",
        Usage:     "Execute a recipe against a repository",
        ArgsUsage: "<recipe-id> [repository]",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:    "repo",
                Aliases: []string{"r"},
                Usage:   "Repository URL or local path",
            },
            &cli.StringFlag{
                Name:  "branch",
                Value: "main",
                Usage: "Git branch to use",
            },
            &cli.StringFlag{
                Name:  "output-dir",
                Usage: "Directory for output files",
            },
            &cli.BoolFlag{
                Name:  "report",
                Usage: "Generate detailed execution report",
            },
        },
        Action: handleRecipeRun,
    }
}

func handleRecipeRun(c *cli.Context) error {
    recipeID := c.Args().First()
    if recipeID == "" {
        return cli.NewExitError("Recipe ID is required", 1)
    }
    
    // Load recipe from storage
    recipe, err := getRecipe(recipeID)
    if err != nil {
        return cli.NewExitError(fmt.Sprintf("Recipe not found: %v", err), 1)
    }
    
    // Determine repository source
    repoSource := c.String("repo")
    if repoSource == "" && c.Args().Len() > 1 {
        repoSource = c.Args().Get(1)
    }
    if repoSource == "" {
        repoSource = "." // Default to current directory
    }
    
    // Create benchmark configuration for recipe execution
    benchmarkConfig := &BenchmarkConfig{
        Name:       fmt.Sprintf("recipe-%s", recipe.Metadata.Name),
        Repository: repoSource,
        Branch:     c.String("branch"),
        Recipe:     recipe,
        Iterations: 1,
        OutputDir:  c.String("output-dir"),
    }
    
    // Execute through existing benchmark system
    result, err := executeBenchmark(benchmarkConfig)
    if err != nil {
        return cli.NewExitError(fmt.Sprintf("Execution failed: %v", err), 1)
    }
    
    // Display results
    displayExecutionResult(result, c.Bool("report"))
    return nil
}
```

### Recipe Composition & Chaining

```go
// composeRecipeCommand enables multi-recipe execution
func composeRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:      "compose",
        Usage:     "Execute multiple recipes in sequence",
        ArgsUsage: "<recipe-ids...>",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:    "repo",
                Aliases: []string{"r"},
                Usage:   "Repository URL or local path",
            },
            &cli.BoolFlag{
                Name:  "stop-on-error",
                Usage: "Stop execution if any recipe fails",
                Value: true,
            },
            &cli.StringFlag{
                Name:  "composition-name",
                Usage: "Name for the recipe composition",
            },
        },
        Action: handleRecipeCompose,
    }
}

func handleRecipeCompose(c *cli.Context) error {
    if c.Args().Len() < 2 {
        return cli.NewExitError("At least 2 recipe IDs required", 1)
    }
    
    // Create composite recipe from individual recipes
    compositeRecipe, err := createCompositeRecipe(
        c.Args().Slice(),
        c.String("composition-name"),
        c.Bool("stop-on-error"),
    )
    if err != nil {
        return cli.NewExitError(fmt.Sprintf("Failed to create composition: %v", err), 1)
    }
    
    // Execute composite recipe
    return executeCompositeRecipe(compositeRecipe, c.String("repo"))
}
```

### Import/Export Commands

```go
// exportRecipeCommand creates recipe archives
func exportRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:  "export",
        Usage: "Export recipes to an archive file",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:     "output",
                Aliases:  []string{"o"},
                Usage:    "Output archive file path",
                Required: true,
            },
            &cli.StringSliceFlag{
                Name:  "recipe-id",
                Usage: "Specific recipe IDs to export",
            },
            &cli.StringSliceFlag{
                Name:  "tag",
                Usage: "Export recipes with specific tags",
            },
            &cli.StringFlag{
                Name:  "author",
                Usage: "Export recipes by author",
            },
            &cli.StringFlag{
                Name:    "format",
                Aliases: []string{"f"},
                Value:   "tar.gz",
                Usage:   "Archive format: tar.gz, zip",
            },
        },
        Action: handleRecipeExport,
    }
}

// importRecipeCommand imports recipe archives
func importRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:      "import",
        Usage:     "Import recipes from an archive file",
        ArgsUsage: "<archive-file>",
        Flags: []cli.Flag{
            &cli.BoolFlag{
                Name:  "overwrite",
                Usage: "Overwrite existing recipes with same name",
            },
            &cli.BoolFlag{
                Name:  "validate-only",
                Usage: "Validate recipes without importing",
            },
        },
        Action: handleRecipeImport,
    }
}
```

## User Experience Enhancements

### Interactive Recipe Creation

```go
// initRecipeCommand provides guided recipe creation
func initRecipeCommand() *cli.Command {
    return &cli.Command{
        Name:  "init",
        Usage: "Initialize a new recipe from template",
        Flags: []cli.Flag{
            &cli.StringFlag{
                Name:  "template",
                Usage: "Recipe template: openrewrite, shell, composite",
                Value: "openrewrite",
            },
            &cli.StringFlag{
                Name:  "name",
                Usage: "Recipe name",
            },
            &cli.BoolFlag{
                Name:  "interactive",
                Usage: "Use interactive mode",
                Value: true,
            },
        },
        Action: handleRecipeInit,
    }
}

func handleRecipeInit(c *cli.Context) error {
    if c.Bool("interactive") {
        return runInteractiveRecipeCreation(c.String("template"))
    }
    
    return createRecipeFromTemplate(c.String("template"), c.String("name"))
}

// Interactive prompts for recipe creation
func runInteractiveRecipeCreation(templateType string) error {
    prompt := promptui.Prompt{
        Label: "Recipe Name",
        Validate: func(input string) error {
            if len(input) < 3 {
                return errors.New("Recipe name must be at least 3 characters")
            }
            return nil
        },
    }
    
    name, err := prompt.Run()
    if err != nil {
        return err
    }
    
    // Additional prompts for description, tags, etc.
    // ... (implementation continues with comprehensive interactive flow)
}
```

### Help & Documentation Integration

```go
// Enhanced help system with examples
func enhanceRecipeCommands() {
    // Add detailed usage examples to each command
    uploadCmd.UsageText = `
Upload a transformation recipe to the ARF system.

EXAMPLES:
   ploy arf recipe upload my-java-migration.yaml
   ploy arf recipe upload recipe.yaml --name "Custom Migration" 
   ploy arf recipe upload recipe.yaml --dry-run --force

RECIPE FORMAT:
   Recipes use YAML format with metadata, steps, execution config, and validation rules.
   See documentation at: https://docs.ployd.app/arf/recipes
`
}
```

## Implementation Plan

### Core CLI Commands
- ⏳ Implement basic CRUD commands (upload, update, delete, list, show)
- ⏳ Build recipe discovery commands (search, filter)  
- ✅ Integration testing with Phase 5.1 storage backend

### Execution Integration
- ✅ Integrate recipe execution with existing benchmark system
- ⏳ Implement recipe composition and chaining capabilities
- ✅ Build comprehensive execution reporting and error handling

### Advanced Features & UX
- ⏳ Implement import/export functionality with archive support
- ❌ Build interactive recipe creation and validation tools
- ⏳ Enhance help system, documentation, and user experience

## Testing Strategy

### Unit Tests
- Command argument parsing and validation
- Recipe filtering and search logic
- Integration with storage backend APIs
- Error handling and user feedback

### Integration Tests  
- End-to-end recipe management workflows
- Recipe execution through benchmark system
- Import/export functionality with various formats
- CLI usability with different terminal environments

### User Experience Tests
- Interactive recipe creation flow
- Help system comprehensiveness and accuracy
- Error message clarity and actionability
- Command discoverability and intuition

## Configuration Integration

### CLI Configuration Extension

```yaml
# ~/.ploy/config.yaml extension for recipe management
arf:
  recipes:
    default_author: "username"
    default_license: "MIT"
    auto_validate: true
    preferred_format: "yaml"
    
  storage:
    cache_recipes: true
    cache_duration: "1h"
    max_cache_size: "100MB"
    
  execution:
    default_timeout: "15m"
    auto_backup: true
    backup_location: "~/.ploy/recipe-backups"
```

## Success Metrics

### Usability Metrics
- **Command Discovery**: <30s average time to find relevant command
- **Recipe Upload Success**: >95% success rate on first attempt
- **Search Accuracy**: >90% relevant results in top 5 search results
- **Help System Usage**: <20% users requiring external documentation

### Performance Metrics  
- **Command Response Time**: <2s for all non-execution commands
- **Recipe Search Speed**: <500ms for complex queries
- **Import/Export Speed**: >10MB/s for archive operations
- **Concurrent Operations**: Support 20+ simultaneous CLI sessions

### Integration Metrics
- **Benchmark Integration**: 100% compatibility with existing benchmark system
- **Storage Backend**: >99.9% command success rate with Phase 5.1 storage
- **Error Recovery**: Automatic recovery from 90% of transient failures

## Documentation Deliverables

### User Documentation
- **CLI Reference**: Complete command documentation with examples
- **Recipe Format Guide**: YAML specification with best practices
- **Workflow Tutorials**: Common recipe management scenarios
- **Troubleshooting Guide**: Common issues and resolution steps

### Developer Documentation  
- **Command Implementation Guide**: Adding new recipe commands
- **Integration Patterns**: Working with storage and execution backends
- **Testing Framework**: CLI testing utilities and patterns
- **Extension Points**: Customizing CLI behavior and output

## Next Phase Integration

Phase ARF-5.2 completion enables:
- **Phase ARF-5.3**: Generic Execution Engine needs CLI for recipe orchestration
- **Phase ARF-5.4**: Discovery Features require CLI for user interaction with ecosystem
- **User Adoption**: Complete recipe management workflow from creation to execution

## Risk Mitigation

### User Experience Risks
- **Complex Command Structure**: Mitigated through hierarchical help and interactive modes
- **Recipe Format Complexity**: Addressed via templates and validation feedback
- **Discovery Overwhelm**: Resolved through intelligent filtering and categorization

### Technical Risks
- **CLI Performance**: Optimized through caching and lazy loading strategies
- **Cross-Platform Compatibility**: Tested on Windows, macOS, and Linux environments
- **Integration Complexity**: Isolated through clean interface boundaries

### Adoption Risks
- **Learning Curve**: Reduced through comprehensive tutorials and examples  
- **Migration Friction**: Minimized via backward compatibility and gradual transition
- **Feature Discoverability**: Enhanced through contextual help and command suggestions
