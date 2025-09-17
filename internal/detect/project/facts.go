package project

import (
	"os"
	"path/filepath"

	ddotnet "github.com/iw2rmb/ploy/internal/detect/dotnet"
	dgo "github.com/iw2rmb/ploy/internal/detect/go"
	djava "github.com/iw2rmb/ploy/internal/detect/java"
	dnode "github.com/iw2rmb/ploy/internal/detect/node"
	dpy "github.com/iw2rmb/ploy/internal/detect/python"
	drust "github.com/iw2rmb/ploy/internal/detect/rust"
	dscala "github.com/iw2rmb/ploy/internal/detect/scala"
)

type Versions struct {
	Java   string
	Scala  string
	Node   string
	Python string
	Go     string
	Dotnet string
	Rust   string
}

type BuildFacts struct {
	Language      string
	BuildTool     string // gradle, maven, sbt, npm, pip, go, dotnet
	Versions      Versions
	MainClass     string
	HasJib        bool
	HasDockerfile bool
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func ComputeFacts(srcDir, language string) BuildFacts {
    f := BuildFacts{Language: language}
    // Common flags
    f.HasDockerfile = exists(filepath.Join(srcDir, "Dockerfile"))

    // Heuristic: detect JVM build tools even if language was not pre-detected
    if f.Language == "" {
        if exists(filepath.Join(srcDir, "build.gradle.kts")) || exists(filepath.Join(srcDir, "build.gradle")) {
            f.Language = "java"
            f.BuildTool = "gradle"
        } else if exists(filepath.Join(srcDir, "pom.xml")) {
            f.Language = "java"
            f.BuildTool = "maven"
        }
    }

    // JVM
    if language == "java" || language == "scala" || language == "kotlin" {
        f.Versions.Java = djava.DetectVersion(srcDir)
        if language == "scala" {
            f.Versions.Scala = dscala.DetectVersion(srcDir)
        }
		// Build tool
		if exists(filepath.Join(srcDir, "build.gradle.kts")) || exists(filepath.Join(srcDir, "build.gradle")) {
			f.BuildTool = "gradle"
		}
		if exists(filepath.Join(srcDir, "pom.xml")) {
			f.BuildTool = "maven"
		}
		// Jib presence (plugin detection)
		f.HasJib = djava.DetectJib(srcDir)
		// Main class
		f.MainClass = djava.DetectMainClass(srcDir)
	}

    // If JVM heuristics fired (above), still populate versions and main class
    if f.Language == "java" && f.BuildTool != "" && f.Versions.Java == "" {
        f.Versions.Java = djava.DetectVersion(srcDir)
        f.HasJib = djava.DetectJib(srcDir)
        if f.MainClass == "" {
            f.MainClass = djava.DetectMainClass(srcDir)
        }
    }

    // Node
    if language == "node" || language == "javascript" || language == "typescript" {
        f.Versions.Node = dnode.DetectVersion(srcDir)
        f.BuildTool = "npm"
    }

	// Python
	if language == "python" {
		f.Versions.Python = dpy.DetectVersion(srcDir)
		f.BuildTool = "pip"
	}

	// Go
	if language == "go" {
		f.Versions.Go = dgo.DetectVersion(srcDir)
		f.BuildTool = "go"
	}

	// .NET
	if language == "dotnet" || language == "csharp" || language == "fsharp" {
		f.Versions.Dotnet = ddotnet.DetectVersion(srcDir)
		f.BuildTool = "dotnet"
	}

	// Rust
	if language == "rust" {
		f.Versions.Rust = drust.DetectVersion(srcDir)
		f.BuildTool = "cargo"
	}

	return f
}
