package contracts

import (
	"encoding/json"
	"fmt"
)

// ToMap converts the ModsSpec to a map[string]any for wire serialization.
// This is useful when the spec needs to be passed through systems that
// expect untyped map representations.
func (s ModsSpec) ToMap() map[string]any {
	data, err := json.Marshal(s)
	if err != nil {
		panic(fmt.Sprintf("ModsSpec.ToMap: json.Marshal failed: %v", err))
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		panic(fmt.Sprintf("ModsSpec.ToMap: json.Unmarshal failed: %v", err))
	}
	return result
}
