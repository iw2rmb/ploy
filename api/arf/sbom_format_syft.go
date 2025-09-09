package arf

// extractSyftDependencies extracts dependencies from syft format
func (s *SyftSBOMAnalyzer) extractSyftDependencies(artifacts []interface{}) ([]Dependency, error) {
	var dependencies []Dependency

	for _, artifact := range artifacts {
		art, ok := artifact.(map[string]interface{})
		if !ok {
			continue
		}

		dep := Dependency{
			Name:      s.getStringValue(art, "name"),
			Version:   s.getStringValue(art, "version"),
			Ecosystem: s.getStringValue(art, "language"),
			Type:      s.getStringValue(art, "type"),
			Metadata: map[string]interface{}{
				"id":      s.getStringValue(art, "id"),
				"foundBy": s.getStringValue(art, "foundBy"),
			},
		}

		// Extract location information
		if locations, ok := art["locations"].([]interface{}); ok && len(locations) > 0 {
			if loc, ok := locations[0].(map[string]interface{}); ok {
				dep.Path = s.getStringValue(loc, "path")
			}
		}

		// Extract license information
		if licenses, ok := art["licenses"].([]interface{}); ok {
			var licenseList []string
			for _, license := range licenses {
				if licStr, ok := license.(string); ok {
					licenseList = append(licenseList, licStr)
				}
			}
			if len(licenseList) > 0 {
				dep.Metadata["licenses"] = licenseList
			}
		}

		// Map ecosystem names to standard values
		dep.Ecosystem = s.normalizeEcosystem(dep.Ecosystem, dep.Type)

		dependencies = append(dependencies, dep)
	}

	return dependencies, nil
}
