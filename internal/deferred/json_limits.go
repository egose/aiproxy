package deferred

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

type JSONLimitError struct {
	Kind  string
	Limit int
}

func (e JSONLimitError) Error() string {
	return fmt.Sprintf("deferred JSON %s limit exceeded (%d)", e.Kind, e.Limit)
}

type JSONShape struct {
	Tokens int
}

type structureScanner struct {
	data      []byte
	at        int
	tokens    int
	maxDepth  int
	maxTokens int
	chat      bool
	object    bool
	models    int
	model     []byte
	streams   int
	stream    []byte
}

func InspectJSON(data []byte, maxDepth, maxTokens int) (JSONShape, error) {
	s := structureScanner{data: data, maxDepth: maxDepth, maxTokens: maxTokens}
	err := s.scan()
	return JSONShape{Tokens: s.tokens}, err
}

func InspectChatInput(data []byte, maxBytes int64, maxDepth, maxTokens int) (JSONShape, bool, error) {
	if maxBytes < 1 || int64(len(data)) > maxBytes {
		return JSONShape{}, false, fmt.Errorf("deferred input exceeds request limit")
	}
	s := structureScanner{data: data, maxDepth: maxDepth, maxTokens: maxTokens, chat: true}
	if err := s.scan(); err != nil {
		return JSONShape{}, false, err
	}
	if !s.object || s.models != 1 || len(s.model) < 3 || s.model[0] != '"' {
		return JSONShape{}, false, fmt.Errorf("deferred input requires one model string")
	}
	if s.streams > 1 || s.streams == 1 && !bytes.Equal(s.stream, []byte("true")) && !bytes.Equal(s.stream, []byte("false")) {
		return JSONShape{}, false, fmt.Errorf("deferred input stream must be one boolean")
	}
	return JSONShape{Tokens: s.tokens}, bytes.Equal(s.stream, []byte("true")), nil
}

func ValidateChatInput(data []byte, maxBytes int64) (bool, error) {
	_, streaming, err := InspectChatInput(data, maxBytes, JSONDepth, JSONTokens)
	return streaming, err
}

func ChatInputModel(data []byte) (string, error) {
	if _, _, err := InspectChatInput(data, HardRequestBytes, JSONDepth, JSONTokens); err != nil {
		return "", err
	}
	s := structureScanner{data: data, maxDepth: JSONDepth, maxTokens: JSONTokens, chat: true}
	if err := s.scan(); err != nil {
		return "", err
	}
	if len(s.model) > 512*6+2 {
		return "", fmt.Errorf("invalid deferred public model")
	}
	var model string
	if json.Unmarshal(s.model, &model) != nil || ValidatePublicModel(model) != nil {
		return "", fmt.Errorf("invalid deferred public model")
	}
	return model, nil
}

func (s *structureScanner) scan() error {
	if s.maxDepth < 1 || s.maxDepth > JSONDepth || s.maxTokens < 1 || s.maxTokens > JSONTokens {
		return fmt.Errorf("invalid deferred JSON limits")
	}
	s.space()
	if err := s.value(1); err != nil {
		return err
	}
	s.space()
	if s.at != len(s.data) {
		return fmt.Errorf("trailing deferred JSON")
	}
	return nil
}

func (s *structureScanner) space() {
	for s.at < len(s.data) {
		switch s.data[s.at] {
		case ' ', '\t', '\r', '\n':
			s.at++
		default:
			return
		}
	}
}

func (s *structureScanner) token(depth int) error {
	if depth > s.maxDepth {
		return JSONLimitError{Kind: "depth", Limit: s.maxDepth}
	}
	if s.tokens >= s.maxTokens {
		return JSONLimitError{Kind: "tokens", Limit: s.maxTokens}
	}
	s.tokens++
	return nil
}

func (s *structureScanner) value(depth int) error {
	if err := s.token(depth); err != nil {
		return err
	}
	s.space()
	if s.at == len(s.data) {
		return fmt.Errorf("incomplete deferred JSON")
	}
	switch s.data[s.at] {
	case '{':
		if depth == 1 {
			s.object = true
		}
		s.at++
		s.space()
		if s.take('}') {
			return nil
		}
		for {
			if err := s.token(depth + 1); err != nil {
				return err
			}
			keyStart := s.at
			if err := s.string(); err != nil {
				return err
			}
			key := s.data[keyStart:s.at]
			s.space()
			if !s.take(':') {
				return fmt.Errorf("invalid deferred JSON object")
			}
			s.space()
			start := s.at
			if err := s.value(depth + 1); err != nil {
				return err
			}
			if s.chat && depth == 1 {
				if jsonNameEqual(key, "model") {
					s.models++
					s.model = s.data[start:s.at]
				} else if jsonNameEqual(key, "stream") {
					s.streams++
					s.stream = s.data[start:s.at]
				}
			}
			s.space()
			if s.take('}') {
				return nil
			}
			if !s.take(',') {
				return fmt.Errorf("invalid deferred JSON object")
			}
			s.space()
		}
	case '[':
		s.at++
		s.space()
		if s.take(']') {
			return nil
		}
		for {
			if err := s.value(depth + 1); err != nil {
				return err
			}
			s.space()
			if s.take(']') {
				return nil
			}
			if !s.take(',') {
				return fmt.Errorf("invalid deferred JSON array")
			}
			s.space()
		}
	case '"':
		return s.string()
	case 't':
		return s.literal("true")
	case 'f':
		return s.literal("false")
	case 'n':
		return s.literal("null")
	default:
		return s.number()
	}
}

func (s *structureScanner) take(c byte) bool {
	if s.at < len(s.data) && s.data[s.at] == c {
		s.at++
		return true
	}
	return false
}

func (s *structureScanner) string() error {
	if !s.take('"') {
		return fmt.Errorf("invalid deferred JSON string")
	}
	for s.at < len(s.data) {
		c := s.data[s.at]
		s.at++
		if c == '"' {
			return nil
		}
		if c < ' ' {
			return fmt.Errorf("invalid deferred JSON string")
		}
		if c != '\\' {
			continue
		}
		if s.at == len(s.data) {
			break
		}
		c = s.data[s.at]
		s.at++
		switch c {
		case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
		case 'u':
			if len(s.data)-s.at < 4 {
				return fmt.Errorf("invalid deferred JSON escape")
			}
			for range 4 {
				if hexDigit(s.data[s.at]) < 0 {
					return fmt.Errorf("invalid deferred JSON escape")
				}
				s.at++
			}
		default:
			return fmt.Errorf("invalid deferred JSON escape")
		}
	}
	return fmt.Errorf("incomplete deferred JSON string")
}

func (s *structureScanner) literal(value string) error {
	if !bytes.HasPrefix(s.data[s.at:], []byte(value)) {
		return fmt.Errorf("invalid deferred JSON value")
	}
	s.at += len(value)
	return nil
}

func (s *structureScanner) number() error {
	start := s.at
	s.take('-')
	if !s.take('0') {
		if s.at == len(s.data) || s.data[s.at] < '1' || s.data[s.at] > '9' {
			return fmt.Errorf("invalid deferred JSON number")
		}
		s.digits()
	}
	if s.take('.') {
		if !s.digits() {
			return fmt.Errorf("invalid deferred JSON number")
		}
	}
	if s.take('e') || s.take('E') {
		if !s.take('+') {
			s.take('-')
		}
		if !s.digits() {
			return fmt.Errorf("invalid deferred JSON number")
		}
	}
	if start == s.at {
		return fmt.Errorf("invalid deferred JSON value")
	}
	return nil
}

func (s *structureScanner) digits() bool {
	start := s.at
	for s.at < len(s.data) && s.data[s.at] >= '0' && s.data[s.at] <= '9' {
		s.at++
	}
	return s.at != start
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

func jsonNameEqual(raw []byte, name string) bool {
	at := 1
	for i := range name {
		if at >= len(raw)-1 {
			return false
		}
		c := int(raw[at])
		at++
		if c == '\\' {
			if at+5 > len(raw)-1 || raw[at] != 'u' {
				return false
			}
			c = 0
			for j := 1; j <= 4; j++ {
				c = c*16 + hexDigit(raw[at+j])
			}
			at += 5
		}
		if c != int(name[i]) {
			return false
		}
	}
	return at == len(raw)-1
}

type jsonSizer struct {
	bytes     int64
	limit     int64
	tokens    int
	maxDepth  int
	maxTokens int
}

func JSONEncodedSize(value any, limit int64, maxDepth, maxTokens int) (int64, error) {
	if limit < 0 || maxDepth < 1 || maxDepth > JSONDepth || maxTokens < 1 || maxTokens > JSONTokens {
		return 0, fmt.Errorf("invalid deferred JSON encoding limits")
	}
	s := jsonSizer{limit: limit, maxDepth: maxDepth, maxTokens: maxTokens}
	if err := s.value(reflect.ValueOf(value), 1, false); err != nil {
		return 0, err
	}
	return s.bytes, nil
}

func JSONStringBytes(value string) int64 {
	size := int64(2)
	for i := 0; i < len(value); {
		c := value[i]
		switch {
		case c == '\\' || c == '"' || c == '\b' || c == '\f' || c == '\n' || c == '\r' || c == '\t':
			size += 2
			i++
		case c < ' ' || c == '<' || c == '>' || c == '&':
			size += 6
			i++
		case c < utf8.RuneSelf:
			size++
			i++
		default:
			r, n := utf8.DecodeRuneInString(value[i:])
			if r == utf8.RuneError && n == 1 {
				size += 3
			} else if r == '\u2028' || r == '\u2029' {
				size += 6
			} else {
				size += int64(n)
			}
			i += n
		}
	}
	return size
}

func (s *jsonSizer) add(n int64) error {
	if n < 0 || n > s.limit-s.bytes {
		return fmt.Errorf("deferred JSON encoded size exceeds limit")
	}
	s.bytes += n
	return nil
}

func (s *jsonSizer) token(depth int) error {
	if depth > s.maxDepth {
		return JSONLimitError{Kind: "depth", Limit: s.maxDepth}
	}
	if s.tokens >= s.maxTokens {
		return JSONLimitError{Kind: "tokens", Limit: s.maxTokens}
	}
	s.tokens++
	return nil
}

func (s *jsonSizer) value(v reflect.Value, depth int, quoted bool) error {
	if err := s.token(depth); err != nil {
		return err
	}
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return s.add(4)
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return s.add(4)
	}
	if v.Type() == reflect.TypeFor[json.RawMessage]() {
		return fmt.Errorf("raw JSON requires a separate bounded encoder")
	}
	if v.CanInterface() {
		if _, ok := v.Interface().(json.Marshaler); ok {
			return fmt.Errorf("custom JSON encoders are not supported by bounded encoding")
		}
	}
	var number [64]byte
	switch v.Kind() {
	case reflect.String:
		n := JSONStringBytes(v.String())
		if quoted {
			return fmt.Errorf("quoted JSON strings require a separate bounded encoder")
		}
		return s.add(n)
	case reflect.Bool:
		n := int64(5)
		if v.Bool() {
			n = 4
		}
		if quoted {
			n += 2
		}
		return s.add(n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n := int64(len(strconv.AppendInt(number[:0], v.Int(), 10)))
		if quoted {
			n += 2
		}
		return s.add(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n := int64(len(strconv.AppendUint(number[:0], v.Uint(), 10)))
		if quoted {
			n += 2
		}
		return s.add(n)
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return fmt.Errorf("invalid JSON float")
		}
		bits := v.Type().Bits()
		format := byte('f')
		absolute := math.Abs(f)
		if absolute != 0 && (bits == 64 && (absolute < 1e-6 || absolute >= 1e21) || bits == 32 && (float32(absolute) < 1e-6 || float32(absolute) >= 1e21)) {
			format = 'e'
		}
		data := strconv.AppendFloat(number[:0], f, format, -1, bits)
		n := int64(len(data))
		if format == 'e' && len(data) > 3 && data[len(data)-3] == '-' && data[len(data)-2] == '0' {
			n--
		}
		if quoted {
			n += 2
		}
		return s.add(n)
	case reflect.Slice:
		if v.IsNil() {
			return s.add(4)
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if int64(v.Len()) > (s.limit-2)/4*3+3 {
				return fmt.Errorf("deferred JSON encoded size exceeds limit")
			}
			return s.add(int64(base64.StdEncoding.EncodedLen(v.Len())) + 2)
		}
		fallthrough
	case reflect.Array:
		if err := s.add(2); err != nil {
			return err
		}
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				if err := s.add(1); err != nil {
					return err
				}
			}
			if err := s.value(v.Index(i), depth+1, false); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if v.IsNil() {
			return s.add(4)
		}
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("bounded JSON requires string map keys")
		}
		if err := s.add(2); err != nil {
			return err
		}
		first := true
		iter := v.MapRange()
		for iter.Next() {
			if err := s.key(iter.Key().String(), depth+1, &first); err != nil {
				return err
			}
			if err := s.value(iter.Value(), depth+1, false); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		if err := s.add(2); err != nil {
			return err
		}
		first := true
		return s.fields(v, depth, &first)
	}
	return fmt.Errorf("unsupported bounded JSON type")
}

func (s *jsonSizer) key(name string, depth int, first *bool) error {
	if err := s.token(depth); err != nil {
		return err
	}
	if !*first {
		if err := s.add(1); err != nil {
			return err
		}
	}
	*first = false
	return s.add(JSONStringBytes(name) + 1)
}

func (s *jsonSizer) fields(v reflect.Value, depth int, first *bool) error {
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			nested := field.Type
			if nested.Kind() == reflect.Pointer {
				nested = nested.Elem()
			}
			if !field.Anonymous || nested.Kind() != reflect.Struct {
				continue
			}
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, options, _ := strings.Cut(tag, ",")
		value := v.Field(i)
		if field.Anonymous && name == "" {
			for value.Kind() == reflect.Pointer {
				if value.IsNil() {
					break
				}
				value = value.Elem()
			}
			if value.Kind() == reflect.Struct {
				if depth > s.maxDepth {
					return JSONLimitError{Kind: "depth", Limit: s.maxDepth}
				}
				if err := s.fields(value, depth, first); err != nil {
					return err
				}
				continue
			}
		}
		if name == "" {
			name = field.Name
		}
		if jsonOption(options, "omitempty") && emptyJSONValue(value) || jsonOption(options, "omitzero") && value.IsZero() {
			continue
		}
		if err := s.key(name, depth+1, first); err != nil {
			return err
		}
		if err := s.value(value, depth+1, jsonOption(options, "string")); err != nil {
			return err
		}
	}
	return nil
}

func jsonOption(options, name string) bool {
	for options != "" {
		var option string
		option, options, _ = strings.Cut(options, ",")
		if option == name {
			return true
		}
	}
	return false
}

func emptyJSONValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}
