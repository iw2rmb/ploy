package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	shift "github.com/iw2rmb/shift/pkg/shift"
)

// main validates lane TOML definitions using the SHIFT loader and reports the result.
func main() {
	var dir string
	flag.StringVar(&dir, "dir", "lanes", "directory containing lane TOML files")
	flag.Parse()

	abs, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("resolve directory: %v", err)
	}

	registry, err := shift.LoadLaneDirectory(abs)
	if err != nil {
		log.Fatalf("load lanes: %v", err)
	}

	names := registry.List()
	if len(names) == 0 {
		log.Fatalf("no lane definitions found in %s", abs)
	}

	if _, err := fmt.Fprintf(os.Stdout, "Validated %d lanes in %s\n", len(names), abs); err != nil {
		log.Fatalf("write result: %v", err)
	}
}
