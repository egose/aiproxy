package deferred

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type jsonScanner struct {
	data      []byte
	offset    int
	tokens    int
	maxDepth  int
	maxTokens int
	keyBytes  int
	top       map[string][]byte
}

func ValidateJSON(data []byte, maxDepth, maxTokens int) error {
	_, err := scanJSON(data, maxDepth, maxTokens)
	return err
}

func scanJSON(data []byte, maxDepth, maxTokens int) (map[string][]byte, error) {
	if _, err := InspectJSON(data, maxDepth, maxTokens); err != nil {
		return nil, err
	}
	scanner := jsonScanner{data: data, maxDepth: maxDepth, maxTokens: maxTokens}
	scanner.space()
	if err := scanner.value(1); err != nil {
		return nil, err
	}
	scanner.space()
	if scanner.offset != len(data) {
		return nil, fmt.Errorf("trailing deferred JSON")
	}
	return scanner.top, nil
}

func (s *jsonScanner) space() {
	for s.offset < len(s.data) {
		switch s.data[s.offset] {
		case ' ', '\n', '\r', '\t':
			s.offset++
		default:
			return
		}
	}
}

func (s *jsonScanner) token() error {
	s.tokens++
	if s.tokens > s.maxTokens {
		return fmt.Errorf("deferred JSON token limit exceeded")
	}
	return nil
}

func (s *jsonScanner) stringEnd() int {
	s.offset++
	for s.offset < len(s.data) {
		c := s.data[s.offset]
		s.offset++
		if c == '\\' {
			s.offset++
		} else if c == '"' {
			break
		}
	}
	return s.offset
}

func (s *jsonScanner) value(depth int) error {
	if depth > s.maxDepth {
		return fmt.Errorf("deferred JSON depth limit exceeded")
	}
	if err := s.token(); err != nil {
		return err
	}
	s.space()
	switch s.data[s.offset] {
	case '{':
		s.offset++
		s.space()
		seen := make(map[string]bool)
		if depth == 1 {
			s.top = make(map[string][]byte)
		}
		for s.data[s.offset] != '}' {
			if err := s.token(); err != nil {
				return err
			}
			start := s.offset
			end := s.stringEnd()
			if end-start > MaxMetadataBytes {
				return fmt.Errorf("deferred JSON key limit exceeded")
			}
			s.keyBytes += end - start
			if s.maxDepth <= ControlJSONDepth && s.maxTokens <= ControlJSONTokens && s.keyBytes > MaxMetadataBytes {
				return fmt.Errorf("deferred JSON metadata key limit exceeded")
			}
			var name string
			if err := json.Unmarshal(s.data[start:end], &name); err != nil {
				return fmt.Errorf("invalid deferred JSON key")
			}
			if seen[name] {
				return fmt.Errorf("duplicate deferred JSON field")
			}
			seen[name] = true
			s.space()
			s.offset++
			s.space()
			start = s.offset
			if err := s.value(depth + 1); err != nil {
				return err
			}
			if depth == 1 {
				s.top[name] = s.data[start:s.offset]
			}
			s.space()
			if s.data[s.offset] != ',' {
				break
			}
			s.offset++
			s.space()
		}
		s.offset++
	case '[':
		s.offset++
		s.space()
		for s.data[s.offset] != ']' {
			if err := s.value(depth + 1); err != nil {
				return err
			}
			s.space()
			if s.data[s.offset] != ',' {
				break
			}
			s.offset++
			s.space()
		}
		s.offset++
	case '"':
		s.stringEnd()
	default:
		for s.offset < len(s.data) {
			switch s.data[s.offset] {
			case ' ', '\r', '\n', '\t', ',', '}', ']':
				return nil
			default:
				s.offset++
			}
		}
	}
	return nil
}

func DecodeSubmit(data []byte) (SubmitRequest, error) {
	maxWire, _ := SubmitWireBytes(HardRequestBytes)
	if int64(len(data)) > maxWire {
		return SubmitRequest{}, protocolError(413, "request_too_large", "deferred submit body exceeds hard limit", AcceptanceNotAccepted)
	}
	fields, err := scanJSON(data, ControlJSONDepth, ControlJSONTokens)
	if err != nil || fields == nil {
		return SubmitRequest{}, protocolError(400, "invalid_body", "invalid deferred submit envelope", AcceptanceNotAccepted)
	}
	for _, name := range []string{"protocol", "version", "generation", "submit_token", "attempt_key", "headers", "body"} {
		if _, ok := fields[name]; !ok {
			return SubmitRequest{}, protocolError(400, "invalid_body", "missing deferred submit field", AcceptanceNotAccepted)
		}
	}
	if len(fields) != 7 {
		return SubmitRequest{}, protocolError(400, "invalid_body", "unknown deferred submit field", AcceptanceNotAccepted)
	}
	body := fields["body"]
	if len(body) < 2 || body[0] != '"' || body[len(body)-1] != '"' || bytes.ContainsRune(body, '\\') {
		return SubmitRequest{}, protocolError(400, "invalid_body", "deferred body must be a standard base64 string", AcceptanceNotAccepted)
	}
	if base64.StdEncoding.DecodedLen(len(body)-2) > int(HardRequestBytes)+2 {
		return SubmitRequest{}, protocolError(413, "request_too_large", "deferred input exceeds hard limit", AcceptanceNotAccepted)
	}
	metadata := len(data) - (len(body) - 2)
	if metadata > MaxMetadataBytes {
		return SubmitRequest{}, protocolError(400, "invalid_body", "deferred submit metadata exceeds limit", AcceptanceNotAccepted)
	}
	var request SubmitRequest
	for _, field := range []struct {
		name string
		out  any
	}{
		{"protocol", &request.Protocol}, {"version", &request.Version}, {"generation", &request.Generation},
		{"submit_token", &request.SubmitToken}, {"attempt_key", &request.AttemptKey}, {"headers", &request.Headers},
	} {
		if bytes.Equal(fields[field.name], []byte("null")) || json.Unmarshal(fields[field.name], field.out) != nil {
			return SubmitRequest{}, protocolError(400, "invalid_body", "invalid deferred submit envelope", AcceptanceNotAccepted)
		}
	}
	request.Body = make([]byte, base64.StdEncoding.DecodedLen(len(body)-2))
	n, err := base64.StdEncoding.Strict().Decode(request.Body, body[1:len(body)-1])
	if err != nil {
		return SubmitRequest{}, protocolError(400, "invalid_body", "invalid deferred inference base64", AcceptanceNotAccepted)
	}
	request.Body = request.Body[:n:n]
	if int64(len(request.Body)) > HardRequestBytes {
		return SubmitRequest{}, protocolError(413, "request_too_large", "deferred input exceeds hard limit", AcceptanceNotAccepted)
	}
	return request, nil
}

func DecodeControl(data []byte, destination any) error {
	if len(data) > MaxControlBodyBytes {
		return fmt.Errorf("deferred control body exceeds limit")
	}
	fields, err := scanJSON(data, ControlJSONDepth, ControlJSONTokens)
	if err != nil || fields == nil {
		return fmt.Errorf("invalid deferred control envelope")
	}
	value := reflect.ValueOf(destination)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("deferred decode requires a destination")
	}
	if err := validateWireFields(value.Elem().Type(), fields); err != nil {
		return err
	}
	metadata := len(data)
	if raw, ok := fields["data"]; ok {
		if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' || bytes.ContainsRune(raw, '\\') || base64.StdEncoding.DecodedLen(len(raw)-2) > MaxReadBytes+2 {
			return fmt.Errorf("invalid deferred read data")
		}
		scratch := make([]byte, base64.StdEncoding.DecodedLen(len(raw)-2))
		n, err := base64.StdEncoding.Strict().Decode(scratch, raw[1:len(raw)-1])
		if err != nil || n > MaxReadBytes {
			return fmt.Errorf("invalid deferred read data")
		}
		metadata -= len(raw) - 2
	}
	if metadata > MaxMetadataBytes {
		return fmt.Errorf("deferred control metadata exceeds limit")
	}
	for _, name := range []string{"available_bytes", "cursor", "next_cursor"} {
		if raw, ok := fields[name]; ok {
			if len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' {
				return fmt.Errorf("invalid deferred byte cursor")
			}
			if _, err := ParseCursor(string(raw[1 : len(raw)-1])); err != nil {
				return err
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid deferred control envelope: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing deferred control envelope")
	}
	switch decoded := destination.(type) {
	case *Capability:
		return validateCapability(*decoded)
	case *Receipt:
		return ValidateReceipt(*decoded)
	case *JobInfo:
		return ValidateJobInfo(*decoded)
	case *ResultManifest:
		if err := ValidateJobInfo(decoded.JobInfo); err != nil {
			return err
		}
		if !decoded.State.Terminal() || validateHash(decoded.BodySHA256) != nil {
			return fmt.Errorf("invalid deferred result manifest")
		}
	case *ReadFrame:
		return ValidateReadFrame(*decoded)
	case *ControlError:
		if decoded.Envelope != NewEnvelope() || ValidateErrorDetail(decoded.Error) != nil ||
			decoded.Acceptance != AcceptanceNotAccepted && decoded.Acceptance != AcceptanceAccepted && decoded.Acceptance != AcceptanceIndeterminate ||
			decoded.PollAfterMS < 0 || decoded.PollAfterMS > 2000 {
			return fmt.Errorf("invalid deferred control error")
		}
	default:
		return fmt.Errorf("unsupported deferred control type")
	}
	return nil
}

func validateWireFields(t reflect.Type, fields map[string][]byte) error {
	if t.Kind() != reflect.Struct {
		return fmt.Errorf("invalid deferred wire type")
	}
	declared := make(map[string]reflect.StructField)
	collectWireFields(t, declared)
	for name := range fields {
		if _, ok := declared[name]; !ok {
			return fmt.Errorf("unknown deferred field %q", name)
		}
	}
	for _, field := range declared {
		tag := field.Tag.Get("json")
		name, options, _ := strings.Cut(tag, ",")
		optional := strings.Contains(options, "omitempty")
		raw, ok := fields[name]
		if name != "" && name != "-" && !optional {
			if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return fmt.Errorf("missing or null deferred field %q", name)
			}
		}
		nestedType := field.Type
		if nestedType.Kind() == reflect.Pointer {
			nestedType = nestedType.Elem()
		}
		if ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && nestedType.Kind() == reflect.Struct {
			nested, err := scanJSON(raw, ControlJSONDepth, ControlJSONTokens)
			if err != nil || nested == nil {
				return fmt.Errorf("invalid deferred field %q", name)
			}
			if err := validateWireFields(nestedType, nested); err != nil {
				return err
			}
		}
	}
	return nil
}

func collectWireFields(t reflect.Type, fields map[string]reflect.StructField) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous {
			collectWireFields(field.Type, fields)
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = field
		}
	}
}
