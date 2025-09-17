package nomad

import (
	"os"
	"strings"
	"testing"
)

func TestTemplateForLane(t *testing.T) {
	cases := map[string]string{
		"A": "platform/nomad/lane-a-unikraft.hcl",
		"B": "platform/nomad/lane-b-unikraft-posix.hcl",
		"C": "platform/nomad/lane-c-osv.hcl",
		"D": "platform/nomad/lane-d-jail.hcl",
		"E": "platform/nomad/lane-e-oci-kontain.hcl",
		"F": "platform/nomad/lane-f-vm.hcl",
		"x": "platform/nomad/lane-c-osv.hcl",
	}
	for in, want := range cases {
		if got := templateForLane(in); got != want {
			t.Fatalf("templateForLane(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestTemplateForLaneAndLanguage(t *testing.T) {
	if got := templateForLaneAndLanguage("C", "java"); got != "platform/nomad/lane-c-java.hcl" {
		t.Fatalf("language-specific template not selected for C/java: %q", got)
	}
	if got := templateForLaneAndLanguage("C", "node"); got != "platform/nomad/lane-c-node.hcl" {
		t.Fatalf("language-specific template not selected for C/node: %q", got)
	}
	// Fallback for other lanes/languages
	if got := templateForLaneAndLanguage("E", "java"); got != templateForLane("E") {
		t.Fatalf("expected fallback to generic for lane E: %q", got)
	}
}

func TestDebugTemplateForLane(t *testing.T) {
	cases := map[string]string{
		"A": "platform/nomad/debug-unikraft.hcl",
		"B": "platform/nomad/debug-unikraft.hcl",
		"C": "platform/nomad/debug-unikraft.hcl",
		"D": "platform/nomad/debug-jail.hcl",
		"E": "platform/nomad/debug-oci.hcl",
		"F": "platform/nomad/debug-oci.hcl",
		"x": "platform/nomad/debug-oci.hcl",
	}
	for in, want := range cases {
		if got := debugTemplateForLane(in); got != want {
			t.Fatalf("debugTemplateForLane(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestGetTaskNameForLane(t *testing.T) {
	cases := map[string]string{
		"A": "unikernel",
		"B": "unikernel",
		"C": "osv-jvm",
		"D": "jail",
		"E": "oci-kontain",
		"F": "vm",
		"X": "app",
	}
	for in, want := range cases {
		if got := getTaskNameForLane(in); got != want {
			t.Fatalf("getTaskNameForLane(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestGetDriverConfigForLane(t *testing.T) {
	d := RenderData{ImagePath: "/tmp/image", DockerImage: "example:latest", HttpPort: 8080, JvmMemory: 256}
	if cfg := getDriverConfigForLane("E", d); cfg.Driver != "docker" {
		t.Fatalf("expected docker driver for lane E, got %q", cfg.Driver)
	}
	if cfg := getDriverConfigForLane("D", d); cfg.Driver != "jail" {
		t.Fatalf("expected jail driver for lane D, got %q", cfg.Driver)
	}
	if cfg := getDriverConfigForLane("A", d); cfg.Driver != "qemu" {
		t.Fatalf("expected qemu driver for lane A, got %q", cfg.Driver)
	}
}

func TestRenderEnvVars(t *testing.T) {
	if out := renderCustomEnvVars(nil); out != "" {
		t.Fatalf("expected empty for nil env, got %q", out)
	}
	vars := map[string]string{"FOO": "bar", "NUM": "1"}
	ce := renderCustomEnvVars(vars)
	if !strings.Contains(ce, "FOO = \"bar\"") || !strings.Contains(ce, "NUM = \"1\"") {
		t.Fatalf("custom env rendering missing keys: %q", ce)
	}

	le := renderLegacyEnvVars(vars)
	if !strings.HasPrefix(le, "      env {\n") || !strings.Contains(le, "FOO = \"bar\"") || !strings.HasSuffix(le, "\n      }") {
		t.Fatalf("legacy env block malformed: %q", le)
	}
}

func TestProcessConditionalBlocksAndEvaluate(t *testing.T) {
	tpl := "" +
		"line1\n" +
		"{{#if DEBUG_ENABLED}}debug-on{{/if}}\n" +
		"{{#if GRPC_PORT}}grpc={{GRPC_PORT}}{{/if}}\n" +
		"{{#if DISK_SIZE}}disk={{DISK_SIZE}}{{/if}}\n" +
		"{{#if CONNECT_ENABLED}}connect{{/if}}\n"

	data := RenderData{DebugEnabled: true, GrpcPort: 9090, DiskSize: 0, ConnectEnabled: false}
	out := processConditionalBlocks(tpl, data)
	if !strings.Contains(out, "debug-on") {
		t.Fatalf("expected debug-on in output: %q", out)
	}
	if !strings.Contains(out, "grpc=") {
		t.Fatalf("expected grpc line in output: %q", out)
	}
	if strings.Contains(out, "disk=") {
		t.Fatalf("did not expect disk line when DiskSize=0: %q", out)
	}
	if strings.Contains(out, "connect") {
		t.Fatalf("did not expect connect line when disabled: %q", out)
	}
}

func TestIsPlatformServiceAndDefaults(t *testing.T) {
	// Explicit flag
	if !isPlatformService(RenderData{IsPlatformService: true}) {
		t.Fatal("expected platform service when flag set")
	}
	// Name-based
	if !isPlatformService(RenderData{App: "api"}) {
		t.Fatal("expected platform service for 'api'")
	}
	if isPlatformService(RenderData{App: "my-app"}) {
		t.Fatal("did not expect platform service for custom app")
	}

	// Defaults
	data := RenderData{Language: "java"}
	data.SetDefaults()
	if data.JavaVersion == "" || data.HttpPort == 0 || data.InstanceCount == 0 {
		t.Fatal("defaults not populated")
	}
	// Node memory default adjustment
	data2 := RenderData{Language: "node"}
	data2.SetDefaults()
	if data2.MemoryLimit != 512 {
		t.Fatalf("expected MemoryLimit 512 for node default, got %d", data2.MemoryLimit)
	}

	// Domain suffix substitution behavior (platform vs apps)
	if err := os.Setenv("PLOY_PLATFORM_DOMAIN", "dev.ployman.app"); err != nil {
		t.Fatalf("set platform domain: %v", err)
	}
	if err := os.Setenv("PLOY_APPS_DOMAIN", "dev.ployd.app"); err != nil {
		t.Fatalf("set apps domain: %v", err)
	}
	defer func() { _ = os.Unsetenv("PLOY_PLATFORM_DOMAIN"); _ = os.Unsetenv("PLOY_APPS_DOMAIN") }()

	tpl := "app={{APP_NAME}} domain={{DOMAIN_SUFFIX}}"
	out := applyTemplateSubstitutions(tpl, RenderData{App: "api"})
	if !strings.Contains(out, "domain=dev.ployman.app") {
		t.Fatalf("platform service should use platform domain, got %q", out)
	}
	out2 := applyTemplateSubstitutions(tpl, RenderData{App: "my-app"})
	if !strings.Contains(out2, "domain=dev.ployd.app") {
		t.Fatalf("regular app should use apps domain, got %q", out2)
	}
}
