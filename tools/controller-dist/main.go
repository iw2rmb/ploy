package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ploy/ploy/controller/config"
	"github.com/ploy/ploy/internal/distribution"
	"github.com/ploy/ploy/internal/storage"
)

func main() {
	var (
		command    = flag.String("command", "help", "Command to run (upload, download, list, build)")
		version    = flag.String("version", "", "Version to work with")
		binaryPath = flag.String("binary", "./build/controller", "Path to controller binary")
		platform   = flag.String("platform", runtime.GOOS, "Target platform")
		arch       = flag.String("arch", runtime.GOARCH, "Target architecture")
		output     = flag.String("output", "./controller", "Output path for downloaded binary")
		buildDir   = flag.String("build-dir", "./build/dist", "Build output directory")
		upload     = flag.Bool("upload", true, "Upload binaries after building")
	)
	flag.Parse()

	switch *command {
	case "upload":
		if *version == "" {
			log.Fatal("Version is required for upload")
		}
		if err := runUpload(*version, *binaryPath, *platform, *arch); err != nil {
			log.Fatal(err)
		}
	case "download":
		if *version == "" {
			log.Fatal("Version is required for download")
		}
		if err := runDownload(*version, *platform, *arch, *output); err != nil {
			log.Fatal(err)
		}
	case "list":
		if err := runList(); err != nil {
			log.Fatal(err)
		}
	case "build":
		if *version == "" {
			log.Fatal("Version is required for build")
		}
		platforms := []string{"linux/amd64", "linux/arm64"}
		if err := runBuild(*version, platforms, *buildDir, *upload); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Printf("Usage: %s [options]\n", os.Args[0])
		fmt.Println("Commands:")
		fmt.Println("  upload   Upload controller binary")
		fmt.Println("  download Download controller binary")
		fmt.Println("  list     List available versions")
		fmt.Println("  build    Build and distribute binaries")
		flag.PrintDefaults()
	}
}

func runUpload(version, binaryPath, platform, arch string) error {
	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
		return fmt.Errorf("failed to create storage client: %w", err)
	}

	// Create distributor
	cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
	distributor := distribution.NewBinaryDistributor(storageClient, cfg.Storage.Collections.Artifacts, cacheDir)

	// Create binary info
	metadata := map[string]string{
		"uploader": "controller-dist",
		"source":   "manual-upload",
	}

	info := distribution.CreateBinaryInfo(version, "", metadata)
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

func runDownload(version, platform, arch, output string) error {
	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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

func runList() error {
	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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

func runBuild(version string, platforms []string, buildDir string, upload bool) error {
	// Load storage config
	configPath := config.GetStorageConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
			return fmt.Errorf("failed to create storage client: %w", err)
		}

		// Create build pipeline
		cacheDir := filepath.Join(os.TempDir(), "ploy-controller-cache")
		gitRepo := "." // Current directory
		pipeline := distribution.NewBuildPipeline(storageClient, cfg.Storage.Collections.Artifacts, cacheDir, buildDir, gitRepo)

		// Build and distribute
		metadata := map[string]string{
			"builder": "controller-dist",
			"source":  "automated-build",
		}

		fmt.Printf("Building controller %s for platforms: %v\n", version, platforms)
		if err := pipeline.BuildAndDistribute(version, platforms, metadata); err != nil {
			return fmt.Errorf("build and distribute failed: %w", err)
		}

		fmt.Printf("Successfully built and distributed controller %s\n", version)
	} else {
		fmt.Println("Build-only mode not implemented yet")
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