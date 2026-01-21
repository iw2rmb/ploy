package stackdetect

// Observation represents a detected stack configuration from the workspace.
// It captures the language, build tool, and optionally the release version
// along with evidence supporting the detection.
type Observation struct {
	// Language is the detected programming language (e.g., "java").
	Language string `json:"language"`

	// Tool is the detected build tool (e.g., "maven", "gradle").
	Tool string `json:"tool"`

	// Release is the detected version/release (e.g., "11", "17", "21").
	// Nil when no version could be determined.
	Release *string `json:"release,omitempty"`

	// Evidence contains the file paths and keys that support this detection.
	Evidence []EvidenceItem `json:"evidence"`
}

// EvidenceItem represents a single piece of evidence for a detection.
// It records where in the filesystem a configuration was found.
type EvidenceItem struct {
	// Path is the relative path to the file containing the evidence.
	Path string `json:"path"`

	// Key is the configuration key or property name.
	Key string `json:"key"`

	// Value is the raw value found in the configuration.
	Value string `json:"value"`
}

// DetectionError represents an error during stack detection.
// It includes a reason code for programmatic handling.
type DetectionError struct {
	// Reason is a machine-readable error code.
	// Values: "ambiguous" (multiple build tools), "unknown" (no detection possible).
	Reason string

	// Message is a human-readable description of the error.
	Message string

	// Evidence contains any partial evidence gathered before the error.
	Evidence []EvidenceItem
}

// Error implements the error interface.
func (e *DetectionError) Error() string {
	return e.Message
}

// IsAmbiguous returns true if the error is due to ambiguous detection
// (e.g., both Maven and Gradle build files present).
func (e *DetectionError) IsAmbiguous() bool {
	return e.Reason == "ambiguous"
}

// IsUnknown returns true if the error is due to unknown stack
// (e.g., no build files found, or no version detected).
func (e *DetectionError) IsUnknown() bool {
	return e.Reason == "unknown"
}
