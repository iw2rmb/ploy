package stackdetect

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const (
	dotnetProject = `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><TargetFramework>net6.0</TargetFramework></PropertyGroup></Project>`
	dotnetPin     = `{"sdk":{"version":"8.0.302"}}`
)

func TestDetectDotnetProjects(t *testing.T) {
	for _, tc := range []struct {
		name, project, content, solution, manifest string
	}{
		{name: "C# project SDK attribute", project: "App.csproj", content: dotnetProject},
		{name: "MSTest project SDK", project: "Tests.csproj", content: `<Project Sdk="MSTest.Sdk/4.1.0"/>`},
		{name: "F# project SDK element", project: "App.fsproj", content: `<Project><Sdk Name="Microsoft.NET.Sdk"/></Project>`},
		{name: "VB project SDK import", project: "App.vbproj", content: `<Project><Import Project="Sdk.props" Sdk="Microsoft.NET.Sdk"/></Project>`},
		{
			name: "solution references a nested project", project: "src/App/App.csproj", content: dotnetProject,
			solution: "App.sln", manifest: "Microsoft Visual Studio Solution File, Format Version 12.00\n" +
				`Project("{FAE04EC0-301F-11D3-BF4B-00C04F79EFBC}") = "App", "src\App\App.csproj", "{3EE08243-09A1-482D-878D-1154F0DD4CC3}"` + "\nEndProject\n",
		},
		{
			name: "XML solution references a nested project", project: "src/App/App.fsproj", content: dotnetProject,
			solution: "App.slnx", manifest: `<Solution><Folder Name="/src/"><Project Path="src/App/App.fsproj"/></Folder></Solution>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeDetectFile(t, root, tc.project, tc.content)
			writeDetectFile(t, root, "global.json", dotnetPin)
			if tc.solution != "" {
				writeDetectFile(t, root, tc.solution, tc.manifest)
			}
			obs, err := Detect(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			assertObservation(t, obs, "dotnet", "dotnet", "8.0.302")
		})
	}
}

func TestDetectDotnetToolWithoutSDKRelease(t *testing.T) {
	for name, pin := range map[string]string{
		"missing file": "", "missing version": `{"sdk":{"rollForward":"latestFeature"}}`,
		"invalid JSON": `{"sdk":`, "invalid version": `{"sdk":{"version":"8.x"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeDetectFile(t, root, "App.csproj", dotnetProject)
			if pin != "" {
				writeDetectFile(t, root, "global.json", pin)
			}
			_, err := Detect(t.Context(), root)
			assertDetectionError(t, err, "unknown")
			obs, err := DetectTool(t.Context(), root)
			if err != nil {
				t.Fatal(err)
			}
			if obs.Language != "dotnet" || obs.Tool != "dotnet" || obs.Release != nil {
				t.Fatalf("unexpected tool observation: %+v", obs)
			}
		})
	}
}

func TestDetectDotnetRequiresSDKProject(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"standalone SDK pin":          {},
		"legacy project":              {"App.csproj": `<Project ToolsVersion="15.0"><Import Project="Microsoft.CSharp.targets"/></Project>`},
		"C++ solution":                {"App.slnx": `<Solution><Project Path="App.vcxproj"/></Solution>`, "App.vcxproj": dotnetProject},
		"unreferenced nested project": {"App.slnx": `<Solution/>`, "src/App.csproj": dotnetProject},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeDetectFile(t, root, "global.json", dotnetPin)
			for path, body := range files {
				writeDetectFile(t, root, path, body)
			}
			_, err := DetectTool(t.Context(), root)
			assertDetectionError(t, err, "unknown")
		})
	}
}

func TestDetectDotnetWithAnotherLanguageIsAmbiguous(t *testing.T) {
	root := t.TempDir()
	writeDetectFile(t, root, "App.csproj", dotnetProject)
	writeDetectFile(t, root, goModuleFile, "module example.com/app\ngo 1.25\n")
	_, err := Detect(t.Context(), root)
	assertDetectionError(t, err, "ambiguous")
	_, err = DetectTool(t.Context(), root)
	assertDetectionError(t, err, "ambiguous")
}

func TestDetectDotnetSolutionKeepsProjectReferencesInsideWorkspace(t *testing.T) {
	for _, ref := range []string{"../outside/App.csproj", "linked/App.csproj"} {
		t.Run(ref, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "repo")
			writeDetectFile(t, parent, "outside/App.csproj", dotnetProject)
			writeDetectFile(t, root, "App.slnx", fmt.Sprintf(`<Solution><Project Path="%s"/></Solution>`, ref))
			if err := os.Symlink(filepath.Join(parent, "outside"), filepath.Join(root, "linked")); err != nil {
				t.Fatal(err)
			}
			_, err := DetectTool(t.Context(), root)
			assertDetectionError(t, err, "unknown")
		})
	}
}

func TestDetectDotnetIgnoresSDKPinOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeDetectFile(t, root, "App.csproj", dotnetProject)
	writeDetectFile(t, outside, "global.json", dotnetPin)
	if err := os.Symlink(filepath.Join(outside, "global.json"), filepath.Join(root, "global.json")); err != nil {
		t.Fatal(err)
	}
	_, err := Detect(t.Context(), root)
	assertDetectionError(t, err, "unknown")
}

func TestDetectDotnetProjectsShareSDKPin(t *testing.T) {
	root := t.TempDir()
	writeDetectFile(t, root, "App.csproj", dotnetProject)
	writeDetectFile(t, root, "Tests.fsproj", dotnetProject)
	writeDetectFile(t, root, "global.json", `{
		// The gate image supplies the installed SDKs.
		"sdk": {"version": "8.0.302", "rollForward": "latestFeature",},
	}`)
	obs, err := Detect(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	assertObservation(t, obs, "dotnet", "dotnet", "8.0.302")
	for _, path := range []string{"App.csproj", "Tests.fsproj", "global.json"} {
		found := false
		for _, item := range obs.Evidence {
			if item.Path == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing evidence for %s: %+v", path, obs.Evidence)
		}
	}
}
