package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

type stubAdapter struct {
	got    provider.Request
	result *provider.Result
	err    error
}

func (s *stubAdapter) Do(ctx context.Context, r provider.Request) (*provider.Result, error) {
	s.got = r
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"id":"chatcmpl-stub"}`),
	}, nil
}

func newRT() *config.Runtime {
	return &config.Runtime{
		Providers: []config.Provider{
			{
				Type:    config.ProviderTypeOpenAI,
				Name:    "openai",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "sk",
				ModelByName: map[string]config.Model{
					"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"},
				},
				Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
			},
		},
		ProviderByName: map[string]config.Provider{
			"openai": {
				Type:    config.ProviderTypeOpenAI,
				Name:    "openai",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "sk",
				ModelByName: map[string]config.Model{
					"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"},
				},
				Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
			},
		},
		AliasByName: map[string]config.Alias{
			"chat_default": {
				Name:      "chat_default",
				Algorithm: config.AlgorithmRoundRobin,
				Targets:   []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}},
			},
		},
		Aliases: []config.Alias{
			{
				Name:      "chat_default",
				Algorithm: config.AlgorithmRoundRobin,
				Targets:   []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}},
			},
		},
	}
}

func newHandler(t *testing.T, rt *config.Runtime, adapter provider.Adapter) http.Handler {
	t.Helper()
	return NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   adapter,
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:   BuildModelCatalog(rt),
		Metrics:   observability.NewMetrics(),
		Providers: rt.ProviderByName,
	})
}

type denySecondLimiter struct {
	count int32
}

type failFirstProviderAdapter struct {
	calls []string
}

func (a *failFirstProviderAdapter) Do(ctx context.Context, r provider.Request) (*provider.Result, error) {
	a.calls = append(a.calls, r.APIKey)
	if r.APIKey == "bad-key" { // pragma: allowlist secret
		return nil, errors.New("transport failed")
	}
	return &provider.Result{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"id":"chatcmpl_ok"}`)}, nil
}

func (l *denySecondLimiter) Allow(string) (bool, time.Duration) {
	if atomic.AddInt32(&l.count, 1) == 1 {
		return true, 0
	}
	return false, 2 * time.Second
}

func TestHandlerDirectRoute(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, newRT(), stub)

	body := []byte(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.UpstreamModel != "gpt-4o-mini" {
		t.Errorf("upstream model = %q", stub.got.UpstreamModel)
	}
	if stub.got.APIKey != "sk" {
		t.Errorf("api key forwarded = %q", stub.got.APIKey)
	}
	if got := w.Body.String(); got == "" {
		t.Errorf("empty response body")
	}
}

func TestHandlerRewritesModelToUpstream(t *testing.T) {
	rt := newRT()
	rt.ProviderByName["openai"].ModelByName["gpt-4o-mini"] = config.Model{
		Name: "gpt-4o-mini", UpstreamName: "gpt-4o-2024-08-06",
	}
	stub := &stubAdapter{}
	h := newHandler(t, rt, stub)

	body := []byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if stub.got.UpstreamModel != "gpt-4o-2024-08-06" {
		t.Errorf("upstream model = %q, want gpt-4o-2024-08-06", stub.got.UpstreamModel)
	}
}

func TestHandlerAliasRoute(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, newRT(), stub)

	body := []byte(`{"model":"alias/chat_default","messages":[]}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.APIKey != "sk" {
		t.Errorf("alias target credentials not forwarded: %q", stub.got.APIKey)
	}
}

func TestHandlerEmbeddingsRoute(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"object":"list","data":[],"model":"text-embedding-3-large"}`),
	}}
	h := newHandler(t, newRT(), stub)

	body := []byte(`{"model":"openai/gpt-4o-mini","input":"hello"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != provider.OpEmbeddings {
		t.Fatalf("operation = %v, want OpEmbeddings", stub.got.Operation)
	}
	if stub.got.PublicModel != "openai/gpt-4o-mini" {
		t.Fatalf("public model = %q", stub.got.PublicModel)
	}
}

func TestHandlerResponsesRoute(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"object":"response","id":"resp_123"}`),
	}}
	h := newHandler(t, newRT(), stub)

	body := []byte(`{"model":"openai/gpt-4o-mini","input":"hello"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != provider.OpResponses {
		t.Fatalf("operation = %v, want OpResponses", stub.got.Operation)
	}
	if stub.got.PublicModel != "openai/gpt-4o-mini" {
		t.Fatalf("public model = %q", stub.got.PublicModel)
	}
}

func TestHandlerImagesRoute(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"created":123,"data":[{"url":"https://example.com/image.png"}]}`),
	}}
	rt := newRT()
	providerCfg := rt.ProviderByName["openai"]
	providerCfg.ModelByName["gpt-image-1"] = config.Model{Name: "gpt-image-1", UpstreamName: "gpt-image-1", Capabilities: []config.Capability{config.CapabilityImages}}
	providerCfg.Models = append(providerCfg.Models, config.Model{Name: "gpt-image-1", UpstreamName: "gpt-image-1", Capabilities: []config.Capability{config.CapabilityImages}})
	rt.ProviderByName["openai"] = providerCfg
	rt.Providers = []config.Provider{providerCfg}
	h := newHandler(t, rt, stub)

	body := []byte(`{"model":"openai/gpt-image-1","prompt":"a cat"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != provider.OpImagesGenerations {
		t.Fatalf("operation = %v, want OpImagesGenerations", stub.got.Operation)
	}
	if stub.got.PublicModel != "openai/gpt-image-1" {
		t.Fatalf("public model = %q", stub.got.PublicModel)
	}
}

func TestHandlerAudioTranscriptionsRoute(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"text":"hello world"}`),
	}}
	rt := newRT()
	providerCfg := rt.ProviderByName["openai"]
	providerCfg.ModelByName["gpt-4o-transcribe"] = config.Model{Name: "gpt-4o-transcribe", UpstreamName: "gpt-4o-transcribe", Capabilities: []config.Capability{config.CapabilityAudioTranscriptions}}
	providerCfg.Models = append(providerCfg.Models, config.Model{Name: "gpt-4o-transcribe", UpstreamName: "gpt-4o-transcribe", Capabilities: []config.Capability{config.CapabilityAudioTranscriptions}})
	rt.ProviderByName["openai"] = providerCfg
	rt.Providers = []config.Provider{providerCfg}
	h := newHandler(t, rt, stub)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormField("model")
	_, _ = io.WriteString(part, "openai/gpt-4o-transcribe")
	filePart, _ := writer.CreateFormFile("file", "sample.wav")
	_, _ = io.WriteString(filePart, "audio-bytes")
	_ = writer.Close()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != provider.OpAudioTranscriptions {
		t.Fatalf("operation = %v, want OpAudioTranscriptions", stub.got.Operation)
	}
	if stub.got.PublicModel != "openai/gpt-4o-transcribe" {
		t.Fatalf("public model = %q", stub.got.PublicModel)
	}
}

func TestHandlerAudioSpeechRoute(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
		Body:       []byte("mp3-bytes"),
	}}
	rt := newRT()
	providerCfg := rt.ProviderByName["openai"]
	providerCfg.ModelByName["tts-1"] = config.Model{Name: "tts-1", UpstreamName: "tts-1", Capabilities: []config.Capability{config.CapabilityAudioSpeech}}
	providerCfg.Models = append(providerCfg.Models, config.Model{Name: "tts-1", UpstreamName: "tts-1", Capabilities: []config.Capability{config.CapabilityAudioSpeech}})
	rt.ProviderByName["openai"] = providerCfg
	rt.Providers = []config.Provider{providerCfg}
	h := newHandler(t, rt, stub)

	body := []byte(`{"model":"openai/tts-1","input":"hello","voice":"alloy"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != provider.OpAudioSpeech {
		t.Fatalf("operation = %v, want OpAudioSpeech", stub.got.Operation)
	}
	if stub.got.PublicModel != "openai/tts-1" {
		t.Fatalf("public model = %q", stub.got.PublicModel)
	}
}

func TestHandlerRejectsImagesForDefaultOpenAIModel(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, newRT(), stub)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","prompt":"a cat"}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != 0 || stub.got.PublicModel != "" {
		t.Fatalf("adapter should not have been called, got request %+v", stub.got)
	}
}

func TestHandlerRejectsEmbeddingsForChatOnlyModel(t *testing.T) {
	rt := newRT()
	providerCfg := rt.ProviderByName["openai"]
	providerCfg.ModelByName["gpt-4o-mini"] = config.Model{
		Name:         "gpt-4o-mini",
		UpstreamName: "gpt-4o-mini",
		Capabilities: []config.Capability{config.CapabilityChat},
	}
	providerCfg.Models = []config.Model{{
		Name:         "gpt-4o-mini",
		UpstreamName: "gpt-4o-mini",
		Capabilities: []config.Capability{config.CapabilityChat},
	}}
	rt.ProviderByName["openai"] = providerCfg
	rt.Providers = []config.Provider{providerCfg}

	stub := &stubAdapter{}
	h := newHandler(t, rt, stub)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","input":"hello"}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.Operation != 0 || stub.got.PublicModel != "" {
		t.Fatalf("adapter should not have been called, got request %+v", stub.got)
	}
}

func TestHandlerRejectsResponsesForAliasWithoutSharedCapability(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{
			{
				Type:   config.ProviderTypeOpenAI,
				Name:   "openai",
				APIKey: "sk-openai",
				Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat, config.CapabilityResponses}}},
				ModelByName: map[string]config.Model{
					"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat, config.CapabilityResponses}},
				},
			},
			{
				Type:   config.ProviderTypeGemini,
				Name:   "gemini",
				APIKey: "gem-key",
				Models: []config.Model{{Name: "gemini-2.5-pro", UpstreamName: "gemini-2.5-pro", Capabilities: []config.Capability{config.CapabilityChat}}},
				ModelByName: map[string]config.Model{
					"gemini-2.5-pro": {Name: "gemini-2.5-pro", UpstreamName: "gemini-2.5-pro", Capabilities: []config.Capability{config.CapabilityChat}},
				},
			},
		},
		ProviderByName: map[string]config.Provider{},
		Aliases: []config.Alias{{
			Name:      "mixed",
			Algorithm: config.AlgorithmRoundRobin,
			Targets:   []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}, {Provider: "gemini", Model: "gemini-2.5-pro"}},
		}},
		AliasByName: map[string]config.Alias{},
	}
	for _, p := range rt.Providers {
		rt.ProviderByName[p.Name] = p
	}
	for _, a := range rt.Aliases {
		rt.AliasByName[a.Name] = a
	}

	stub := &stubAdapter{}
	h := newHandler(t, rt, stub)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"alias/mixed","input":"hello"}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.PublicModel != "" {
		t.Fatalf("adapter should not have been called, got request %+v", stub.got)
	}
}

func TestHandlerEmbeddingsUnsupportedProvider(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{{
			Type:   config.ProviderTypeAnthropic,
			Name:   "anthropic",
			APIKey: "sk-ant",
			Models: []config.Model{{Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"}},
			ModelByName: map[string]config.Model{
				"claude-sonnet": {Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"},
			},
		}},
		ProviderByName: map[string]config.Provider{
			"anthropic": {
				Type:   config.ProviderTypeAnthropic,
				Name:   "anthropic",
				APIKey: "sk-ant",
				Models: []config.Model{{Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"}},
				ModelByName: map[string]config.Model{
					"claude-sonnet": {Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"},
				},
			},
		},
	}
	h := newHandler(t, rt, provider.New())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader([]byte(`{"model":"anthropic/claude-sonnet","input":"hello"}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Type != "unsupported_operation" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
}

func TestHandlerRejectsResponsesForChatOnlyModel(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{{
			Type:   config.ProviderTypeGemini,
			Name:   "gemini",
			APIKey: "gem-key",
			Models: []config.Model{{Name: "gemini-2.5-pro", UpstreamName: "gemini-2.5-pro", Capabilities: []config.Capability{config.CapabilityChat}}},
			ModelByName: map[string]config.Model{
				"gemini-2.5-pro": {Name: "gemini-2.5-pro", UpstreamName: "gemini-2.5-pro", Capabilities: []config.Capability{config.CapabilityChat}},
			},
		}},
		ProviderByName: map[string]config.Provider{
			"gemini": {
				Type:   config.ProviderTypeGemini,
				Name:   "gemini",
				APIKey: "gem-key",
				Models: []config.Model{{Name: "gemini-2.5-pro", UpstreamName: "gemini-2.5-pro", Capabilities: []config.Capability{config.CapabilityChat}}},
				ModelByName: map[string]config.Model{
					"gemini-2.5-pro": {Name: "gemini-2.5-pro", UpstreamName: "gemini-2.5-pro", Capabilities: []config.Capability{config.CapabilityChat}},
				},
			},
		},
	}
	h := newHandler(t, rt, provider.New())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"gemini/gemini-2.5-pro","input":"hello"}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Type != "unsupported_operation" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
}

func TestHandlerRejectsImagesForUnsupportedProvider(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{{
			Type:   config.ProviderTypeAnthropic,
			Name:   "anthropic",
			APIKey: "sk-ant",
			Models: []config.Model{{Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"}},
			ModelByName: map[string]config.Model{
				"claude-sonnet": {Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"},
			},
		}},
		ProviderByName: map[string]config.Provider{
			"anthropic": {
				Type:   config.ProviderTypeAnthropic,
				Name:   "anthropic",
				APIKey: "sk-ant",
				Models: []config.Model{{Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"}},
				ModelByName: map[string]config.Model{
					"claude-sonnet": {Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"},
				},
			},
		},
	}
	h := newHandler(t, rt, provider.New())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader([]byte(`{"model":"anthropic/claude-sonnet","prompt":"a cat"}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Type != "unsupported_operation" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
}

func TestHandlerUnknownModel(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, newRT(), stub)

	body := []byte(`{"model":"unknown/model","messages":[]}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandlerMissingModelField(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, newRT(), stub)

	body := []byte(`{"messages":[]}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandlerUnknownEndpoint(t *testing.T) {
	h := newHandler(t, newRT(), &stubAdapter{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/foo", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandlerListModelsIncludesProvidersAndAliases(t *testing.T) {
	rt := newRT()
	rt.ProviderByName["openai"] = config.Provider{
		Type:        config.ProviderTypeOpenAI,
		Name:        "openai",
		DisplayName: "OpenAI",
		BaseURL:     "https://api.openai.com/v1",
		APIKey:      "sk",
		ModelByName: map[string]config.Model{
			"gpt-4o-mini": {Name: "gpt-4o-mini", DisplayName: "GPT-4o mini", UpstreamName: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}},
			"gpt-4.1":     {Name: "gpt-4.1", DisplayName: "GPT-4.1", UpstreamName: "gpt-4.1", Capabilities: []config.Capability{config.CapabilityChat, config.CapabilityResponses}},
		},
		Models: []config.Model{
			{Name: "gpt-4o-mini", DisplayName: "GPT-4o mini", UpstreamName: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}},
			{Name: "gpt-4.1", DisplayName: "GPT-4.1", UpstreamName: "gpt-4.1", Capabilities: []config.Capability{config.CapabilityChat, config.CapabilityResponses}},
		},
	}
	rt.Providers = []config.Provider{rt.ProviderByName["openai"]}
	h := newHandler(t, rt, &stubAdapter{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string      `json:"object"`
		Data   []ModelCard `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal models response: %v", err)
	}
	if resp.Object != "list" {
		t.Fatalf("object = %q, want list", resp.Object)
	}
	gotIDs := make([]string, 0, len(resp.Data))
	for _, card := range resp.Data {
		gotIDs = append(gotIDs, card.ID)
		if card.Object != "model" {
			t.Fatalf("model object = %q, want model", card.Object)
		}
	}
	want := []string{"openai/gpt-4o-mini", "openai/gpt-4.1", "alias/chat_default"}
	if len(gotIDs) != len(want) {
		t.Fatalf("models count = %d, want %d (%v)", len(gotIDs), len(want), gotIDs)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("models[%d] = %q, want %q (all=%v)", i, gotIDs[i], want[i], gotIDs)
		}
	}
	if resp.Data[0].OwnedBy != "openai" {
		t.Fatalf("first owned_by = %q, want openai", resp.Data[0].OwnedBy)
	}
	if resp.Data[0].DisplayName != "GPT-4o mini" {
		t.Fatalf("first display_name = %q", resp.Data[0].DisplayName)
	}
	if resp.Data[0].ProviderType != "openai" {
		t.Fatalf("first provider_type = %q", resp.Data[0].ProviderType)
	}
	if len(resp.Data[0].Capabilities) != 1 || resp.Data[0].Capabilities[0] != "chat" {
		t.Fatalf("first capabilities = %+v", resp.Data[0].Capabilities)
	}
	if resp.Data[1].DisplayName != "GPT-4.1" {
		t.Fatalf("second display_name = %q", resp.Data[1].DisplayName)
	}
	if len(resp.Data[1].Capabilities) != 2 || resp.Data[1].Capabilities[1] != "responses" {
		t.Fatalf("second capabilities = %+v", resp.Data[1].Capabilities)
	}
	if resp.Data[2].OwnedBy != "alias" {
		t.Fatalf("alias owned_by = %q, want alias", resp.Data[2].OwnedBy)
	}
	if resp.Data[2].ProviderType != "" {
		t.Fatalf("alias provider_type = %q, want empty", resp.Data[2].ProviderType)
	}
	if len(resp.Data[2].Capabilities) != 1 || resp.Data[2].Capabilities[0] != "chat" {
		t.Fatalf("alias capabilities = %+v", resp.Data[2].Capabilities)
	}
	if len(resp.Data[2].AliasTargets) != 1 {
		t.Fatalf("alias targets = %+v", resp.Data[2].AliasTargets)
	}
	if resp.Data[2].AliasTargets[0].Provider != "openai" || resp.Data[2].AliasTargets[0].Model != "gpt-4o-mini" {
		t.Fatalf("alias target summary = %+v", resp.Data[2].AliasTargets[0])
	}
	if resp.Data[2].AliasTargets[0].DisplayName != "GPT-4o mini" {
		t.Fatalf("alias target display_name = %q", resp.Data[2].AliasTargets[0].DisplayName)
	}
}

func TestHandlerListModelsFiltersUnauthorizedModels(t *testing.T) {
	rt := newRT()
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth: auth.NewAuthenticator(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4o-mini"}},
			},
		}),
		Authorizer: auth.NewAuthorizer(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4o-mini"}},
			},
		}),
		Catalog:   BuildModelCatalog(rt),
		Metrics:   observability.NewMetrics(),
		Providers: rt.ProviderByName,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	r.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []ModelCard `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal models response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("models = %+v", resp.Data)
	}
	if resp.Data[0].ID != "openai/gpt-4o-mini" {
		t.Fatalf("model id = %q", resp.Data[0].ID)
	}
}

func TestHandlerStripsHopByHopHeaders(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"Connection":   []string{"Keep-Alive, X-Remove"},
			"Keep-Alive":   []string{"timeout=5"},
			"Trailer":      []string{"Expires"},
			"Upgrade":      []string{"websocket"},
			"X-Remove":     []string{"gone"},
			"X-Keep":       []string{"ok"},
		},
		Body: []byte(`{"id":"chatcmpl-stub"}`),
	}}
	h := newHandler(t, newRT(), stub)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Connection"); got != "" {
		t.Fatalf("connection header = %q", got)
	}
	if got := w.Header().Get("Keep-Alive"); got != "" {
		t.Fatalf("keep-alive header = %q", got)
	}
	if got := w.Header().Get("Trailer"); got != "" {
		t.Fatalf("trailer header = %q", got)
	}
	if got := w.Header().Get("Upgrade"); got != "" {
		t.Fatalf("upgrade header = %q", got)
	}
	if got := w.Header().Get("X-Remove"); got != "" {
		t.Fatalf("x-remove header = %q", got)
	}
	if got := w.Header().Get("X-Keep"); got != "ok" {
		t.Fatalf("x-keep header = %q", got)
	}
}

func TestHandlerSanitizesUpstreamErrors(t *testing.T) {
	h := newHandler(t, newRT(), &stubAdapter{err: errors.New("dial tcp internal.example:443: connect: connection refused")})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "internal.example") {
		t.Fatalf("response leaked upstream details: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "upstream request failed") {
		t.Fatalf("response = %s", w.Body.String())
	}
}

func TestHandlerNormalizesPlainTextUpstreamErrors(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}, "Vary": []string{"Origin"}},
		Body:       []byte("404 page not found\n"),
	}}
	h := newHandler(t, newRT(), stub)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	if got := w.Header().Get("Vary"); got != "" {
		t.Fatalf("vary header leaked upstream value: %q", got)
	}
	var resp apiError
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error.Type != "upstream_not_found" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
	if resp.Error.Message != "404 page not found" {
		t.Fatalf("error message = %q", resp.Error.Message)
	}
}

func TestHandlerNormalizesStreamingPlainTextUpstreamErrors(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		StreamBody: io.NopCloser(strings.NewReader("404 page not found\n")),
		Streaming:  true,
	}}
	h := newHandler(t, newRT(), stub)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","stream":true,"messages":[]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	var resp apiError
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Error.Type != "upstream_not_found" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
	if resp.Error.Message != "404 page not found" {
		t.Fatalf("error message = %q", resp.Error.Message)
	}
}

func TestHandlerPreservesJSONUpstreamErrors(t *testing.T) {
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"12"}},
		Body:       []byte(`{"error":{"message":"upstream said no","type":"invalid_request_error"}}`),
	}}
	h := newHandler(t, newRT(), stub)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	if got := w.Header().Get("Retry-After"); got != "12" {
		t.Fatalf("retry-after = %q", got)
	}
	if got := strings.TrimSpace(w.Body.String()); got != `{"error":{"message":"upstream said no","type":"invalid_request_error"}}` {
		t.Fatalf("body = %s", got)
	}
}

func TestHandlerHealthEndpoints(t *testing.T) {
	h := newHandler(t, newRT(), &stubAdapter{})
	for _, p := range []string{"/healthz", "/readyz"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, p, nil)
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d", p, w.Code)
		}
		b, _ := io.ReadAll(w.Body)
		if string(b) != "ok" {
			t.Errorf("%s: body = %q", p, string(b))
		}
	}
}

func TestHandlerReadyzFailsWithoutProviders(t *testing.T) {
	rt := &config.Runtime{ProviderByName: map[string]config.Provider{}, AliasByName: map[string]config.Alias{}}
	metrics := observability.NewMetrics()
	metrics.RecordConfig(rt)
	h := NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   &stubAdapter{},
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	h.ServeHTTP(metricsW, metricsReq)
	body := metricsW.Body.String()
	for _, want := range []string{
		"aiproxy_ready 0",
		`aiproxy_ready_reason_info{reason="active_providers"} 0`,
		`aiproxy_ready_reason_info{reason="no_active_providers"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestHandlerReadyzFailsWithoutHealthyProviders(t *testing.T) {
	rt := newRT()
	metrics := observability.NewMetrics()
	metrics.RecordConfig(rt)
	health := providerhealth.New(metrics, config.ProviderHealth{})
	health.SetProviders(rt.ProviderByName)
	health.MarkFailure("openai")
	h := NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   &stubAdapter{},
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
		Health:    health,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	h.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	h.ServeHTTP(metricsW, metricsReq)
	body := metricsW.Body.String()
	for _, want := range []string{
		"aiproxy_ready 0",
		`aiproxy_ready_reason_info{reason="active_providers"} 0`,
		`aiproxy_ready_reason_info{reason="no_healthy_providers"} 1`,
		`aiproxy_provider_healthy{name="openai"} 0`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestHandlerMetricsEndpointAndCounters(t *testing.T) {
	rt := newRT()
	metrics := observability.NewMetrics()
	metrics.RecordConfig(rt)
	stub := &stubAdapter{}
	h := NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   stub,
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:   BuildModelCatalog(rt),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
	})

	post := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)))
	postW := httptest.NewRecorder()
	h.ServeHTTP(postW, post)
	if postW.Code != http.StatusOK {
		t.Fatalf("chat status = %d", postW.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	h.ServeHTTP(metricsW, metricsReq)
	if metricsW.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", metricsW.Code)
	}
	body := metricsW.Body.String()
	for _, want := range []string{
		`aiproxy_http_requests_total{method="POST",path="/v1/chat/completions",status="200"} 1`,
		`aiproxy_http_request_duration_seconds_count{method="POST",path="/v1/chat/completions",status="200"} 1`,
		`aiproxy_http_request_body_bytes_count{method="POST",path="/v1/chat/completions"} 1`,
		`aiproxy_http_response_body_bytes_count{method="POST",path="/v1/chat/completions",status="200"} 1`,
		`aiproxy_usage_events_total{client="anonymous",model="openai/gpt-4o-mini",operation="chat_completions",status="200",tenant="anonymous"} 1`,
		"aiproxy_active_providers 1",
		"aiproxy_disabled_providers 0",
		"aiproxy_aliases 1",
		"aiproxy_ready 1",
		`aiproxy_ready_reason_info{reason="active_providers"} 1`,
		`aiproxy_ready_reason_info{reason="no_active_providers"} 0`,
		`aiproxy_provider_selections_total{model="gpt-4o-mini",operation="chat_completions",provider="openai",public_model="openai/gpt-4o-mini"} 1`,
		`aiproxy_upstream_requests_total{operation="chat_completions",outcome="success",provider="openai"} 1`,
		`aiproxy_upstream_request_duration_seconds_count{operation="chat_completions",outcome="success",provider="openai"} 1`,
		`aiproxy_upstream_response_body_bytes_count{operation="chat_completions",outcome="success",provider="openai"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestHandlerAliasSkipsUnhealthyProviderAfterTransientFailure(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{
			{Type: config.ProviderTypeOpenAI, Name: "primary", APIKey: "bad-key", BaseURL: "https://x", Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}}, ModelByName: map[string]config.Model{"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}}},
			{Type: config.ProviderTypeOpenAI, Name: "backup", APIKey: "good-key", BaseURL: "https://x", Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}}, ModelByName: map[string]config.Model{"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}}},
		},
		ProviderByName: map[string]config.Provider{},
		Aliases:        []config.Alias{{Name: "chat_default", Algorithm: config.AlgorithmLeastConnections, Targets: []config.AliasTarget{{Provider: "primary", Model: "gpt-4o-mini"}, {Provider: "backup", Model: "gpt-4o-mini"}}}},
		AliasByName:    map[string]config.Alias{},
	}
	for _, p := range rt.Providers {
		rt.ProviderByName[p.Name] = p
	}
	for _, a := range rt.Aliases {
		rt.AliasByName[a.Name] = a
	}
	metrics := observability.NewMetrics()
	health := providerhealth.New(metrics, config.ProviderHealth{})
	health.SetProviders(rt.ProviderByName)
	adapter := &failFirstProviderAdapter{}
	h := NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   adapter,
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:   BuildModelCatalog(rt),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
		Health:    health,
	})

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"alias/chat_default","messages":[]}`)))
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d status = %d body=%s", i, w.Code, w.Body.String())
		}
	}
	if len(adapter.calls) != 3 {
		t.Fatalf("calls = %+v", adapter.calls)
	}
	if adapter.calls[0] != "bad-key" || adapter.calls[1] != "good-key" || adapter.calls[2] != "good-key" {
		t.Fatalf("unexpected call sequence: %+v", adapter.calls)
	}
}

func TestHandlerMetricsIncludeProxyErrors(t *testing.T) {
	rt := newRT()
	metrics := observability.NewMetrics()
	metrics.RecordConfig(rt)
	h := NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   &stubAdapter{},
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:   BuildModelCatalog(rt),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
	})

	badReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hi"}]}`)))
	badW := httptest.NewRecorder()
	h.ServeHTTP(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("bad request status = %d", badW.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	h.ServeHTTP(metricsW, metricsReq)
	if metricsW.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", metricsW.Code)
	}
	body := metricsW.Body.String()
	for _, want := range []string{
		`aiproxy_http_errors_total{error_type="invalid_model",method="POST",path="/v1/chat/completions",status="400"} 1`,
		`aiproxy_http_requests_total{method="POST",path="/v1/chat/completions",status="400"} 1`,
		`aiproxy_http_request_body_bytes_count{method="POST",path="/v1/chat/completions"} 1`,
		`aiproxy_http_response_body_bytes_count{method="POST",path="/v1/chat/completions",status="400"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestHandlerMetricsIncludeStreamingResponses(t *testing.T) {
	rt := newRT()
	metrics := observability.NewMetrics()
	metrics.RecordConfig(rt)
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Streaming:  true,
		StreamBody: io.NopCloser(strings.NewReader("data: hello\n\ndata: [DONE]\n\n")),
	}}
	h := NewHandler(Dependencies{
		Resolver:  modelresolver.New(rt),
		Adapter:   stub,
		Auth:      auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:   BuildModelCatalog(rt),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","stream":true,"messages":[]}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("streaming status = %d", w.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsW := httptest.NewRecorder()
	h.ServeHTTP(metricsW, metricsReq)
	if metricsW.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", metricsW.Code)
	}
	body := metricsW.Body.String()
	for _, want := range []string{
		`aiproxy_http_stream_responses_total{method="POST",path="/v1/chat/completions",status="200"} 1`,
		`aiproxy_http_stream_duration_seconds_count{method="POST",path="/v1/chat/completions",status="200"} 1`,
		`aiproxy_upstream_response_body_bytes_count{operation="chat_completions",outcome="success",provider="openai"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestHandlerBearerAuthRejects(t *testing.T) {
	rt := newRT()
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth: auth.NewAuthenticator(config.Auth{
			Mode:    config.AuthModeBearerStatic,
			Clients: map[string]config.Client{"ci": {Token: "tok"}},
		}),
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini"}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestHandlerAuthorizerRejectsForbiddenModel(t *testing.T) {
	rt := newRT()
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth: auth.NewAuthenticator(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4.1"}},
			},
		}),
		Authorizer: auth.NewAuthorizer(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4.1"}},
			},
		}),
		Metrics:   observability.NewMetrics(),
		Providers: rt.ProviderByName,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	r.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Type != "forbidden" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
}

func TestHandlerAccountingRecordsTenantClientModelAndStatus(t *testing.T) {
	rt := newRT()
	recorder := &accounting.MemoryRecorder{}
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth: auth.NewAuthenticator(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", Tenant: "team-a", AllowedModels: []string{"openai/gpt-4o-mini"}},
			},
		}),
		Authorizer: auth.NewAuthorizer(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", Tenant: "team-a", AllowedModels: []string{"openai/gpt-4o-mini"}},
			},
		}),
		Catalog:    BuildModelCatalog(rt),
		Metrics:    observability.NewMetrics(),
		Providers:  rt.ProviderByName,
		Accounting: recorder,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	r.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].Tenant != "team-a" || events[0].Client != "ci" || events[0].Model != "openai/gpt-4o-mini" || events[0].Operation != "chat_completions" || events[0].StatusCode != http.StatusOK {
		t.Fatalf("event = %+v", events[0])
	}
}

func TestHandlerAccountingCollapsesUnknownModels(t *testing.T) {
	rt := newRT()
	usage := accounting.NewAggregator()
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    &stubAdapter{},
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    BuildModelCatalog(rt),
		Metrics:    observability.NewMetrics(),
		Providers:  rt.ProviderByName,
		Accounting: usage,
	})

	for _, model := range []string{"unknown/model-a", "unknown/model-b"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"`+model+`","messages":[]}`)))
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
		}
	}

	summaries := usage.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[0].Model != accountingModelNotFound {
		t.Fatalf("summary model = %q", summaries[0].Model)
	}
	if summaries[0].Count != 2 {
		t.Fatalf("summary count = %d", summaries[0].Count)
	}
	if summaries[0].StatusCode != http.StatusNotFound {
		t.Fatalf("summary status = %d", summaries[0].StatusCode)
	}
}

func TestHandlerAccountingCollapsesForbiddenModels(t *testing.T) {
	rt := newRT()
	usage := accounting.NewAggregator()
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth: auth.NewAuthenticator(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4.1"}},
			},
		}),
		Authorizer: auth.NewAuthorizer(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", AllowedModels: []string{"openai/gpt-4.1"}},
			},
		}),
		Metrics:    observability.NewMetrics(),
		Providers:  rt.ProviderByName,
		Accounting: usage,
	})

	for _, model := range []string{"openai/gpt-4o-mini", "alias/chat_default"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"`+model+`","messages":[]}`)))
		r.Header.Set("Authorization", "Bearer tok")
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
		}
	}

	summaries := usage.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[0].Model != accountingModelForbidden {
		t.Fatalf("summary model = %q", summaries[0].Model)
	}
	if summaries[0].Count != 2 {
		t.Fatalf("summary count = %d", summaries[0].Count)
	}
	if summaries[0].StatusCode != http.StatusForbidden {
		t.Fatalf("summary status = %d", summaries[0].StatusCode)
	}
}

func TestHandlerBillingUsageFiltersToTenant(t *testing.T) {
	rt := newRT()
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200})
	usage.Record(accounting.Event{Tenant: "team-b", Client: "ops", Model: "openai/gpt-4.1", Operation: "responses", StatusCode: 200})
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  &stubAdapter{},
		Auth: auth.NewAuthenticator(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", Tenant: "team-a"},
			},
		}),
		Authorizer: auth.NewAuthorizer(config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok", Tenant: "team-a"},
			},
		}),
		Metrics:   observability.NewMetrics(),
		Providers: rt.ProviderByName,
		Usage:     usage,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/billing/usage", nil)
	r.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Object string               `json:"object"`
		Data   []accounting.Summary `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal billing usage: %v", err)
	}
	if resp.Object != "list" || len(resp.Data) != 1 {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Data[0].Tenant != "team-a" || resp.Data[0].Client != "ci" {
		t.Fatalf("filtered summary = %+v", resp.Data[0])
	}
}

func TestHandlerRateLimitRejectsWithRetryAfter(t *testing.T) {
	rt := newRT()
	h := NewHandler(Dependencies{
		Resolver:    modelresolver.New(rt),
		Adapter:     &stubAdapter{},
		Auth:        auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:     BuildModelCatalog(rt),
		Metrics:     observability.NewMetrics(),
		Providers:   rt.ProviderByName,
		RateLimiter: &denySecondLimiter{},
	})

	firstW := httptest.NewRecorder()
	firstR := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(firstW, firstR)
	if firstW.Code != http.StatusOK {
		t.Fatalf("first status = %d", firstW.Code)
	}

	secondW := httptest.NewRecorder()
	secondR := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(secondW, secondR)
	if secondW.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, body=%s", secondW.Code, secondW.Body.String())
	}
	if secondW.Header().Get("Retry-After") != "2" {
		t.Fatalf("retry-after = %q", secondW.Header().Get("Retry-After"))
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(secondW.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Type != "rate_limited" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
}

func TestHandlerStreamingPassthroughFlushesIncrementally(t *testing.T) {
	pr, pw := io.Pipe()
	stub := &stubAdapter{
		result: &provider.Result{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Streaming:  true,
			StreamBody: pr,
		},
	}
	h := newHandler(t, newRT(), stub)
	server := httptest.NewServer(h)
	defer server.Close()

	firstChunkWritten := make(chan struct{})
	allowSecondChunk := make(chan struct{})
	go func() {
		_, _ = io.WriteString(pw, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		close(firstChunkWritten)
		<-allowSecondChunk
		_, _ = io.WriteString(pw, "data: [DONE]\n\n")
		_ = pw.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","stream":true,"messages":[]}`)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		lineCh <- line
	}()

	select {
	case <-firstChunkWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk to be written")
	}

	select {
	case line := <-lineCh:
		if line != "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n" {
			t.Fatalf("unexpected first SSE line: %q", line)
		}
	case err := <-errCh:
		t.Fatalf("read first stream line: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first streamed line; handler likely buffered instead of flushing")
	}

	close(allowSecondChunk)
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read rest of stream: %v", err)
	}
	if !bytes.Contains(body, []byte("[DONE]")) {
		t.Fatalf("expected remaining stream body to include [DONE], got %q", string(body))
	}
	if !resp.ProtoAtLeast(1, 1) {
		t.Fatalf("unexpected response protocol: %s", resp.Proto)
	}
}

func TestHandlerInvalidTranslatedRequestReturns400(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{{
			Type:   config.ProviderTypeAnthropic,
			Name:   "anthropic",
			APIKey: "sk-ant",
			Models: []config.Model{{Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"}},
			ModelByName: map[string]config.Model{
				"claude-sonnet": {Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"},
			},
		}},
		ProviderByName: map[string]config.Provider{},
		AliasByName:    map[string]config.Alias{},
	}
	rt.ProviderByName["anthropic"] = rt.Providers[0]
	h := newHandler(t, rt, provider.New())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"anthropic/claude-sonnet","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}]}]}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Error.Type != "invalid_request" {
		t.Fatalf("error type = %q", resp.Error.Type)
	}
}

func TestCloseResultDefersOnCloseUntilStreamEnds(t *testing.T) {
	pr, pw := io.Pipe()
	var closed int32
	stub := &stubAdapter{result: &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Streaming:  true,
		StreamBody: pr,
		OnClose: func() {
			atomic.StoreInt32(&closed, 1)
		},
	}}
	h := newHandler(t, newRT(), stub)
	server := httptest.NewServer(h)
	defer server.Close()

	allowClose := make(chan struct{})
	go func() {
		_, _ = io.WriteString(pw, "data: hello\n\n")
		<-allowClose
		_ = pw.Close()
	}()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","stream":true,"messages":[]}`)))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(resp.Body)
	_, err = reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first line: %v", err)
	}
	if atomic.LoadInt32(&closed) != 0 {
		t.Fatalf("OnClose fired before stream completion")
	}
	close(allowClose)
	_, _ = io.ReadAll(reader)
	_ = resp.Body.Close()
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&closed) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if atomic.LoadInt32(&closed) == 0 {
		t.Fatalf("OnClose did not fire after stream completion")
	}
}
