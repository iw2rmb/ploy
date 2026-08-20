package types

import (
	"bytes"
	"encoding/json"
)

func marshalRawJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("null"), nil
	}
	return raw.MarshalJSON()
}

func cloneRawJSON(data []byte) json.RawMessage {
	if data == nil {
		return nil
	}
	cloned := make(json.RawMessage, len(data))
	copy(cloned, data)
	return cloned
}

func rawJSONIsEmpty(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}"))
}
