package main

import (
	"fmt"
	"github.com/ploy/ploy/controller/nomad"
)

func main() {
	data := nomad.RenderData{
		App: "test-app",
		Version: "latest",
		HttpPort: 8080,
		InstanceCount: 2,
		CpuLimit: 200,
		MemoryLimit: 128,
		VaultEnabled: false,
		ConsulConfigEnabled: false,
		ConnectEnabled: false,
	}
	data.SetDefaults()
	
	rendered, err := nomad.RenderTemplate("A", data)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	fmt.Printf("Template rendered successfully: %s\n", rendered)
}
