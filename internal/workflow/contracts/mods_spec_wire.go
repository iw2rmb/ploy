package contracts

import (
	"encoding/json"
	"fmt"
)

// ToMap converts the ModsSpec to a map[string]any for wire serialization.
// This is useful when the spec needs to be passed through systems that
// expect untyped map representations.
// Returns an error if marshaling or unmarshaling fails.
func (s ModsSpec) ToMap() (map[string]any, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("ModsSpec.ToMap: json.Marshal: %w", err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("ModsSpec.ToMap: json.Unmarshal: %w", err)
	}
	return result, nil
}
