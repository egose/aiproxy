package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func rewriteTopLevelModel(body []byte, upstreamModel string) (json.RawMessage, error) {
	fields, err := readJSONObjectFields(body)
	if err != nil {
		return nil, err
	}
	modelCount := 0
	var out bytes.Buffer
	out.WriteByte('{')
	for i, field := range fields {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(field.Name)
		out.Write(key)
		out.WriteByte(':')
		if field.Name == "model" {
			modelCount++
			if modelCount > 1 {
				return nil, fmt.Errorf("duplicate model field")
			}
			value, _ := json.Marshal(upstreamModel)
			out.Write(value)
			continue
		}
		out.Write(bytes.TrimSpace(field.Value))
	}
	if modelCount == 0 {
		if len(fields) > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal("model")
		value, _ := json.Marshal(upstreamModel)
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func rejectUnsupportedTopLevelFields(body []byte, allowed map[string]bool) error {
	fields, err := readJSONObjectFields(body)
	if err != nil {
		return err
	}
	for _, field := range fields {
		if !allowed[field.Name] {
			return fmt.Errorf("unsupported field %q", field.Name)
		}
	}
	return nil
}

type jsonObjectField struct {
	Name  string
	Value json.RawMessage
}

func readJSONObjectFields(body []byte) ([]jsonObjectField, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("request body must be a JSON object")
	}
	var fields []jsonObjectField
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("object key must be a string")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		fields = append(fields, jsonObjectField{Name: name, Value: value})
	}
	tok, err = dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok = tok.(json.Delim)
	if !ok || delim != '}' {
		return nil, fmt.Errorf("request body must be a JSON object")
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected JSON after request object")
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected JSON after request object")
	}
	return fields, nil
}

var openAIChatRequestFields = map[string]bool{
	"model": true, "messages": true, "max_tokens": true, "temperature": true, "top_p": true, "stream": true,
}

var openAIResponsesRequestFields = map[string]bool{
	"model": true, "input": true, "instructions": true, "max_output_tokens": true, "temperature": true, "top_p": true, "stream": true,
}

var openAIEmbeddingRequestFields = map[string]bool{
	"model": true, "input": true, "dimensions": true,
}
