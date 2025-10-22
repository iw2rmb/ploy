package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/iw2rmb/ploy/internal/autocomplete"
)

// main regenerates shell completion scripts for the Ploy CLI.
func main() {
	outputs := autocomplete.GenerateAll()
	base := filepath.Join("cmd", "ploy", "autocomplete")
	if err := os.MkdirAll(base, 0o755); err != nil {
		log.Fatalf("create autocomplete directory: %v", err)
	}
	for shell, content := range outputs {
		filename := filepath.Join(base, fmt.Sprintf("ploy.%s", shell))
		if len(content) == 0 || content[len(content)-1] != '\n' {
			content += "\n"
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			log.Fatalf("write %s: %v", filename, err)
		}
	}
}
