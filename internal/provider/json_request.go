package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/egose/aiproxy/internal/deferred"
)

func rewriteTopLevelModel(body []byte, upstreamModel string, states ...*executionState) (json.RawMessage, error) {
	fields, err := readJSONObjectFields(body)
	if err != nil {
		return nil, err
	}
	modelCount := 0
	var out bytes.Buffer
	state := executionArgument(states)
	if state != nil {
		size := int64(2)
		models := 0
		for i, field := range fields {
			if i > 0 {
				size++
			}
			size += deferred.JSONStringBytes(field.Name) + 1
			if field.Name == "model" {
				models++
				size += deferred.JSONStringBytes(upstreamModel)
			} else {
				size += int64(len(bytes.TrimSpace(field.Value)))
			}
			if size > state.limits.RequestBytes {
				return nil, ErrExecutionLimit{Kind: "rewritten request bytes", Limit: state.limits.RequestBytes}
			}
		}
		if models == 0 {
			if len(fields) > 0 {
				size++
			}
			size += deferred.JSONStringBytes("model") + 1 + deferred.JSONStringBytes(upstreamModel)
		}
		if size > state.limits.RequestBytes {
			return nil, ErrExecutionLimit{Kind: "rewritten request bytes", Limit: state.limits.RequestBytes}
		}
		if err := state.hold(size); err != nil {
			return nil, err
		}
		out = *bytes.NewBuffer(make([]byte, 0, int(size)))
	}
	out.WriteByte('{')
	for i, field := range fields {
		if i > 0 {
			out.WriteByte(',')
		}
		if state == nil {
			key, _ := json.Marshal(field.Name)
			out.Write(key)
		} else {
			writeExecutionJSONString(&out, field.Name)
		}
		out.WriteByte(':')
		if field.Name == "model" {
			modelCount++
			if modelCount > 1 {
				return nil, fmt.Errorf("duplicate model field")
			}
			if state == nil {
				value, _ := json.Marshal(upstreamModel)
				out.Write(value)
			} else {
				writeExecutionJSONString(&out, upstreamModel)
			}
			continue
		}
		out.Write(bytes.TrimSpace(field.Value))
	}
	if modelCount == 0 {
		if len(fields) > 0 {
			out.WriteByte(',')
		}
		out.WriteString(`"model"`)
		out.WriteByte(':')
		if state == nil {
			value, _ := json.Marshal(upstreamModel)
			out.Write(value)
		} else {
			writeExecutionJSONString(&out, upstreamModel)
		}
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func writeExecutionJSONString(out *bytes.Buffer, value string) {
	const hex = "0123456789abcdef"
	out.WriteByte('"')
	for i := 0; i < len(value); {
		c := value[i]
		i++
		switch c {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteByte(c)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			switch {
			case c < ' ' || c == '<' || c == '>' || c == '&':
				out.WriteString(`\u00`)
				out.WriteByte(hex[c>>4])
				out.WriteByte(hex[c&15])
			case c < utf8.RuneSelf:
				out.WriteByte(c)
			default:
				r, n := utf8.DecodeRuneInString(value[i-1:])
				if r == utf8.RuneError && n == 1 {
					out.WriteString("\ufffd")
				} else if r == '\u2028' {
					out.WriteString(`\u2028`)
				} else if r == '\u2029' {
					out.WriteString(`\u2029`)
				} else {
					out.WriteString(value[i-1 : i-1+n])
				}
				i += n - 1
			}
		}
	}
	out.WriteByte('"')
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
