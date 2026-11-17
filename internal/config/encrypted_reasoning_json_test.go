package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestEncryptedReasoningJSONDeclarationRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
		passthrough     bool
		mismatch        string
		messages        []string
	}{
		{name: "omitted"},
		{name: "whitespace", raw: " \n "},
		{name: "null", raw: " null "},
		{name: "reset", raw: `{}`, want: `{}`, passthrough: true, mismatch: EncryptedReasoningFail},
		{name: "false", raw: `{"passthrough":false}`, want: `{"passthrough":false}`, mismatch: EncryptedReasoningFail},
		{name: "true", raw: `{"passthrough":true}`, want: `{"passthrough":true}`, passthrough: true, mismatch: EncryptedReasoningFail},
		{name: "retry", raw: `{"on_caller_mismatch":"strip_and_retry"}`, want: `{"on_caller_mismatch":"strip_and_retry"}`, passthrough: true, mismatch: EncryptedReasoningStripAndRetry},
		{name: "empty mismatch", raw: `{"on_caller_mismatch":""}`, want: `{"on_caller_mismatch":""}`, passthrough: true, mismatch: EncryptedReasoningFail},
		{name: "empty list", raw: `{"match_messages":[]}`, want: `{"match_messages":[]}`, passthrough: true, mismatch: EncryptedReasoningFail},
		{name: "patterns", raw: `{"match_messages":["  Vendor Caller Marker  "]}`, want: `{"match_messages":["  Vendor Caller Marker  "]}`, passthrough: true, mismatch: EncryptedReasoningFail, messages: []string{"vendor caller marker"}},
		{name: "legacy full", raw: `{"passthrough":false,"on_caller_mismatch":"strip_and_retry","match_messages":["Custom Marker"]}`, want: `{"passthrough":false,"on_caller_mismatch":"strip_and_retry","match_messages":["Custom Marker"]}`, mismatch: EncryptedReasoningStripAndRetry, messages: []string{"custom marker"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := `provider "p": model "m"`
			raw := []byte(tc.raw)
			before := string(raw)
			policy, err := DecodeEncryptedReasoningJSON(raw, owner)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := EncodeEncryptedReasoningJSON(policy, owner)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if policy != nil || encoded != nil {
					t.Fatal("absence became a declaration")
				}
				return
			}
			if policy == nil || policy.Declaration == nil || policy.Passthrough != tc.passthrough || policy.OnCallerMismatch != tc.mismatch {
				t.Fatalf("wrong effective policy or declaration: %+v", policy)
			}
			messages := tc.messages
			if messages == nil {
				messages = DefaultEncryptedReasoning().MatchMessages
			}
			if !reflect.DeepEqual(policy.MatchMessages, messages) {
				t.Fatalf("messages = %v, want %v", policy.MatchMessages, messages)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("declaration = %s, want %s", encoded, tc.want)
			}
			policy.MatchMessages[0] = "mutated effective value"
			again, err := EncodeEncryptedReasoningJSON(policy, owner)
			if err != nil || string(again) != string(encoded) || string(raw) != before {
				t.Fatal("encoding default-filled, mutated, or used effective members instead of local intent")
			}
			reopened, err := DecodeEncryptedReasoningJSON(encoded, owner)
			if err != nil || !reflect.DeepEqual(DeclaredEncryptedReasoning(reopened), DeclaredEncryptedReasoning(policy)) {
				t.Fatalf("reopened local intent changed: %v", err)
			}
		})
	}
}

func TestEncryptedReasoningJSONRejectsInvalidPolicies(t *testing.T) {
	seventeen := make([]string, 17)
	for i := range seventeen {
		seventeen[i] = fmt.Sprintf("marker-%02d", i)
	}
	tooMany, err := json.Marshal(map[string]any{"match_messages": seventeen})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`[]`, `true`, `0`, `""`, `NULL`, `{`, `{"passthrough":true`, `{} null`, `null {}`,
		`{"unknown":1}`, `{"Declaration":{}}`, `{"encrypted_reasoning_effective":{}}`,
		`{"Passthrough":false}`, `{"ON_CALLER_MISMATCH":"fail"}`, `{"Match_Messages":[]}`,
		`{"passthrough":false,"passthrough":true}`, `{"passthrough":false,"Passthrough":true}`,
		`{"Passthrough":false,"passthrough":true}`, `{"passthrough":false,"\u0070assthrough":true}`,
		`{"on_caller_mismatch":"fail","on_caller_mismatch":"strip_and_retry"}`,
		`{"match_messages":[],"MATCH_MESSAGES":["custom marker"]}`,
		`{"passthrough":null}`, `{"passthrough":0}`, `{"passthrough":"false"}`,
		`{"on_caller_mismatch":null}`, `{"on_caller_mismatch":false}`, `{"on_caller_mismatch":"FAIL"}`,
		`{"on_caller_mismatch":" fail "}`, `{"on_caller_mismatch":"sometimes"}`,
		`{"match_messages":null}`, `{"match_messages":"custom marker"}`, `{"match_messages":{}}`,
		`{"match_messages":[null]}`, `{"match_messages":[false]}`, `{"match_messages":[["nested"]]}`,
		`{"match_messages":["abc"]}`, `{"match_messages":[""]}`, `{"match_messages":["    "]}`,
		`{"match_messages":["Custom Marker"," custom marker "]}`,
		`{"match_messages":["` + strings.Repeat("x", 257) + `"]}`, string(tooMany),
	} {
		t.Run(raw, func(t *testing.T) {
			for _, owner := range []string{`provider "p"`, `provider "p": model "m"`, `alias "a"`} {
				policy, err := DecodeEncryptedReasoningJSON([]byte(raw), owner)
				if err == nil || policy != nil || !strings.Contains(err.Error(), owner) || !strings.Contains(err.Error(), "encrypted_reasoning") {
					t.Fatalf("invalid policy accepted or missing scope: %v", err)
				}
			}
		})
	}
}

func TestEncryptedReasoningJSONEncodeValidationAndCopies(t *testing.T) {
	invalid := "sometimes"
	if _, err := EncodeEncryptedReasoningJSON(NewEncryptedReasoning(&EncryptedReasoningDeclaration{OnCallerMismatch: &invalid}), `provider "p"`); err == nil {
		t.Fatal("encoding accepted an invalid declaration")
	}
	for _, messages := range [][]string{{"Custom Marker", "custom marker"}, {"abc"}} {
		if _, err := EncodeEncryptedReasoningJSON(NewEncryptedReasoning(&EncryptedReasoningDeclaration{MatchMessages: &messages}), `model "m"`); err == nil {
			t.Fatal("encoding accepted invalid patterns")
		}
	}
	var empty []string
	policy := NewEncryptedReasoning(&EncryptedReasoningDeclaration{MatchMessages: &empty})
	raw, err := EncodeEncryptedReasoningJSON(policy, `provider "p"`)
	if err != nil || string(raw) != `{"match_messages":[]}` || *policy.Declaration.MatchMessages != nil {
		t.Fatal("a declared empty list must encode as an array without mutating its declaration")
	}
	legacy := &EncryptedReasoning{Passthrough: false, OnCallerMismatch: EncryptedReasoningStripAndRetry}
	raw, err = EncodeEncryptedReasoningJSON(legacy, `alias "a"`)
	if err != nil || string(raw) != `{"passthrough":false,"on_caller_mismatch":"strip_and_retry"}` {
		t.Fatalf("legacy full-policy meaning changed: %s %v", raw, err)
	}
	decoded, err := DecodeEncryptedReasoningJSON(raw, `alias "a"`)
	if err != nil || decoded.Passthrough || decoded.OnCallerMismatch != EncryptedReasoningStripAndRetry {
		t.Fatal("legacy explicit false was lost")
	}
}
