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
    // Render Lane E (OCI/Kontain) for JVM app to inspect HCL
    rd.ConsulConfigEnabled = true
    rd.VolumeEnabled = false
    rd.Language = "java"
    f, err := orch.RenderTemplate("E", rd)
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	b, _ := os.ReadFile(f)
	fmt.Println(string(b))
}
