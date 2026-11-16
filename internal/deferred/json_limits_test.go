package deferred

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
)

func TestJSONStructurePreflightDoesNotAllocateOrChangeExtensionFields(t *testing.T) {
	body := []byte(`{"model":"local/chat","stream":true,"extra":{"same":1,"same":2},"messages":[{"content":"text"}]}`)
	allocations := testing.AllocsPerRun(100, func() {
		shape, streaming, err := InspectChatInput(body, HardRequestBytes, JSONDepth, JSONTokens)
		if err != nil || !streaming || shape.Tokens == 0 {
			t.Fatal("bounded chat preflight rejected an extension", err)
		}
	})
	if allocations != 0 {
		t.Fatalf("structural preflight allocated %v times", allocations)
	}
	for _, body := range []string{
		`[]`, `null`, `{"model":null}`, `{"model":1}`, `{"model":""}`, `{"stream":false}`,
		`{"model":"a","model":"a"}`, `{"model":"a","\u006dodel":"a"}`,
		`{"model":"a","stream":1}`, `{"model":"a","stream":null}`, `{"model":"a","stream":"false"}`,
		`{"model":"a","stream":true,"str\u0065am":false}`,
		`{"model":"a",}`, `{"model":"a"}{}`, `{"model":"a","x":01}`,
		`{"model":"a","x":[true false]}`, `{"model":"a","x":"\u00zz"}`,
	} {
		if _, err := ValidateChatInput([]byte(body), HardRequestBytes); err == nil {
			t.Fatalf("accepted malformed chat input: %s", body)
		}
	}
	if _, err := ValidateChatInput(body, int64(len(body))-1); err == nil {
		t.Fatal("input byte limit was not checked")
	}
	for _, body := range []string{`{"\u006dodel":"a"}`, `{"model":"a","str\u0065am":false}`} {
		if _, err := ValidateChatInput([]byte(body), HardRequestBytes); err != nil {
			t.Fatal("valid escaped input key rejected", err)
		}
	}
}

func TestJSONStructureDepthTokensAndMalformedBoundaries(t *testing.T) {
	for _, count := range []int{JSONDepth - 1, JSONDepth} {
		body := []byte(strings.Repeat("[", count) + "0" + strings.Repeat("]", count))
		_, err := InspectJSON(body, JSONDepth, JSONTokens)
		var limit JSONLimitError
		if count == JSONDepth-1 && err != nil || count == JSONDepth && (!errors.As(err, &limit) || limit.Kind != "depth") {
			t.Fatalf("depth boundary %d: %v", count, err)
		}
	}
	for _, count := range []int{JSONTokens - 1, JSONTokens} {
		body := []byte("[" + strings.Repeat("0,", count-1) + "0]")
		shape, err := InspectJSON(body, JSONDepth, JSONTokens)
		if count == JSONTokens-1 && (err != nil || shape.Tokens != JSONTokens) || count == JSONTokens && err == nil {
			t.Fatalf("token boundary %d: %+v %v", count, shape, err)
		}
	}
	for _, body := range []string{``, `[`, `{`, `{"x":`, `{"x"}`, `{"x":1,,}`, `[-]`, `[1.]`, `[1e+]`, `"a`, `"\x"`, `truefalse`} {
		if _, err := InspectJSON([]byte(body), JSONDepth, JSONTokens); err == nil {
			t.Fatalf("accepted invalid JSON: %s", body)
		}
	}
}

func TestJSONEncodingSizeMatchesWireBeforeAllocation(t *testing.T) {
	type embedded struct {
		Number uint64 `json:"number,string"`
	}
	type payload struct {
		embedded
		Text    string            `json:"text"`
		Headers http.Header       `json:"headers"`
		Data    []byte            `json:"data"`
		Empty   string            `json:"empty,omitempty"`
		Float   *float64          `json:"float,omitempty"`
		Values  map[string]string `json:"values"`
	}
	negative := -0.00000001
	for _, value := range []any{
		nil, true, false, math.MaxInt64, uint64(math.MaxUint64),
		0.0, 0.000001, 0.0000001, 1e21, float32(0.0000001), math.SmallestNonzeroFloat64,
		"\x00<>&\u2028\u2029\xff\"\\\n", []byte{}, []byte{1, 2, 3}, []any{nil, true, "x"},
		payload{
			embedded: embedded{Number: math.MaxUint64}, Text: "escaped\n",
			Headers: http.Header{"X-Test": {"\t", "😀"}}, Data: []byte{0, 1, 255},
			Float: &negative, Values: map[string]string{"<>": "&"},
		},
	} {
		wire, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		size, err := JSONEncodedSize(value, int64(len(wire)), JSONDepth, JSONTokens)
		if err != nil || size != int64(len(wire)) {
			t.Fatalf("encoding size for %#v: %d != %d (%q): %v", value, size, len(wire), wire, err)
		}
		if _, err := JSONEncodedSize(value, int64(len(wire))-1, JSONDepth, JSONTokens); err == nil {
			t.Fatal("oversized encoding was not rejected")
		}
	}
}

func TestMetadataExpansionRejectedBeforeMarshalAndReadScratchIsOnlyAPrefix(t *testing.T) {
	headers := http.Header{"X-Test": {strings.Repeat("&", 4096)}}
	if validateHeaders(headers) == nil {
		t.Fatal("escaped header expansion exceeded serialized metadata budget")
	}
	if metadataLimit(ErrorDetail{Type: "error", Message: strings.Repeat("\x00", MaxMetadataBytes)}) == nil {
		t.Fatal("escaped error metadata was not bounded before encoding")
	}
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	receipt, err := p.Commit(responseWork([]byte("{}")), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	control, err := s.AcquireControl(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Release()
	frame, err := s.Read(control, receipt.Generation, receipt.ID, 0, 1)
	if err != nil || cap(control.scratch) != 1 || len(frame.Data) != 1 {
		t.Fatalf("read retained more than its bounded prefix: %v", err)
	}
}
