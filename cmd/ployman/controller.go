package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/iw2rmb/ploy/controller/config"
	"github.com/iw2rmb/ploy/internal/distribution"
	"github.com/iw2rmb/ploy/internal/storage"
)

// ControllerCmd handles controller binary management commands
func ControllerCmd(args []string) {
	if len(args) == 0 {
		fmt.Println("Controller binary management commands:")
		fmt.Println("  ployman controller upload <version> [options]     Upload controller binary")
		fmt.Println("  ployman controller download <version> [options]   Download controller binary")
		fmt.Println("  ployman controller list                          List available versions")
		fmt.Println("  ployman controller rollback <version>           Rollback to version")
		fmt.Println("  ployman controller build <version> [options]    Build and distribute")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  --binary=PATH       Path to controller binary (default: ./build/controller)")
		fmt.Println("  --platform=OS       Target platform (default: current)")
		fmt.Println("  --arch=ARCH         Target architecture (default: current)")
		fmt.Println("  --output=PATH       Output path for download (default: ./controller)")
		fmt.Println("  --build-dir=PATH    Build output directory (default: ./build/dist)")
		fmt.Println("  --no-upload         Build only, don't upload")
		return
	}

	subcommand := args[0]
	subArgs := args[1:]

	switch subcommand {
	case "upload":
		runControllerUpload(subArgs)
	case "download":
		runControllerDownload(subArgs)
	case "list":
		runControllerList(subArgs)
	case "rollback":
		runControllerRollback(subArgs)
	case "build":
		runControllerBuild(subArgs)
	default:
		fmt.Printf("Unknown controller command: %s\n", subcommand)
		fmt.Println("Run 'ployman controller' for usage information")
	}
}

func runControllerUpload(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ployman controller upload <version> [options]")
		return
	}

	version := args[0]
	binaryPath := "./build/controller"
	platform := runtime.GOOS
	arch := runtime.GOARCH
	var gitCommit string

	// Parse flags
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--binary" && i+1 < len(args) {
			binaryPath = args[i+1]
			i++
		} else if arg == "--platform" && i+1 < len(args) {
			platform = args[i+1]
			i++
		} else if arg == "--arch" && i+1 < len(args) {
			arch = args[i+1]
			i++
		} else if arg == "--git-commit" && i+1 < len(args) {
			gitCommit = args[i+1]
			i++
		}
	}

	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		return
	}

	// Convert to SeaweedFS config format
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master:      cfg.Storage.Master,
		Filer:       cfg.Storage.Filer,
		Collection:  cfg.Storage.Collection,
		Replication: cfg.Storage.Replication,
		Timeout:     cfg.Storage.Timeout,
		DataCenter:  cfg.Storage.DataCenter,
		Rack:        cfg.Storage.Rack,
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	if err != nil {
		fmt.Printf("Error: failed to create storage client: %v\n", err)
		return
	}

	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)

	// Create binary info
	metadata := map[string]string{
		"uploader": "ployman-cli",
		"source":   "manual-upload",
	}

	info := distribution.CreateBinaryInfo(version, gitCommit, metadata)
	info.Platform = platform
	info.Architecture = arch
	info.Path = binaryPath

	// Upload binary
	fmt.Printf("Uploading controller binary %s for %s/%s...\n", version, platform, arch)
	if err := distributor.UploadBinary(binaryPath, info); err != nil {
		fmt.Printf("Error: upload failed: %v\n", err)
		return
	}

	fmt.Printf("Successfully uploaded controller binary %s\n", version)
}

func runControllerDownload(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ployman controller download <version> [options]")
		return
	}

	version := args[0]
	platform := runtime.GOOS
	arch := runtime.GOARCH
	output := "./controller"

	// Parse flags
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--platform" && i+1 < len(args) {
			platform = args[i+1]
			i++
		} else if arg == "--arch" && i+1 < len(args) {
			arch = args[i+1]
			i++
		} else if arg == "--output" && i+1 < len(args) {
			output = args[i+1]
			i++
		}
	}

	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		return
	}

	// Convert to SeaweedFS config format
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master:      cfg.Storage.Master,
		Filer:       cfg.Storage.Filer,
		Collection:  cfg.Storage.Collection,
		Replication: cfg.Storage.Replication,
		Timeout:     cfg.Storage.Timeout,
		DataCenter:  cfg.Storage.DataCenter,
		Rack:        cfg.Storage.Rack,
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	if err != nil {
		fmt.Printf("Error: failed to create storage client: %v\n", err)
		return
	}

	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)

	// Download binary
	fmt.Printf("Downloading controller binary %s for %s/%s...\n", version, platform, arch)
	localPath, info, err := distributor.DownloadBinary(version, platform, arch)
	if err != nil {
		fmt.Printf("Error: download failed: %v\n", err)
		return
	}

	// Copy to output path
	if err := copyFile(localPath, output); err != nil {
		fmt.Printf("Error: failed to copy binary: %v\n", err)
		return
	}

	fmt.Printf("Successfully downloaded controller binary %s\n", version)
	fmt.Printf("Output: %s\n", output)
	fmt.Printf("Build time: %s\n", info.BuildTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Git commit: %s\n", info.GitCommit)
	fmt.Printf("SHA256: %s\n", info.SHA256Hash)
}

func runControllerList(args []string) {
	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		return
	}

	// Convert to SeaweedFS config format
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master:      cfg.Storage.Master,
		Filer:       cfg.Storage.Filer,
		Collection:  cfg.Storage.Collection,
		Replication: cfg.Storage.Replication,
		Timeout:     cfg.Storage.Timeout,
		DataCenter:  cfg.Storage.DataCenter,
		Rack:        cfg.Storage.Rack,
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	if err != nil {
		fmt.Printf("Error: failed to create storage client: %v\n", err)
		return
	}

	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)

	// List versions
	versions, err := distributor.ListVersions()
	if err != nil {
		fmt.Printf("Error: failed to list versions: %v\n", err)
		return
	}

	if len(versions) == 0 {
		fmt.Println("No controller binaries found in distribution storage")
		return
	}

	fmt.Println("Available controller binary versions:")
	for _, version := range versions {
		fmt.Printf("  %s\n", version)
	}
}

func runControllerRollback(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ployman controller rollback <version>")
		return
	}

	targetVersion := args[0]

	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		return
	}

	// Convert to SeaweedFS config format
	seaweedfsConfig := storage.SeaweedFSConfig{
		Master:      cfg.Storage.Master,
		Filer:       cfg.Storage.Filer,
		Collection:  cfg.Storage.Collection,
		Replication: cfg.Storage.Replication,
		Timeout:     cfg.Storage.Timeout,
		DataCenter:  cfg.Storage.DataCenter,
		Rack:        cfg.Storage.Rack,
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(seaweedfsConfig)
	if err != nil {
		fmt.Printf("Error: failed to create storage client: %v\n", err)
		return
	}

	// Create distributor and rollback manager
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)
	rollbackManager := distribution.NewRollbackManager(distributor, runtime.GOOS, runtime.GOARCH)

	// Perform rollback
	currentVersion := "current" // TODO: Get current version from running controller
	fmt.Printf("Initiating rollback from %s to %s...\n", currentVersion, targetVersion)

	rollbackInfo, err := rollbackManager.RollbackTo(currentVersion, targetVersion, "manual rollback via ployman CLI")
	if err != nil {
		fmt.Printf("Error: rollback failed: %v\n", err)
		return
	}

	fmt.Printf("Rollback successful!\n")
	fmt.Printf("From version: %s\n", rollbackInfo.FromVersion)
	fmt.Printf("To version: %s\n", rollbackInfo.ToVersion)
	fmt.Printf("Binary path: %s\n", rollbackInfo.Metadata["target_binary_path"])
	fmt.Printf("Timestamp: %s\n", rollbackInfo.Timestamp.Format("2006-01-02 15:04:05"))
}

func runControllerBuild(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: ployman controller build <version> [options]")
		return
	}

	version := args[0]
	platforms := []string{"linux/amd64", "linux/arm64"}
	buildDir := "./build/dist"
	upload := true

	// Parse flags
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--platforms" && i+1 < len(args) {
			// Simple comma-separated parsing
			platforms = []string{args[i+1]}
			i++
		} else if arg == "--build-dir" && i+1 < len(args) {
			buildDir = args[i+1]
			i++
		} else if arg == "--no-upload" {
			upload = false
		}
	}

	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Printf("Error: failed to load config: %v\n", err)
		return
	}

	if upload {
		// Convert to SeaweedFS config format
		seaweedfsConfig := storage.SeaweedFSConfig{
			Master:      cfg.Storage.Master,
			Filer:       cfg.Storage.Filer,
			Collection:  cfg.Storage.Collection,
			Replication: cfg.Storage.Replication,
			Timeout:     cfg.Storage.Timeout,
			DataCenter:  cfg.Storage.DataCenter,
			Rack:        cfg.Storage.Rack,
		}
		
		// Create storage client
		storageClient, err := storage.NewSeaweedFSClient(seaweedfsConfig)
		if err != nil {
			fmt.Printf("Error: failed to create storage client: %v\n", err)
			return
		}

		// Create build pipeline
		cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
		gitRepo := "." // Current directory
		pipeline := distribution.NewBuildPipeline(storageClient, cfg.Storage.Collections.Artifacts, cacheDir, buildDir, gitRepo)

		// Build and distribute
		metadata := map[string]string{
			"builder": "ployman-cli",
			"source":  "automated-build",
		}

		fmt.Printf("Building controller %s for platforms: %v\n", version, platforms)
		if err := pipeline.BuildAndDistribute(version, platforms, metadata); err != nil {
			fmt.Printf("Error: build and distribute failed: %v\n", err)
			return
		}

		fmt.Printf("Successfully built and distributed controller %s\n", version)
	} else {
		fmt.Println("Build-only mode not implemented yet. Use without --no-upload flag.")
	}
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	if err != nil {
		return err
	}

	// Copy permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, srcInfo.Mode())
}