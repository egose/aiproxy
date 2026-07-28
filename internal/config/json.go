package config

import "encoding/json"

// extractKeyValue parses a flat JSON object and returns the string value at the
// given top-level key. A missing key or non-string value is an error.
func extractKeyValue(data []byte, key string) (string, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	raw, ok := m[key]
	if !ok {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}
