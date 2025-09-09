package arf

// extractCycloneDXDependencies extracts dependencies from CycloneDX format
func (s *SyftSBOMAnalyzer) extractCycloneDXDependencies(components []interface{}) ([]Dependency, error) {
	var dependencies []Dependency

	for _, component := range components {
		comp, ok := component.(map[string]interface{})
		if !ok {
			continue
		}

		dep := Dependency{
			Name:     s.getStringValue(comp, "name"),
			Version:  s.getStringValue(comp, "version"),
			Type:     s.getStringValue(comp, "type"),
			Metadata: make(map[string]interface{}),
		}

		// Extract purl information for ecosystem
		if purl := s.getStringValue(comp, "purl"); purl != "" {
			dep.Ecosystem = s.extractEcosystemFromPURL(purl)
			dep.Metadata["purl"] = purl
		}

		// Extract license information
		if licenses, ok := comp["licenses"].([]interface{}); ok {
			var licenseList []string
			for _, license := range licenses {
				if lic, ok := license.(map[string]interface{}); ok {
					if id := s.getStringValue(lic, "id"); id != "" {
						licenseList = append(licenseList, id)
					} else if name := s.getStringValue(lic, "name"); name != "" {
						licenseList = append(licenseList, name)
					}
				}
			}
			if len(licenseList) > 0 {
				dep.Metadata["licenses"] = licenseList
			}
		}

		dependencies = append(dependencies, dep)
	}

	return dependencies, nil
}
