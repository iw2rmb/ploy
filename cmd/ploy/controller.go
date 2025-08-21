package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ploy/ploy/internal/config"
	"github.com/ploy/ploy/internal/distribution"
	"github.com/ploy/ploy/internal/storage"
)

var controllerCmd = &cobra.Command{
	Use:   "controller",
	Short: "Controller binary management commands",
	Long:  `Manage controller binary distribution, versioning, and rollbacks`,
}

var uploadCmd = &cobra.Command{
	Use:   "upload [version]",
	Short: "Upload controller binary to distribution storage",
	Long:  `Upload a controller binary to SeaweedFS for distribution across nodes`,
	Args:  cobra.ExactArgs(1),
	RunE:  runUpload,
}

var downloadCmd = &cobra.Command{
	Use:   "download [version]",
	Short: "Download controller binary from distribution storage",
	Long:  `Download a specific version of the controller binary`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDownload,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available controller binary versions",
	Long:  `List all available controller binary versions in distribution storage`,
	RunE:  runList,
}

var rollbackCmd = &cobra.Command{
	Use:   "rollback [to-version]",
	Short: "Rollback controller to previous version",
	Long:  `Rollback the controller to a previous version with safety checks`,
	Args:  cobra.ExactArgs(1),
	RunE:  runRollback,
}

var buildCmd = &cobra.Command{
	Use:   "build [version]",
	Short: "Build and distribute controller binaries",
	Long:  `Build controller binaries for multiple platforms and distribute them`,
	Args:  cobra.ExactArgs(1),
	RunE:  runBuild,
}

func init() {
	rootCmd.AddCommand(controllerCmd)
	controllerCmd.AddCommand(uploadCmd, downloadCmd, listCmd, rollbackCmd, buildCmd)
	
	// Upload command flags
	uploadCmd.Flags().String("binary", "./build/controller", "Path to controller binary")
	uploadCmd.Flags().String("platform", runtime.GOOS, "Target platform (default: current)")
	uploadCmd.Flags().String("arch", runtime.GOARCH, "Target architecture (default: current)")
	uploadCmd.Flags().String("git-commit", "", "Git commit hash (auto-detected if empty)")
	
	// Download command flags
	downloadCmd.Flags().String("platform", runtime.GOOS, "Target platform (default: current)")
	downloadCmd.Flags().String("arch", runtime.GOARCH, "Target architecture (default: current)")
	downloadCmd.Flags().String("output", "./controller", "Output path for downloaded binary")
	
	// Build command flags
	buildCmd.Flags().StringSlice("platforms", []string{"linux/amd64", "linux/arm64"}, "Target platforms to build")
	buildCmd.Flags().String("build-dir", "./build/dist", "Build output directory")
	buildCmd.Flags().Bool("upload", true, "Upload binaries after building")
}

func runUpload(cmd *cobra.Command, args []string) error {
	version := args[0]
	
	// Get flags
	binaryPath, _ := cmd.Flags().GetString("binary")
	platform, _ := cmd.Flags().GetString("platform")
	arch, _ := cmd.Flags().GetString("arch")
	gitCommit, _ := cmd.Flags().GetString("git-commit")
	
	// Load storage config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	
	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)
	
	// Create binary info
	metadata := map[string]string{
		"uploader": "ploy-cli",
		"source":   "manual-upload",
	}
	
	info := distribution.CreateBinaryInfo(version, gitCommit, metadata)
	info.Platform = platform
	info.Architecture = arch
	info.Path = binaryPath
	
	// Upload binary
	fmt.Printf("Uploading controller binary %s for %s/%s...\n", version, platform, arch)
	if err := distributor.UploadBinary(binaryPath, info); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	
	fmt.Printf("Successfully uploaded controller binary %s\n", version)
	return nil
}

func runDownload(cmd *cobra.Command, args []string) error {
	version := args[0]
	
	// Get flags
	platform, _ := cmd.Flags().GetString("platform")
	arch, _ := cmd.Flags().GetString("arch")
	output, _ := cmd.Flags().GetString("output")
	
	// Load storage config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	
	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)
	
	// Download binary
	fmt.Printf("Downloading controller binary %s for %s/%s...\n", version, platform, arch)
	localPath, info, err := distributor.DownloadBinary(version, platform, arch)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	
	// Copy to output path
	if err := copyFile(localPath, output); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}
	
	fmt.Printf("Successfully downloaded controller binary %s\n", version)
	fmt.Printf("Output: %s\n", output)
	fmt.Printf("Build time: %s\n", info.BuildTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Git commit: %s\n", info.GitCommit)
	fmt.Printf("SHA256: %s\n", info.SHA256Hash)
	
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	// Load storage config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	
	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)
	
	// List versions
	versions, err := distributor.ListVersions()
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}
	
	if len(versions) == 0 {
		fmt.Println("No controller binaries found in distribution storage")
		return nil
	}
	
	fmt.Println("Available controller binary versions:")
	for _, version := range versions {
		fmt.Printf("  %s\n", version)
	}
	
	return nil
}

func runRollback(cmd *cobra.Command, args []string) error {
	targetVersion := args[0]
	
	// Load storage config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	// Create storage client
	storageClient, err := storage.NewSeaweedFSClient(cfg.Storage)
	if err != nil {
		return fmt.Errorf("failed to create storage client: %w", err)
	}
	
	// Create distributor and rollback manager
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)
	rollbackManager := distribution.NewRollbackManager(distributor, runtime.GOOS, runtime.GOARCH)
	
	// Perform rollback
	currentVersion := "current" // TODO: Get current version from running controller
	fmt.Printf("Initiating rollback from %s to %s...\n", currentVersion, targetVersion)
	
	rollbackInfo, err := rollbackManager.RollbackTo(currentVersion, targetVersion, "manual rollback via CLI")
	if err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	
	fmt.Printf("Rollback successful!\n")
	fmt.Printf("From version: %s\n", rollbackInfo.FromVersion)
	fmt.Printf("To version: %s\n", rollbackInfo.ToVersion)
	fmt.Printf("Binary path: %s\n", rollbackInfo.Metadata["target_binary_path"])
	fmt.Printf("Timestamp: %s\n", rollbackInfo.Timestamp.Format("2006-01-02 15:04:05"))
	
	return nil
}

func runBuild(cmd *cobra.Command, args []string) error {
	version := args[0]
	
	// Get flags
	platforms, _ := cmd.Flags().GetStringSlice("platforms")
	buildDir, _ := cmd.Flags().GetString("build-dir")
	upload, _ := cmd.Flags().GetBool("upload")
	
	// Load storage config
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	
	if upload {
		// Create storage client
		storageClient, err := storage.NewSeaweedFSClient(cfg.Storage)
		if err != nil {
			return fmt.Errorf("failed to create storage client: %w", err)
		}
		
		// Create build pipeline
		cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
		gitRepo := "." // Current directory
		pipeline := distribution.NewBuildPipeline(storageClient, cfg.Storage.Collections.Artifacts, cacheDir, buildDir, gitRepo)
		
		// Build and distribute
		metadata := map[string]string{
			"builder": "ploy-cli",
			"source":  "automated-build",
		}
		
		fmt.Printf("Building controller %s for platforms: %v\n", version, platforms)
		if err := pipeline.BuildAndDistribute(version, platforms, metadata); err != nil {
			return fmt.Errorf("build and distribute failed: %w", err)
		}
		
		fmt.Printf("Successfully built and distributed controller %s\n", version)
	} else {
		fmt.Println("Build-only mode not implemented yet. Use --upload=false when ready.")
	}
	
	return nil
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