package common

import (
	"io"
	"os"
	"strings"
)

func SupportsOSC8(w io.Writer) bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || strings.EqualFold(term, "dumb") {
		return false
	}

	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
