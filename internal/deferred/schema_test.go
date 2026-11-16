package deferred

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestStrictSubmitSchemaBase64MetadataAndStructuralBounds(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	request := testRequest(t, testCapability(t, s, testOwner("peer")))
	wire, _ := json.Marshal(request)
	decoded, err := DecodeSubmit(wire)
	if err != nil || !bytes.Equal(decoded.Body, request.Body) || decoded.Envelope != request.Envelope {
		t.Fatalf("submit schema round trip: %v", err)
	}
	for _, malformed := range []string{
		string(wire[:len(wire)-1]) + `,"unknown":true}`,
		string(wire[:len(wire)-1]) + `,"protocol":"aiproxy-deferred"}`,
		strings.Replace(string(wire), `"version":1`, `"version":9223372036854775808`, 1),
		strings.Replace(string(wire), `"headers":{`, `"headers":{"Accept":["x"],`, 1),
		strings.Replace(string(wire), `"body":"`, `"body":null,"ignored":"`, 1),
		`{"protocol":"aiproxy-deferred","version":1,"generation":"g","submit_token":"t","attempt_key":"a","headers":{},"body":"Zh=="}`,
		`{"protocol":"aiproxy-deferred","version":1,"generation":"g","submit_token":"t","attempt_key":"a","headers":{},"body":"\u0066"}`,
		`{"protocol":"aiproxy-deferred","version":1,"generation":"g","submit_token":"t","attempt_key":"a","headers":{},"body":""}` + `{}`,
	} {
		if _, err := DecodeSubmit([]byte(malformed)); err == nil {
			t.Fatalf("accepted malformed submit: %.120s", malformed)
		}
	}
	request.Headers.Set("X-Huge", strings.Repeat("x", MaxMetadataBytes))
	wire, _ = json.Marshal(request)
	if _, err := DecodeSubmit(wire); err == nil {
		t.Fatal("submit metadata was not bounded before decoding")
	}
	if err := ValidateJSON([]byte(strings.Repeat("[", 65)+"0"+strings.Repeat("]", 65)), JSONDepth, JSONTokens); err == nil {
		t.Fatal("JSON depth overflow was accepted")
	}
	if err := ValidateJSON([]byte("["+strings.Repeat("0,", JSONTokens)+"0]"), JSONDepth, JSONTokens); err == nil {
		t.Fatal("JSON token overflow was accepted")
	}
	if err := ValidateJSON([]byte(`{"x":1,"x":2}`), JSONDepth, JSONTokens); err == nil {
		t.Fatal("duplicate JSON fields were accepted")
	}
}

func TestStrictControlSchemaRequiredFieldsCanonicalCursorsAndErrorDTO(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	receipt, err := p.Commit(responseWork([]byte("{}")), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	control, _ := s.AcquireControl(owner, false)
	frame, err := s.Read(control, receipt.Generation, receipt.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Release()
	wire, _ := json.Marshal(frame)
	var decoded ReadFrame
	if err := DecodeControl(wire, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range []string{
		strings.Replace(string(wire), `"cursor":"0"`, `"cursor":"00"`, 1),
		strings.Replace(string(wire), `"cursor":"0"`, `"cursor":"+0"`, 1),
		strings.Replace(string(wire), `"cursor":"0"`, `"cursor":"18446744073709551616"`, 1),
		strings.Replace(string(wire), `"cursor":"0"`, `"cursor":0`, 1),
		strings.Replace(string(wire), `"data":"e30="`, `"data":null`, 1),
		strings.Replace(string(wire), `"version":1`, `"version":2`, 1),
		strings.Replace(string(wire), `"prompt_tokens":0,`, "", 1),
		strings.Replace(string(wire), `"prompt_tokens":0`, `"prompt_tokens":-1`, 1),
		string(wire[:len(wire)-1]) + `,"Protocol":"aiproxy-deferred"}`,
		string(wire[:len(wire)-1]) + `,"cursor":"0"}`,
	} {
		if err := DecodeControl([]byte(malformed), &decoded); err == nil {
			t.Fatalf("accepted malformed control: %.160s", malformed)
		}
	}
	failure := protocolError(409, "submission_in_progress", "preparing", AcceptanceNotAccepted)
	wire, _ = json.Marshal(failure.Wire)
	var detail ControlError
	if err := DecodeControl(wire, &detail); err != nil || detail.Error.Type != "submission_in_progress" {
		t.Fatalf("wire error DTO did not round trip: %v", err)
	}
	if _, ok := any(detail).(error); ok {
		t.Fatal("wire DTO improperly implements error through its Error field")
	}
	if _, ok := any(failure).(error); !ok {
		t.Fatal("typed protocol error cannot be consumed by handlers")
	}
}

func TestIdentifiersKeysCursorsHeadersAndMetadataBounds(t *testing.T) {
	for _, value := range []string{"", "-1", "+1", "00", "01", "18446744073709551616", " 1", "1.0"} {
		if _, err := ParseCursor(value); err == nil {
			t.Fatalf("accepted noncanonical/overflow cursor %q", value)
		}
	}
	if got, err := ParseCursor(strconv.FormatUint(math.MaxUint64, 10)); err != nil || got != math.MaxUint64 {
		t.Fatal("canonical maximum cursor failed")
	}
	for _, value := range []string{"0", "65537", "-1", "01"} {
		if _, err := ParseReadLimit(value); err == nil {
			t.Fatalf("accepted invalid limit %q", value)
		}
	}
	for _, value := range []string{"", strings.Repeat("A", 21), strings.Repeat("A", 23), strings.Repeat("A", 22) + "=", strings.Repeat("A", 100000)} {
		if ValidateID(value) == nil {
			t.Fatal("invalid identifier accepted")
		}
	}
	if ValidateAttemptKey(strings.Repeat("A", 22)) != nil || ValidateAttemptKey(strings.Repeat("A", 64)) != nil {
		t.Fatal("valid attempt keys were rejected")
	}
	for _, value := range []string{strings.Repeat("A", 21), strings.Repeat("A", 65), strings.Repeat("A", 21) + "/"} {
		if ValidateAttemptKey(value) == nil {
			t.Fatal("invalid attempt key accepted")
		}
	}
	for _, headers := range []http.Header{
		{"Content-Type": {"application/json"}, "content-type": {"application/json"}},
		{"Content-Type": {"application/json"}, "Authorization": {"Bearer secret"}},
		{"Content-Type": {"application/json"}, "Cookie": {"secret"}},
		{"Content-Type": {"application/json"}, "X-Aiproxy-Deferred-Secret": {"secret"}},
		{"Content-Type": {"application/json"}, "X-Test": {"a\nb"}},
		{"Content-Type": {"application/json"}, "X-Test": {strings.Repeat("x", 4097)}},
		{"Content-Type": {"application/json"}, "X-Test": make([]string, 129)},
	} {
		if _, err := CanonicalSubmitHeaders(headers); err == nil {
			t.Fatal("unsafe/unbounded submit header bag accepted")
		}
	}
	headers, err := FilterResponseHeaders(http.Header{
		"Connection": {"X-Hop"}, "X-Hop": {"discard"}, "X-Aiproxy-Deferred-Version": {"1"},
		"Content-Length": {"100"}, "Content-Type": {"text/event-stream"}, "Retry-After": {"1"},
	})
	if err != nil || len(headers) != 2 || headers.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("response filtering differs from ordinary transport: %v %v", headers, err)
	}
}

func TestResponseConnectionTokensDoNotExpandMetadataAllocations(t *testing.T) {
	short := http.Header{
		"connection":   {" x-hop ", "X-OTHER", ""},
		"X-Hop":        {"discard"},
		"X-Other":      {"discard"},
		"Content-Type": {"application/json"},
		"Retry-After":  {"1"},
	}
	long := short.Clone()
	for i := range long["connection"] {
		var tokens strings.Builder
		for index := range 512 {
			tokens.WriteString("X-")
			tokens.WriteString(strconv.Itoa(i*512 + index))
			tokens.WriteByte(',')
		}
		tokens.WriteString(short["connection"][i])
		long["connection"][i] = tokens.String()
	}
	if err := ValidateHeaderBag(long); err != nil {
		t.Fatal("adversarial connection tokens exceeded the existing header limits", err)
	}
	allocations := func(headers http.Header) float64 {
		return testing.AllocsPerRun(20, func() {
			filtered, err := FilterResponseHeaders(headers)
			if err != nil || len(filtered) != 2 || filtered.Get("Content-Type") != "application/json" ||
				filtered.Get("Retry-After") != "1" || filtered.Get("Connection") != "" {
				t.Fatal("connection token scanning changed response headers", err)
			}
		})
	}
	shortAllocations, longAllocations := allocations(short), allocations(long)
	if longAllocations > shortAllocations+8 {
		t.Fatalf("connection tokens allocated outside fixed metadata ownership: short=%v long=%v", shortAllocations, longAllocations)
	}
}
