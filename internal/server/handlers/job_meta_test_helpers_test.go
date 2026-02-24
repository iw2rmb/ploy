package handlers

import "encoding/json"

func withNextIDMeta(meta []byte, nextID float64) []byte {
	obj := map[string]any{}
	if len(meta) > 0 {
		if err := json.Unmarshal(meta, &obj); err != nil || obj == nil {
			obj = map[string]any{}
		}
	}
	obj["next_id"] = nextID
	encoded, err := json.Marshal(obj)
	if err != nil {
		return meta
	}
	return encoded
}
