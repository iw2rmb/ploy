package arf

// extractSPDXDependencies extracts dependencies from SPDX format
func (s *SyftSBOMAnalyzer) extractSPDXDependencies(packages []interface{}) ([]Dependency, error) {
	var dependencies []Dependency

	for _, pkg := range packages {
		p, ok := pkg.(map[string]interface{})
		if !ok {
			continue
		}

		dep := Dependency{
			Name:     s.getStringValue(p, "name"),
			Version:  s.getStringValue(p, "versionInfo"),
			Metadata: make(map[string]interface{}),
		}

		// Extract download location for ecosystem detection
		if downloadLocation := s.getStringValue(p, "downloadLocation"); downloadLocation != "" {
			dep.Ecosystem = s.inferEcosystemFromURL(downloadLocation)
			dep.Metadata["downloadLocation"] = downloadLocation
		}

		// Extract license information
		if licenseConcluded := s.getStringValue(p, "licenseConcluded"); licenseConcluded != "" {
			dep.Metadata["licenses"] = []string{licenseConcluded}
		}

		dependencies = append(dependencies, dep)
	}

	return dependencies, nil
}
