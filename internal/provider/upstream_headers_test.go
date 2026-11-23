package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestUpstreamHeadersPreserveAdapterHeadersAndMaskOnlyApplied(t *testing.T) {
	inbound := httptest.NewRequest(http.MethodPost, "/", nil)
	inbound.Header["X-Shared"] = []string{"original", "repeated"}
	inbound.Header["X-Caller"] = []string{"unchanged", "also-unchanged"}
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.Header = http.Header{
		"x-adapter-empty": {""},
		"x-adapter-owned": {"adapter-value"},
	}
	r := Request{
		ProviderType:   config.ProviderTypeOpenRouter,
		Inbound:        inbound,
		ForwardHeaders: []string{"X-SHARED", "X-Caller"},
		UpstreamHeaders: map[string]string{
			"x-shared":             "fixed-synthetic-value",
			"X-Adapter-Owned":      "must-not-overwrite",
			"X-Adapter-Empty":      "must-not-fill",
			"x-title":              "adapter-owned-even-if-unset",
			"AUTHORIZATION":        "proxy-owned-even-if-unset",
			"Accept-Encoding":      "proxy-owned-even-if-unset",
			"X-Aiproxy-Deferred-Y": "private-even-if-unset",
		},
	}
	applied, err := prepareUpstreamHeaders(r, request)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotUpstreamHeaders(request.Header, applied)
	expected := http.Header{
		"X-Shared":        {"fixed-synthetic-value"},
		"X-Caller":        {"unchanged", "also-unchanged"},
		"x-adapter-empty": {""},
		"x-adapter-owned": {"adapter-value"},
		"User-Agent":      {"aiproxy/dev"},
	}
	for name, want := range expected {
		if !slices.Equal(request.Header[name], want) {
			t.Errorf("wire header %s = %q, want %q", name, request.Header[name], want)
		}
		if name == "X-Shared" {
			want = []string{"[REDACTED]"}
		}
		if !slices.Equal(snapshot[name], want) {
			t.Errorf("snapshot header %s = %q, want %q", name, snapshot[name], want)
		}
	}
	if len(request.Header) != len(expected) || len(snapshot) != len(expected) {
		t.Fatal("adapter headers with another spelling were duplicated, or a reserved field was set")
	}
}

func TestUpstreamHeadersCountTowardBoundedInferenceMetadata(t *testing.T) {
	budget := &executionTestBudget{limit: 4 << 20}
	r := executionRequestFor(executionProviderCase{provider: config.ProviderTypeOpenAICompatible}, budget, false)
	value := strings.Repeat("x", 4000)
	r.UpstreamHeaders = map[string]string{"X-First": value, "X-Second": value, "X-Third": value, "X-Fourth": value}
	r.ForwardHeaders = []string{"X-Allowed"}
	r.Inbound.Header.Set("X-Allowed", strings.Repeat("y", 1024))
	r.Client = &http.Client{Transport: executionRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Error("over-limit outbound headers reached transport")
		return nil, errors.New("unexpected upstream call")
	})}
	result, err := New().Do(context.Background(), r)
	var limit ErrExecutionLimit
	if result != nil || !errors.As(err, &limit) || limit.Kind != "upstream request headers" || strings.Contains(err.Error(), value) {
		t.Fatal("combined metadata was not rejected without exposing fixed values")
	}
	budget.awaitReleased(t)
}
