package tlsprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"
)

const (
	maxJSONDepth  = 8
	maxJSONTokens = 4096
)

type recipeReader struct {
	decoder *json.Decoder
	tokens  int
}

func readRecipe(data []byte) (map[string]any, error) {
	if len(data) > MaxSpecBytes {
		return nil, recipeError("spec", "must be at most 65536 bytes")
	}
	if !utf8.Valid(data) {
		return nil, recipeError("spec", "must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	r := recipeReader{decoder: decoder}
	value, err := r.value("spec", 0)
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, recipeError("spec", "must be one JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, recipeError("spec", "trailing data is not allowed")
	}
	return root, nil
}

func (r *recipeReader) token(field string) (json.Token, error) {
	if r.tokens >= maxJSONTokens {
		return nil, recipeError(field, "exceeds the 4096-token limit")
	}
	r.tokens++
	token, err := r.decoder.Token()
	if err != nil {
		return nil, recipeError(field, "invalid JSON")
	}
	return token, nil
}

func (r *recipeReader) value(field string, depth int) (any, error) {
	token, err := r.token(field)
	if err != nil {
		return nil, err
	}
	delim, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	depth++
	if depth > maxJSONDepth {
		return nil, recipeError(field, "exceeds the depth limit of 8")
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for r.decoder.More() {
			keyToken, err := r.token(field)
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, recipeError(field, "invalid object key")
			}
			childField := recipeField(field, key)
			if _, exists := object[key]; exists {
				return nil, recipeError(childField, "duplicate object key")
			}
			child, err := r.value(childField, depth)
			if err != nil {
				return nil, err
			}
			object[key] = child
		}
		end, err := r.token(field)
		if err != nil {
			return nil, err
		}
		if end != json.Delim('}') {
			return nil, recipeError(field, "invalid object")
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for r.decoder.More() {
			child, err := r.value(recipeIndex(field, len(array)), depth)
			if err != nil {
				return nil, err
			}
			array = append(array, child)
		}
		end, err := r.token(field)
		if err != nil {
			return nil, err
		}
		if end != json.Delim(']') {
			return nil, recipeError(field, "invalid array")
		}
		return array, nil
	default:
		return nil, recipeError(field, "invalid JSON value")
	}
}

func recipeField(parent, key string) string {
	switch key {
	case "cipher_suites", "compression_methods", "extensions", "min_vers", "max_vers", "name", "named_group_list", "ec_point_format_list", "supported_signature_algorithms", "versions", "protocol_name_list", "client_shares", "group", "key_exchange", "ke_modes", "len":
		if parent == "spec" {
			return key
		}
		return parent + "." + key
	default:
		return parent
	}
}

func recipeIndex(field string, index int) string {
	return field + "[" + strconv.Itoa(index) + "]"
}

func recipeError(field, message string) error {
	return fmt.Errorf("%s: %s", field, message)
}

func recipeObject(value any, field string, allowed ...string) (map[string]any, error) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, recipeError(field, "must be an object")
	}
	for key := range object {
		known := false
		for _, name := range allowed {
			if key == name {
				known = true
				break
			}
		}
		if !known {
			return nil, recipeError(field, "unknown field")
		}
	}
	return object, nil
}

func recipeArray(value any, field string, min, max int) ([]any, error) {
	array, ok := value.([]any)
	if !ok {
		return nil, recipeError(field, "must be a non-null array")
	}
	if len(array) < min || len(array) > max {
		return nil, recipeError(field, fmt.Sprintf("must contain %d..%d entries", min, max))
	}
	return array, nil
}

func recipeStrings(value any, field string, min, max int) ([]string, error) {
	array, err := recipeArray(value, field, min, max)
	if err != nil {
		return nil, err
	}
	strings := make([]string, len(array))
	for i, entry := range array {
		text, ok := entry.(string)
		if !ok || text == "" {
			return nil, recipeError(recipeIndex(field, i), "must be a nonempty string")
		}
		strings[i] = text
	}
	return strings, nil
}

func recipeUnique(values []string, field string) error {
	seen := make(map[string]bool, len(values))
	for i, value := range values {
		if seen[value] {
			return recipeError(recipeIndex(field, i), "duplicate entry")
		}
		seen[value] = true
	}
	return nil
}

func recipeInteger(value any, field string, max uint64) (uint64, error) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, recipeError(field, "must be an integer")
	}
	integer, err := strconv.ParseUint(string(number), 10, 64)
	if err != nil || integer > max {
		return 0, recipeError(field, fmt.Sprintf("must be an integer from 0 to %d", max))
	}
	return integer, nil
}
