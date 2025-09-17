package recipes

import "strings"

func detectArchiveFormat(filename string) ArchiveFormat {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return FormatTarGz
	}
	if strings.HasSuffix(lower, ".zip") {
		return FormatZip
	}
	if strings.HasSuffix(lower, ".tar") {
		return FormatTar
	}
	return ""
}
