package main

import (
	"fmt"
	ac "github.com/iw2rmb/ploy/internal/autocomplete"
)

func main() {
	m := ac.GenerateAll()
	for k, v := range m {
		fmt.Printf("===%s===\n%s\n", k, v)
	}
}
