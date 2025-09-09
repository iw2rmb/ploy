package main

import (
	"fmt"
	"os"

	orch "github.com/iw2rmb/ploy/internal/orchestration"
)

func main() {
	rd := orch.RenderData{
		App:                 "testapp",
		ImagePath:           "/tmp/app-osv.img",
		Version:             "dev",
		HttpPort:            8080,
		InstanceCount:       1,
		CpuLimit:            200,
		MemoryLimit:         256,
		JvmMemory:           512,
		MainClass:           "com.example.Main",
		DomainSuffix:        "ployd.app",
		VaultEnabled:        false,
		ConnectEnabled:      false,
		ConsulConfigEnabled: false,
		Language:            "java",
	}
	f, err := orch.RenderTemplate("C", rd)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	b, _ := os.ReadFile(f)
	fmt.Println(string(b))
}
