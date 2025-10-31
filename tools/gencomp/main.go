package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/iw2rmb/ploy/internal/autocomplete"
)

func main() {
	out := autocomplete.GenerateAll()
	base := filepath.Join("cmd", "ploy", "autocomplete")
	for shell, text := range out {
		file := filepath.Join(base, "ploy."+shell)
		if err := os.WriteFile(file, []byte(text), 0644); err != nil {
			panic(err)
		}
		fmt.Println("wrote", file)
	}
}
