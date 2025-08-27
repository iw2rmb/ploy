package parsers

// Issue represents a single analysis issue found by a parser
type Issue struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Rule     string `json:"rule,omitempty"`
	Message  string `json:"message"`
}