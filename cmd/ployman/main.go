package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "controller":
			ControllerCmd(os.Args[2:])
		case "help", "--help", "-h":
			usage()
		default:
			fmt.Printf("Unknown command: %s\n", os.Args[1])
			usage()
		}
		return
	}
	
	usage()
}

func usage() {
	fmt.Println("Ploy Manager - Controller and infrastructure management CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  ployman controller <subcommand>    Manage controller binaries")
	fmt.Println("")
	fmt.Println("Controller Commands:")
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
}