package jvmversions

type Coordinate struct {
	Group    string
	Artifact string
}

type SelectorKind uint8

const (
	Any SelectorKind = iota
	Exact
	Prefix
)

// Prefix stores a literal dotted prefix, not the original mask or a version range.
type Selector struct {
	Kind  SelectorKind
	Value string
}

type Constraint struct {
	Coordinate Coordinate
	Selector   Selector
}

type Request struct {
	JVM  int64
	For  []Coordinate
	With []Constraint
}

type Observation struct {
	Found []string `json:"found"`
	With  []string `json:"with"`
}
