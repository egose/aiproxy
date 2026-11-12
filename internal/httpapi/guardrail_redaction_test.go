package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

type guardrailPayloadAdapter struct {
	countingAdapter
}

func (a *guardrailPayloadAdapter) Do(ctx context.Context, req provider.Request) (*provider.Result, error) {
	result, err := a.countingAdapter.Do(ctx, req)
	if err == nil {
		result.UpstreamRequestHeaders = http.Header{"Content-Type": []string{"application/json"}}
		result.UpstreamRequestBody = append([]byte(nil), req.Body...)
	}
	return result, err
}

func TestGuardrailPayloadLogRedactsEscapedSecrets(t *testing.T) {
	key := guardrailTestKey(t)
	escaped := `\u0041` + key[1:]
	args, _ := json.Marshal(`{"credential":"` + escaped + `"}`)
	rt := newRT()
	providers := append(rt.Catalog.Providers(), config.Provider{
		Type: config.ProviderTypeAnthropic, Name: "anthropic", APIKey: "sk-ant",
		BaseURL: "https://api.anthropic.com",
		Models:  []config.Model{{Name: "claude", UpstreamName: "claude"}},
	})
	rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
	cases := []struct {
		name string
		path string
		op   provider.Operation
		body string
	}{
		{"chat", "/v1/chat/completions", provider.OpChatCompletions, strings.ReplaceAll(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key), key, escaped)},
		{"alias", "/v1/chat/completions", provider.OpChatCompletions, strings.ReplaceAll(guardrailChatBody("alias/chat_default", "deploy with "+key), key, escaped)},
		{"responses", "/v1/responses", provider.OpResponses, `{"model":"openai/gpt-4o-mini","input":"deploy with ` + escaped + `"}`},
		{"messages", "/v1/messages", provider.OpMessages, `{"model":"anthropic/claude","max_tokens":64,"messages":[{"role":"user","content":"deploy with ` + escaped + `"}]}`},
		{"encoded tool arguments", "/v1/chat/completions", provider.OpChatCompletions, `{"model":"openai/gpt-4o-mini","messages":[{"role":"assistant","content":"hello","tool_calls":[{"function":{"name":"deploy","arguments":` + string(args) + `}}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pl, dir := payloadTestLogger(t, 1<<20)
			adapter := &countingAdapter{}
			h := NewHandler(Dependencies{
				Resolver: modelresolver.New(rt), Adapter: adapter,
				Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), Catalog: rt.Catalog,
				Guardrails: guardrailTestScanner(t, guardrails.ModeBlock, 0, 0),
				PayloadLog: pl, RedactGuardrailBodies: true,
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest || guardrailErrorType(t, w.Body.String()) != guardrailBlockType || adapter.count() != 0 {
				t.Fatalf("request was not blocked: status=%d calls=%d body=%s", w.Code, adapter.count(), w.Body.String())
			}
			entries := readPayloadEntries(t, dir)
			if len(entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(entries))
			}
			logged := entries[0].Request.Body.AsString()
			if !strings.Contains(logged, guardrails.DefaultRedactPlaceholder) {
				t.Fatal("logged request is missing its redaction placeholder")
			}
			texts, reason := extractGuardrailTexts("application/json", tc.op, []byte(logged), guardrails.DefaultMaxStrings, guardrails.DefaultMaxTextBytes)
			if reason != "" {
				t.Fatalf("logged body cannot be decoded: %s", reason)
			}
			for _, text := range texts {
				if strings.Contains(text, key) {
					t.Fatal("blocked secret remains recoverable from the payload log")
				}
			}
		})
	}
}

func TestRewriteGuardrailBodyPreservesJSON(t *testing.T) {
	key := guardrailTestKey(t)
	placeholder := `hidden"\value`
	args, _ := json.Marshal(`{"credential":"\u0041` + key[1:] + `","count":9007199254740993}`)
	body := []byte(`{"count":9007199254740993,"text":"` + key + ` and \u0041` + key[1:] + `","arguments":` + string(args) + `}`)
	out := rewriteGuardrailBody(body, map[string]string{key: placeholder})
	var doc struct {
		Count     json.Number `json:"count"`
		Text      string      `json:"text"`
		Arguments string      `json:"arguments"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("rewritten body is not valid JSON: %v", err)
	}
	if doc.Count.String() != "9007199254740993" || doc.Text != placeholder+" and "+placeholder {
		t.Fatalf("number or replacement changed: %+v", doc)
	}
	var nested struct {
		Credential string      `json:"credential"`
		Count      json.Number `json:"count"`
	}
	if err := json.Unmarshal([]byte(doc.Arguments), &nested); err != nil {
		t.Fatalf("rewritten arguments are not valid JSON: %v", err)
	}
	if nested.Credential != placeholder || nested.Count.String() != "9007199254740993" {
		t.Fatalf("nested replacement or number changed: %+v", nested)
	}
}

func TestRewriteGuardrailBodyFailsClosed(t *testing.T) {
	key := guardrailTestKey(t)
	for _, body := range []string{
		`{"content":"` + key,
		`{"content":"clean"}`,
		`{"content":` + strings.Repeat("[", 256) + `"` + key + `"` + strings.Repeat("]", 256) + `}`,
	} {
		if out := rewriteGuardrailBody([]byte(body), map[string]string{key: "REDACTED"}); out != nil {
			t.Fatal("unrewritable body must be omitted")
		}
	}
}

func TestGuardrailCaptureCapCannotClearUnreviewedSecrets(t *testing.T) {
	rt := newRT()
	scanner := guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	var texts []string
	var messages []map[string]string
	for n := 0; n < guardrails.MaxCapturedFindings+2; n++ {
		text := "deploy with " + guardrailTestKey(t)
		texts = append(texts, text)
		messages = append(messages, map[string]string{"role": "user", "content": text})
	}
	res, captured, _, complete := scanner.ScanCapture(context.Background(), texts)
	if complete || res.FindingCount <= len(captured) {
		t.Fatalf("fixture did not exceed the cap: complete=%v total=%d captured=%d", complete, res.FindingCount, len(captured))
	}
	shas := make([]string, 0, len(captured))
	for _, f := range captured {
		shas = append(shas, f.SecretSHA)
	}
	body, _ := json.Marshal(map[string]any{"model": "openai/gpt-4o-mini", "messages": messages})
	for _, mode := range []guardrails.Mode{guardrails.ModeBlock, guardrails.ModeAudit} {
		for _, quarantine := range []bool{false, true} {
			for _, redactLogs := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/quarantine=%t/logs=%t", mode, quarantine, redactLogs), func(t *testing.T) {
					exc := exceptionTestStore(t)
					if _, err := exc.Decide(shas, guardrails.ExceptionActionAllow, "test", nil); err != nil {
						t.Fatal(err)
					}
					adapter := &guardrailPayloadAdapter{}
					q := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: quarantine})
					deps := exceptionTestDeps(t, rt, &adapter.countingAdapter, guardrailTestScanner(t, mode, 0, 0), q, exc)
					deps.Adapter = adapter
					pl, dir := payloadTestLogger(t, 1<<20)
					deps.PayloadLog, deps.RedactGuardrailBodies = pl, redactLogs
					h := NewHandler(deps)
					r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
					r.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					wantStatus, wantCalls := http.StatusBadRequest, 0
					if mode == guardrails.ModeAudit {
						wantStatus, wantCalls = http.StatusOK, 1
					}
					if w.Code != wantStatus || adapter.count() != wantCalls {
						t.Fatalf("status=%d calls=%d, want status=%d calls=%d", w.Code, adapter.count(), wantStatus, wantCalls)
					}
					entries := readPayloadEntries(t, dir)
					if len(entries) != 1 || entries[0].Request.Body.AsString() != "" || entries[0].UpstreamRequest.Body.AsString() != "" {
						t.Fatal("incomplete captures must omit both logged request bodies")
					}
				})
			}
		}
	}
}

func TestGuardrailExceptionsDoNotDependOnPayloadLogging(t *testing.T) {
	key := guardrailTestKey(t)
	body := strings.ReplaceAll(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key), key, `\u0041`+key[1:])
	for _, action := range []string{guardrails.ExceptionActionAllow, guardrails.ExceptionActionRedact} {
		for _, redactLogs := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/logs=%t", action, redactLogs), func(t *testing.T) {
				rt := newRT()
				exc := exceptionTestStore(t)
				if _, err := exc.Decide([]string{guardrails.Fingerprint(key)}, action, "test", nil); err != nil {
					t.Fatal(err)
				}
				adapter := &countingAdapter{}
				deps := exceptionTestDeps(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0), nil, exc)
				pl, dir := payloadTestLogger(t, 1<<20)
				deps.PayloadLog, deps.RedactGuardrailBodies = pl, redactLogs
				h := NewHandler(deps)
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, r)
				if w.Code != http.StatusOK || adapter.count() != 1 {
					t.Fatalf("complete exception must proceed: status=%d calls=%d", w.Code, adapter.count())
				}
				texts, reason := extractGuardrailTexts("application/json", provider.OpChatCompletions, adapter.lastBody(), guardrails.DefaultMaxStrings, guardrails.DefaultMaxTextBytes)
				if reason != "" || len(texts) != 1 {
					t.Fatalf("invalid forwarded body: reason=%s texts=%d", reason, len(texts))
				}
				if strings.Contains(texts[0], key) != (action == guardrails.ExceptionActionAllow) {
					t.Fatal("upstream body does not reflect the exception action")
				}
				entries := readPayloadEntries(t, dir)
				if len(entries) != 1 {
					t.Fatalf("entries = %d", len(entries))
				}
				logged := entries[0].Request.Body.AsString()
				if redactLogs && !strings.Contains(logged, guardrails.DefaultRedactPlaceholder) {
					t.Fatal("the original request must be logged with flagged text masked")
				}
				if !redactLogs && logged != "" {
					t.Fatal("request bodies must remain omitted when redacted logging is disabled")
				}
			})
		}
	}
}

func TestGuardrailUnrewritableBodiesRespectAuditMode(t *testing.T) {
	key := guardrailTestKey(t)
	body := strings.TrimSuffix(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key), "}") + `,"metadata":` + strings.Repeat("[", maxGuardrailRedactionDepth+1) + "0" + strings.Repeat("]", maxGuardrailRedactionDepth+1) + "}"
	for _, mode := range []guardrails.Mode{guardrails.ModeBlock, guardrails.ModeAudit} {
		for _, action := range []string{guardrails.ExceptionActionAllow, guardrails.ExceptionActionRedact} {
			for _, redactLogs := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/logs=%t", mode, action, redactLogs), func(t *testing.T) {
					rt := newRT()
					exc := exceptionTestStore(t)
					if _, err := exc.Decide([]string{guardrails.Fingerprint(key)}, action, "test", nil); err != nil {
						t.Fatal(err)
					}
					adapter := &guardrailPayloadAdapter{}
					deps := exceptionTestDeps(t, rt, &adapter.countingAdapter, guardrailTestScanner(t, mode, 0, 0), nil, exc)
					deps.Adapter = adapter
					pl, dir := payloadTestLogger(t, 1<<20)
					deps.PayloadLog, deps.RedactGuardrailBodies = pl, redactLogs
					h := NewHandler(deps)
					r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
					r.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					blocked := mode == guardrails.ModeBlock && action == guardrails.ExceptionActionRedact
					if blocked {
						if w.Code != http.StatusBadRequest || adapter.count() != 0 || guardrailErrorType(t, w.Body.String()) != guardrailGapType {
							t.Fatal("block-mode redaction failure must not reach upstream")
						}
					} else if w.Code != http.StatusOK || adapter.count() != 1 || string(adapter.lastBody()) != body {
						t.Fatal("allow or audit fallback must preserve upstream forwarding")
					}
					entries := readPayloadEntries(t, dir)
					if len(entries) != 1 || entries[0].Request.Body.AsString() != "" || entries[0].UpstreamRequest.Body.AsString() != "" {
						t.Fatal("unrewritable bodies must be omitted from both request sides")
					}
				})
			}
		}
	}
}

func TestRewriteGuardrailBodyRedactsKeysAndOverlappingSecrets(t *testing.T) {
	key := guardrailTestKey(t)
	body := []byte(`{` + strconv.Quote(key) + `:` + strconv.Quote(key) + `}`)
	out := rewriteGuardrailBody(body, map[string]string{key: "MASKED", key[4:]: "SHORT"})
	var doc map[string]string
	if err := json.Unmarshal(out, &doc); err != nil || len(doc) != 1 || doc["MASKED"] != "MASKED" {
		t.Fatalf("overlapping secrets or key were not fully redacted: err=%v body=%s", err, out)
	}
	collision := []byte(`{` + strconv.Quote(key) + `:"first","MASKED":"second"}`)
	if out := rewriteGuardrailBody(collision, map[string]string{key: "MASKED"}); out != nil {
		t.Fatal("colliding rewritten keys must not discard request fields")
	}
}

func TestGuardrailRedactExceptionUsesFullSecret(t *testing.T) {
	secret := strings.Repeat("sensitive-", 128)
	sha := guardrails.Fingerprint(secret)
	exc := exceptionTestStore(t)
	if _, err := exc.Decide([]string{sha}, guardrails.ExceptionActionRedact, "test", nil); err != nil {
		t.Fatal(err)
	}
	captured := []guardrails.CapturedFinding{{Secret: secret[:guardrails.DefaultQuarantineMaxSnippet], SecretSHA: sha}}
	blocking, redactions := partitionGuardrailFindings(exc, captured, []string{secret})
	if len(blocking) != 0 || len(redactions) != 1 || redactions[secret] != guardrails.DefaultRedactPlaceholder {
		t.Fatal("redaction must use the full secret, not the quarantine snippet")
	}
}
