package copilotlogin

import (
	"net/http"
	"net/textproto"
	"strings"
	"testing"
)

func TestBuildHeadersAllowlist(t *testing.T) {
	body := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}]}`)
	h := BuildHeaders("gho_token", "1.2.3", body)
	if got := h.Get("Authorization"); got != "Bearer gho_token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := h.Get("User-Agent"); got != "aiproxy/1.2.3" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := h.Get(APIVersionHeader); got != APIVersion {
		t.Fatalf("version = %q", got)
	}
	if got := h.Get(IntentHeader); got != OpenAIIntent {
		t.Fatalf("intent = %q", got)
	}
	if got := h.Get(InitiatorHeader); got != InitiatorUser {
		t.Fatalf("initiator = %q", got)
	}
	if got := h.Get(VisionHeader); got != "" {
		t.Fatalf("vision must be omitted without image parts, got %q", got)
	}
	if got := h.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q", got)
	}
	for key := range h {
		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Authorization", "Content-Type", "User-Agent", textproto.CanonicalMIMEHeaderKey(APIVersionHeader), textproto.CanonicalMIMEHeaderKey(IntentHeader), textproto.CanonicalMIMEHeaderKey(InitiatorHeader), "Accept", VisionHeader:
			continue
		default:
			t.Fatalf("unexpected header %q", key)
		}
	}
}

func TestBuildHeadersVisionDerivedFromBody(t *testing.T) {
	bodies := []string{
		`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,xx"}}]}]}`,
		`{"messages":[{"role":"user","content":[{"type":"input_image","data":"xx"}]}]}`,
		`{"messages":[{"role":"user","content":{"image_url":"https://example.com/x.png"}}]}`,
	}
	for _, b := range bodies {
		h := BuildHeaders("tok", "v", []byte(b))
		if got := h.Get(VisionHeader); got != "true" {
			t.Fatalf("vision missing for body %s", b)
		}
	}
	plain := BuildHeaders("tok", "v", []byte(`{"messages":[{"role":"user","content":"tell me about image_url handling"}]}`))
	if got := plain.Get(VisionHeader); got != "" {
		t.Fatalf("text mention of image_url must not set vision, got %q", got)
	}
}

func TestBuildHeadersStreamingAccept(t *testing.T) {
	stream := BuildHeaders("tok", "v", []byte(`{"stream":true,"messages":[]}`))
	if got := stream.Get("Accept"); got != "text/event-stream" {
		t.Fatalf("accept = %q", got)
	}
	plain := BuildHeaders("tok", "v", []byte(`{"messages":[]}`))
	if got := plain.Get("Accept"); got != "application/json" {
		t.Fatalf("accept = %q", got)
	}
}

func TestUserAgentDefaultsVersion(t *testing.T) {
	if got := UserAgent(""); got != "aiproxy/dev" {
		t.Fatalf("ua = %q", got)
	}
}

func TestApplyHeadersOverwritesInboundValues(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer inbound")
	req.Header.Set("Cookie", "s=1")
	req.Header.Set(InitiatorHeader, "agent")
	req.Header.Set(VisionHeader, "true")
	req.Header.Set("X-Interaction-Id", "abc")
	ApplyHeaders(req, "gho_real", "9.9.9", []byte(`{"messages":[]}`))
	if got := req.Header.Get("Authorization"); got != "Bearer gho_real" {
		t.Fatalf("auth = %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "" {
		t.Fatalf("cookie must not survive, got %q", got)
	}
	if got := req.Header.Get("X-Interaction-Id"); got != "" {
		t.Fatalf("interaction id must not survive, got %q", got)
	}
	if got := req.Header.Get(InitiatorHeader); got != InitiatorUser {
		t.Fatalf("initiator = %q", got)
	}
	if got := req.Header.Get(VisionHeader); got != "" {
		t.Fatalf("vision must be derived, got %q", got)
	}
	if strings.Contains(req.Header.Get("Authorization"), "inbound") {
		t.Fatalf("inbound credential leaked")
	}
}
