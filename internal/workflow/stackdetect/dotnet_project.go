package stackdetect

import (
	"encoding/xml"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var solutionProjectLine = regexp.MustCompile(`(?m)^[ \t]*Project\("[^"\r\n]+"\)[ \t]*=[ \t]*"[^"\r\n]*",[ \t]*"([^"\r\n]+)",`)

func dotnetProjectEvidence(workspace string) []EvidenceItem {
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil
	}
	defer root.Close()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil
	}

	// A root solution declares which nested projects belong to the build.
	// Scanning the whole checkout would also pick up unrelated fixtures and examples.
	candidates := make(map[string]bool)
	for _, entry := range entries {
		name := entry.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".sln", ".slnx":
			for _, path := range solutionProjects(root, name) {
				path = filepath.FromSlash(strings.ReplaceAll(path, `\`, "/"))
				if filepath.IsLocal(path) && isDotnetProject(path) {
					candidates[filepath.Clean(path)] = true
				}
			}
		default:
			if isDotnetProject(name) {
				candidates[name] = true
			}
		}
	}

	var evidence []EvidenceItem
	for _, path := range slices.Sorted(maps.Keys(candidates)) {
		if sdk := dotnetProjectSDK(readDotnetBuildFile(root, path)); sdk != "" {
			evidence = append(evidence, EvidenceItem{Path: filepath.ToSlash(path), Key: "Sdk", Value: sdk})
		}
	}
	return evidence
}

func isDotnetProject(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csproj", ".fsproj", ".vbproj":
		return true
	}
	return false
}

func readDotnetBuildFile(root *os.Root, path string) []byte {
	info, err := root.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	data, err := root.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func solutionProjects(root *os.Root, path string) []string {
	data := readDotnetBuildFile(root, path)
	var paths []string
	if strings.EqualFold(filepath.Ext(path), ".sln") {
		for _, match := range solutionProjectLine.FindAllSubmatch(data, -1) {
			paths = append(paths, string(match[1]))
		}
		return paths
	}

	type project struct {
		Path string `xml:"Path,attr"`
	}
	var solution struct {
		XMLName        xml.Name  `xml:"Solution"`
		Projects       []project `xml:"Project"`
		FolderProjects []project `xml:"Folder>Project"`
	}
	if err := xml.Unmarshal(data, &solution); err != nil {
		return nil
	}
	for _, item := range append(solution.Projects, solution.FolderProjects...) {
		paths = append(paths, item.Path)
	}
	return paths
}

func dotnetProjectSDK(data []byte) string {
	type sdkImport struct {
		SDK string `xml:"Sdk,attr"`
	}
	var project struct {
		XMLName xml.Name `xml:"Project"`
		SDK     string   `xml:"Sdk,attr"`
		SDKs    []struct {
			Name string `xml:"Name,attr"`
		} `xml:"Sdk"`
		Imports        []sdkImport `xml:"Import"`
		GroupedImports []sdkImport `xml:"ImportGroup>Import"`
	}
	if err := xml.Unmarshal(data, &project); err != nil {
		return ""
	}
	sdks := strings.Split(project.SDK, ";")
	for _, sdk := range project.SDKs {
		sdks = append(sdks, sdk.Name)
	}
	for _, item := range append(project.Imports, project.GroupedImports...) {
		sdks = append(sdks, item.SDK)
	}
	for _, sdk := range sdks {
		sdk = strings.TrimSpace(sdk)
		name, _, _ := strings.Cut(strings.ToLower(sdk), "/")
		if name == "mstest.sdk" || name == "microsoft.net.sdk" || strings.HasPrefix(name, "microsoft.net.sdk.") {
			return sdk
		}
	}
	return ""
}
