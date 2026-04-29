package handlers

import "sort"

func sortedSectionNames[T any](all map[string][]T) []string {
	sections := make([]string, 0, len(all))
	for section := range all {
		sections = append(sections, section)
	}
	sort.Strings(sections)
	return sections
}
