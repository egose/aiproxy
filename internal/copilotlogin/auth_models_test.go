package copilotlogin

import (
	"net/http"
	"net/textproto"
	"testing"
)

func TestBuildModelsHeadersSharesContract(t *testing.T) {
	h := BuildModelsHeaders("gho_token", "1.2.3")
	if got := h.Get("Authorization"); got != "Bearer gho_token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := h.Get("User-Agent"); got != "aiproxy/1.2.3" {
		t.Fatalf("User-Agent = %q", got)
	}
	if got := h.Get(APIVersionHeader); got != APIVersion {
		t.Fatalf("version = %q", got)
	}
	for key := range h {
		switch textproto.CanonicalMIMEHeaderKey(key) {
		case "Authorization", "User-Agent", textproto.CanonicalMIMEHeaderKey(APIVersionHeader), "Accept":
			continue
		default:
			t.Fatalf("models headers must stay minimal, got %q", key)
		}
	}
	if got := UserAgent("1.2.3"); got != h.Get("User-Agent") {
		t.Fatalf("models User-Agent must share UserAgent helper: %q vs %q", got, h.Get("User-Agent"))
	}
}

func TestApplyModelsHeadersStripsInbound(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com/models", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer inbound")
	req.Header.Set("Cookie", "s=1")
	req.Header.Set("X-Interaction-Id", "abc")
	ApplyModelsHeaders(req, "gho_real", "9.9.9")
	if got := req.Header.Get("Authorization"); got != "Bearer gho_real" {
		t.Fatalf("auth = %q", got)
	}
	if got := req.Header.Get("Cookie"); got != "" {
		t.Fatalf("cookie must not survive")
	}
	if got := req.Header.Get("X-Interaction-Id"); got != "" {
		t.Fatalf("interaction id must not survive")
	}
}
