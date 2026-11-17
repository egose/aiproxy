package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func scopedReasoningSingleTarget(rt *config.Runtime) {
	aliases := rt.Catalog.Aliases()
	aliases[0].Targets = aliases[0].Targets[:1]
	rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
}

func scopedReasoningCustomPolicy(patterns []string) *config.EncryptedReasoning {
	declaration := config.DeclaredEncryptedReasoning(scopedReasoningPolicy("retry"))
	declaration.MatchMessages = &patterns
	return config.NewEncryptedReasoning(declaration)
}

func TestScopedEncryptedReasoningRecoveryGates(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, target := range []string{"p1/m", "alias/a"} {
			for _, tc := range []struct {
				name, bodyKind, errorBody string
				status                    int
				custom                    []string
				retry                     bool
			}{
				{name: "recognized", status: 400, errorBody: scopedReasoningMismatch, retry: true},
				{name: "unrelated-400", status: 400, errorBody: `{"error":{"message":"bad max_tokens"}}`},
				{name: "unauthorized", status: 401, errorBody: scopedReasoningMismatch},
				{name: "forbidden", status: 403, errorBody: scopedReasoningMismatch},
				{name: "not-found", status: 404, errorBody: scopedReasoningMismatch},
				{name: "rate-limited", status: 429, errorBody: scopedReasoningMismatch},
				{name: "server-error", status: 500, errorBody: scopedReasoningMismatch},
				{name: "bad-gateway", status: 502, errorBody: scopedReasoningMismatch},
				{name: "unavailable", status: 503, errorBody: scopedReasoningMismatch},
				{name: "empty-error", status: 400},
				{name: "no-opaque", status: 400, errorBody: scopedReasoningMismatch, bodyKind: "plain"},
				{name: "unsigned-typed-item", status: 400, errorBody: scopedReasoningMismatch, bodyKind: "unsigned"},
				{name: "marker-with-unchanged-strip", status: 400, errorBody: scopedReasoningMismatch, bodyKind: "text-marker"},
				{name: "custom-replaces-builtins", status: 400, errorBody: scopedReasoningMismatch, custom: []string{"vendor caller bound"}},
				{name: "custom-case-insensitive", status: 400, errorBody: `{"error":{"message":"VENDOR CALLER BOUND"}}`, custom: []string{"  Vendor Caller Bound  "}, retry: true},
				{name: "empty-custom-falls-back", status: 400, errorBody: scopedReasoningMismatch, custom: []string{}, retry: true},
				{name: "outside-scan-prefix", status: 400, errorBody: `{"padding":"` + strings.Repeat("x", encryptedReasoningScanLimit) + `","error":"not issued to this caller"}`},
			} {
				t.Run(op.String()+"/"+target+"/"+tc.name, func(t *testing.T) {
					rt := scopedReasoningRuntime("retry", "", nil, nil)
					scopedReasoningSingleTarget(rt)
					if tc.custom != nil {
						rt.EncryptedReasoning = scopedReasoningCustomPolicy(tc.custom)
					}
					probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
						if call > 1 || call == 1 && !tc.retry {
							t.Errorf("unexpected inference attempt %d", call+1)
						}
						if call == 1 {
							return scopedReasoningResult(200), nil
						}
						result := scopedReasoningResult(tc.status)
						result.Body = []byte(tc.errorBody)
						return result, nil
					}}
					h, _, clients := scopedReasoningHandler(t, rt, probe)
					ctx := context.Background()
					original, stripped := scopedReasoningBodies(op, target)
					switch tc.bodyKind {
					case "plain", "unsigned", "text-marker":
						content := `"hi"`
						if tc.bodyKind == "text-marker" {
							content = `"ordinary text mentions encrypted_content"`
						}
						if op == provider.OpResponses {
							input := `[{"type":"message","role":"user","content":` + content + `}]`
							if tc.bodyKind == "unsigned" {
								input = `[{"type":"reasoning","summary":[]},{"type":"message","role":"user","content":"hi"}]`
							}
							original = []byte(`{ "model":"` + target + `","input":` + input + `}`)
						} else {
							content = `[{"type":"text","text":` + content + `}]`
							if tc.bodyKind == "unsigned" {
								content = `[{"type":"thinking","thinking":"plain"},{"type":"text","text":"hi"}]`
							}
							original = []byte(`{ "model":"` + target + `","messages":[{"role":"assistant","content":` + content + `}]}`)
						}
					}
					w := scopedReasoningPost(h, ctx, op, original)
					expectedStatus, expectedCalls := tc.status, 1
					if tc.retry {
						expectedStatus, expectedCalls = 200, 2
					}
					attempts := probe.snapshot()
					if w.Code != expectedStatus || len(attempts) != expectedCalls {
						t.Fatalf("status=%d calls=%d; want status=%d calls=%d body=%s", w.Code, len(attempts), expectedStatus, expectedCalls, w.Body.String())
					}
					for i, attempt := range attempts {
						assertScopedReasoningAttempt(t, attempt, rt, clients, ctx, op, target, "p1")
						assertScopedReasoningBody(t, attempt.Request.Body, original, stripped, i == 1)
						if attempt.Closes.Load() != 1 {
							t.Fatalf("attempt %d finalized %d times", i+1, attempt.Closes.Load())
						}
					}
					if !tc.retry && (!bytes.Equal(w.Body.Bytes(), []byte(tc.errorBody)) ||
						strings.Join(w.Header().Values("X-Upstream-Marker"), ",") != "original,second") {
						t.Fatal("non-recovered response bytes or header values changed")
					}
				})
			}
		}
	}
}

func TestScopedEncryptedReasoningCustomPatternsResolvePerTarget(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		rt := scopedReasoningRuntime("strip", "", []string{"retry", "retry"}, nil)
		providers := rt.Catalog.Providers()
		providers[0].EncryptedReasoning = scopedReasoningCustomPolicy([]string{"other vendor marker"})
		providers[1].Models[0].EncryptedReasoning = scopedReasoningCustomPolicy([]string{"  Vendor Caller Bound  "})
		providers[1].ModelByName = nil
		aliases := rt.Catalog.Aliases()
		aliases[0].RetryStatusCodes = []int{400}
		rt.Catalog = config.NewCatalog(providers, nil, aliases)
		probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
			if call > 2 {
				t.Errorf("unexpected inference attempt %d", call+1)
			}
			if call == 2 {
				return scopedReasoningResult(200), nil
			}
			result := scopedReasoningResult(400)
			result.Body = []byte(`{"error":"VENDOR CALLER BOUND"}`)
			return result, nil
		}}
		h, _, clients := scopedReasoningHandler(t, rt, probe)
		original, stripped := scopedReasoningBodies(op, "alias/a")
		ctx := context.Background()
		w := scopedReasoningPost(h, ctx, op, original)
		attempts := probe.snapshot()
		if w.Code != 200 || len(attempts) != 3 {
			t.Fatalf("selected target patterns were not resolved: status=%d calls=%d", w.Code, len(attempts))
		}
		for i, target := range []string{"p1", "p2", "p2"} {
			assertScopedReasoningAttempt(t, attempts[i], rt, clients, ctx, op, "alias/a", target)
			assertScopedReasoningBody(t, attempts[i].Request.Body, original, stripped, i == 2)
		}
	}
}

func TestScopedEncryptedReasoningStripBoundary(t *testing.T) {
	for _, tc := range []struct {
		name               string
		op                 provider.Operation
		original, stripped string
	}{
		{
			name: "chat-unsigned-fields-and-selected-parts", op: provider.OpChatCompletions,
			original: `{"model":"p1/m","reasoning_effort":"high","messages":[{"role":"assistant","reasoning":"plain","reasoning_content":"plain","content":[{"type":"thinking","thinking":"plain"},{"type":"reasoning_text","text":"plain"},{"type":"vendor","data":"compact"},{"type":"text","text":"ordinary encrypted_content text"},{"type":"image_url","image_url":{"url":"https://image.invalid"}}],"tool_calls":[{"id":"1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"1","content":"done"}]}`,
			stripped: `{"model":"p1/m","reasoning_effort":"high","messages":[{"role":"assistant","content":[{"type":"text","text":"ordinary encrypted_content text"},{"type":"image_url","image_url":{"url":"https://image.invalid"}}],"tool_calls":[{"id":"1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"1","content":"done"}]}`,
		},
		{
			name: "responses-only-top-level-markers-on-kept-items", op: provider.OpResponses,
			original: `{"model":"p1/m","reasoning":{"effort":"high"},"input":[{"type":"reasoning","summary":[]},{"type":"thinking","thinking":"plain"},{"type":"redacted_thinking","data":"opaque"},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi","signature":"nested-preserved"}],"signature":"top-removed","encrypted_content":"top-removed"},{"type":"function_call_output","call_id":"1","output":"done","vendor":{"encrypted_content":"nested-preserved"}}]}`,
			stripped: `{"model":"p1/m","reasoning":{"effort":"high"},"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi","signature":"nested-preserved"}]},{"type":"function_call_output","call_id":"1","output":"done","vendor":{"encrypted_content":"nested-preserved"}}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := scopedReasoningRuntime("strip-retry", "", nil, nil)
			probe := &scopedReasoningProbe{do: func(int, context.Context, provider.Request) (*provider.Result, error) {
				return scopedReasoningResult(400), nil
			}}
			h, _, _ := scopedReasoningHandler(t, rt, probe)
			w := scopedReasoningPost(h, context.Background(), tc.op, []byte(tc.original))
			attempts := probe.snapshot()
			if w.Code != 400 || len(attempts) != 1 {
				t.Fatal("unconditional stripping spent a recovery allowance")
			}
			assertScopedReasoningBody(t, attempts[0].Request.Body, []byte(tc.original), []byte(tc.stripped), true)
		})
	}
}

func TestScopedEncryptedReasoningCancellationBeforeExtraDo(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, target := range []string{"p1/m", "alias/a"} {
			for _, stage := range []string{"before-result", "on-discard-close", "deadline-on-discard-close", "during-recovery"} {
				t.Run(op.String()+"/"+target+"/"+stage, func(t *testing.T) {
					rt := scopedReasoningRuntime("retry", "", nil, nil)
					ctx, cancel := context.WithCancel(context.Background())
					if stage == "deadline-on-discard-close" {
						cancel()
						ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
					}
					defer cancel()
					probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
						result := scopedReasoningResult(400)
						switch stage {
						case "before-result":
							cancel()
						case "on-discard-close":
							result.OnClose = cancel
						case "deadline-on-discard-close":
							result.OnClose = func() { <-ctx.Done() }
						case "during-recovery":
							if call == 1 {
								cancel()
								return scopedReasoningResult(503), ctx.Err()
							}
						}
						return result, nil
					}}
					h, recorder, _ := scopedReasoningHandler(t, rt, probe)
					original, _ := scopedReasoningBodies(op, target)
					w := scopedReasoningPost(h, ctx, op, original)
					expectedCalls := 1
					if stage == "during-recovery" {
						expectedCalls = 2
					}
					attempts := probe.snapshot()
					if w.Code != 502 || len(attempts) != expectedCalls || ctx.Err() == nil || len(recorder.Events()) != 1 {
						t.Fatalf("cancellation authorized another attempt: status=%d calls=%d err=%v", w.Code, len(attempts), ctx.Err())
					}
					for _, attempt := range attempts {
						if attempt.Closes.Load() != 1 {
							t.Fatalf("cancellation finalized a result %d times", attempt.Closes.Load())
						}
					}
					metrics := collectMetrics(t, h)
					route02GaugeZero(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
				})
			}
		}
	}
}

func TestScopedEncryptedReasoningOtherOperationsUnchanged(t *testing.T) {
	for _, mode := range []string{"strip", "retry"} {
		for _, target := range []string{"p1/m", "alias/a"} {
			for _, operation := range []struct {
				op         provider.Operation
				path       string
				capability config.Capability
			}{
				{provider.OpMessages, "/v1/messages", config.CapabilityMessages},
				{provider.OpEmbeddings, "/v1/embeddings", config.CapabilityEmbeddings},
				{provider.OpImagesGenerations, "/v1/images/generations", config.CapabilityImages},
				{provider.OpAudioTranscriptions, "/v1/audio/transcriptions", config.CapabilityAudioTranscriptions},
				{provider.OpAudioSpeech, "/v1/audio/speech", config.CapabilityAudioSpeech},
			} {
				t.Run(mode+"/"+target+"/"+operation.op.String(), func(t *testing.T) {
					rt := scopedReasoningRuntime(mode, mode, nil, nil)
					scopedReasoningSingleTarget(rt)
					providers := rt.Catalog.Providers()
					for i := range providers {
						if operation.op == provider.OpMessages {
							providers[i].Type = config.ProviderTypeAnthropic
						}
						providers[i].Models[0].Capabilities = []config.Capability{operation.capability}
						providers[i].ModelByName = nil
					}
					rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
					probe := &scopedReasoningProbe{do: func(int, context.Context, provider.Request) (*provider.Result, error) {
						return scopedReasoningResult(400), nil
					}}
					h, _, _ := scopedReasoningHandler(t, rt, probe)
					body, _ := scopedReasoningBodies(provider.OpChatCompletions, target)
					r := httptest.NewRequest(http.MethodPost, operation.path, bytes.NewReader(body))
					r.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					attempts := probe.snapshot()
					if w.Code != 400 || len(attempts) != 1 || attempts[0].Request.Operation != operation.op {
						t.Fatalf("non-covered operation changed dispatch: status=%d calls=%d body=%s", w.Code, len(attempts), w.Body.String())
					}
					assertScopedReasoningBody(t, attempts[0].Request.Body, body, nil, false)
				})
			}
		}
	}
}

func TestScopedEncryptedReasoningPreservesAdmissionGuards(t *testing.T) {
	for _, target := range []string{"p1/m", "alias/a"} {
		for _, rejection := range []string{"authentication", "authorization", "unresolved", "capability", "guardrail"} {
			t.Run(target+"/"+rejection, func(t *testing.T) {
				rt := scopedReasoningRuntime("strip", "", nil, nil)
				probe := &scopedReasoningProbe{do: func(int, context.Context, provider.Request) (*provider.Result, error) {
					t.Error("rejected admission reached inference")
					return scopedReasoningResult(200), nil
				}}
				h, _, _ := scopedReasoningHandler(t, rt, probe)
				deps := h.current()
				expectedStatus := 400
				original, _ := scopedReasoningBodies(provider.OpChatCompletions, target)
				switch rejection {
				case "authentication":
					deps.Auth = auth.NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic})
					expectedStatus = 401
				case "authorization":
					deps.Authorizer = auth.AuthorizerFunc(func(*auth.Principal, string) bool { return false })
					expectedStatus = 403
				case "unresolved":
					deps.Catalog = config.NewCatalog(nil, nil, nil)
					deps.Resolver = modelresolver.New(&config.Runtime{Catalog: deps.Catalog, EncryptedReasoning: rt.EncryptedReasoning})
					expectedStatus = 404
				case "capability":
					providers := rt.Catalog.Providers()
					for i := range providers {
						providers[i].Models[0].Capabilities = []config.Capability{config.CapabilityEmbeddings}
						providers[i].ModelByName = nil
					}
					rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
					deps.Catalog, deps.Resolver = rt.Catalog, modelresolver.New(rt)
				case "guardrail":
					deps.Guardrails = guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
					deps.Quarantine = guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
					original = bytes.Replace(original, []byte(`"hi"`), []byte(fmt.Sprintf("%q", "deploy with "+guardrailTestKey(t))), 1)
				}
				h.UpdateDependencies(deps)
				w := scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, original)
				if w.Code != expectedStatus || len(probe.snapshot()) != 0 {
					t.Fatalf("policy bypassed %s: status=%d calls=%d body=%s", rejection, w.Code, len(probe.snapshot()), w.Body.String())
				}
			})
		}
	}
}

func TestScopedEncryptedReasoningAliasRestoresOnlyGuardrailAdmittedBody(t *testing.T) {
	rt := scopedReasoningRuntime("", "", []string{"strip", "retry"}, nil)
	statuses := []int{503, 400, 200}
	probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
		if call >= len(statuses) {
			t.Errorf("unexpected inference attempt %d", call+1)
			return scopedReasoningResult(200), nil
		}
		return scopedReasoningResult(statuses[call]), nil
	}}
	h, _, _ := scopedReasoningHandler(t, rt, probe)
	key := guardrailTestKey(t)
	exceptions := exceptionTestStore(t)
	if _, err := exceptions.Decide([]string{guardrails.Fingerprint(key)}, guardrails.ExceptionActionRedact, "reasoning-test", nil); err != nil {
		t.Fatal(err)
	}
	deps := h.current()
	deps.Guardrails = guardrailTestScanner(t, guardrails.ModeBlock, 0, 0)
	deps.Exceptions = exceptions
	deps.Quarantine = guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
	h.UpdateDependencies(deps)
	body, stripped := scopedReasoningBodies(provider.OpChatCompletions, "alias/a")
	body = bytes.Replace(body, []byte(`"hi"`), []byte(fmt.Sprintf("%q", "deploy with "+key)), 1)
	admitted := bytes.Replace(body, []byte(key), []byte(guardrails.DefaultRedactPlaceholder), 1)
	stripped = bytes.Replace(stripped, []byte(`"hi"`), []byte(fmt.Sprintf("%q", "deploy with "+guardrails.DefaultRedactPlaceholder)), 1)
	w := scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, body)
	attempts := probe.snapshot()
	if w.Code != 200 || len(attempts) != 3 {
		t.Fatalf("guardrail/scoped policy sequence failed: status=%d calls=%d body=%s", w.Code, len(attempts), w.Body.String())
	}
	for i, attempt := range attempts {
		expected := stripped
		if i == 1 {
			expected = admitted
		}
		assertScopedReasoningBody(t, attempt.Request.Body, expected, expected, true)
		if bytes.Contains(attempt.Request.Body, []byte(key)) || !bytes.Contains(attempt.Request.Body, []byte(guardrails.DefaultRedactPlaceholder)) {
			t.Fatal("target-local restoration escaped the guardrail-admitted input boundary")
		}
	}
}
