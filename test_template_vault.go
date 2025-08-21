package main

import (
	"fmt"
	"os"
	"github.com/ploy/ploy/controller/nomad"
)

func main() {
	data := nomad.RenderData{
		App: "test-vault-app",
		Version: "v1.0.0",
		HttpPort: 8080,
		InstanceCount: 1,
		CpuLimit: 500,
		MemoryLimit: 256,
		VaultEnabled: true,       // Enable Vault
		ConsulConfigEnabled: true, // Enable Consul
		ConnectEnabled: false,    // Keep Connect disabled
	}
	data.SetDefaults()
	
	rendered, err := nomad.RenderTemplate("A", data)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	
	// Read and display first 100 lines of the rendered template
	content, err := os.ReadFile(rendered)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	
	fmt.Printf("Template with Vault enabled (first 2000 chars):\n%s\n", string(content)[:2000])
}
