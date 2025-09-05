package builders

func bytesTrimSpace(b []byte) string {
	i := 0
	j := len(b)
	for i < j && (b[i] == '\n' || b[i] == '\r' || b[i] == ' ') {
		i++
	}
	for i < j && (b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == ' ') {
		j--
	}
	return string(b[i:j])
}
