package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseRetryCooldownMillisPrecedence(t *testing.T) {
	now := time.Now()
	header := http.Header{
		"Retry-After-Ms": []string{"5000"},
		"Retry-After":    []string{"120"},
	}
	delay, ok := ParseRetryCooldown(header, now)
	if !ok || delay != 5*time.Second {
		t.Fatalf("delay = %v, %v; want 5s, true", delay, ok)
	}
}

func TestParseRetryCooldownMillisFormats(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		header http.Header
		delay  time.Duration
		ok     bool
	}{
		{name: "plain", header: msHeader("250"), delay: 250 * time.Millisecond, ok: true},
		{name: "ows", header: msHeader("  \t250 \t"), delay: 250 * time.Millisecond, ok: true},
		{name: "lowercase", header: http.Header{"retry-after-ms": []string{"250"}}, delay: 250 * time.Millisecond, ok: true},
		{name: "upper", header: http.Header{"RETRY-AFTER-MS": []string{"250"}}, delay: 250 * time.Millisecond, ok: true},
		{name: "first valid wins", header: http.Header{"Retry-After-Ms": []string{"abc", "2000"}}, delay: 2 * time.Second, ok: true},
		{name: "first parseable wins", header: http.Header{"Retry-After-Ms": []string{"3000", "2000"}}, delay: 3 * time.Second, ok: true},
		{name: "zero falls back", header: msAndAfter("0", "7"), delay: 7 * time.Second, ok: true},
		{name: "zero alone", header: msHeader("0"), ok: false},
		{name: "negative", header: msHeader("-5"), ok: false},
		{name: "plus sign", header: msHeader("+5"), ok: false},
		{name: "decimal", header: msHeader("1.5"), ok: false},
		{name: "empty", header: msHeader(""), ok: false},
		{name: "spaces only", header: msHeader("   "), ok: false},
		{name: "comma", header: msHeader("5,10"), ok: false},
		{name: "malformed falls back", header: msAndAfter("abc", "9"), delay: 9 * time.Second, ok: true},
		{name: "malformed alone", header: msHeader("abc"), ok: false},
		{name: "absent falls back", header: http.Header{"Retry-After": []string{"9"}}, delay: 9 * time.Second, ok: true},
		{name: "uint64 overflow", header: msHeader("99999999999999999999999"), ok: false},
		{name: "duration overflow", header: msHeader("9223372036855"), ok: false},
		{name: "max duration", header: msHeader("9223372036854"), delay: time.Duration(9223372036854) * time.Millisecond, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, ok := ParseRetryCooldown(tt.header, now)
			if ok != tt.ok || delay != tt.delay {
				t.Fatalf("delay = %v, %v; want %v, %v", delay, ok, tt.delay, tt.ok)
			}
		})
	}
}

func TestParseRetryCooldownRetryAfter(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	future := now.Add(90 * time.Second).UTC().Format(http.TimeFormat)
	past := now.Add(-90 * time.Second).UTC().Format(http.TimeFormat)
	saturated := time.Time{}.Add(time.Duration(1<<63 - 1)).UTC().Format(http.TimeFormat)
	tests := []struct {
		name   string
		header http.Header
		delay  time.Duration
		ok     bool
	}{
		{name: "delay seconds", header: afterHeader("120"), delay: 2 * time.Minute, ok: true},
		{name: "delay ows", header: afterHeader("  30\t"), delay: 30 * time.Second, ok: true},
		{name: "comma first token", header: afterHeader("5, 10"), delay: 5 * time.Second, ok: true},
		{name: "first valid wins", header: http.Header{"Retry-After": []string{"bogus", "12"}}, delay: 12 * time.Second, ok: true},
		{name: "http date", header: afterHeader(future), delay: 90 * time.Second, ok: true},
		{name: "past date", header: afterHeader(past), ok: false},
		{name: "zero", header: afterHeader("0"), ok: false},
		{name: "negative", header: afterHeader("-5"), ok: false},
		{name: "decimal", header: afterHeader("1.5"), ok: false},
		{name: "empty", header: afterHeader(""), ok: false},
		{name: "garbage", header: afterHeader("soon"), ok: false},
		{name: "seconds overflow", header: afterHeader("99999999999999999999999"), ok: false},
		{name: "seconds duration overflow", header: afterHeader("9223372037"), ok: false},
		{name: "date overflow", header: afterHeader(saturated), ok: false},
		{name: "none", header: http.Header{}, ok: false},
		{name: "nil", header: nil, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, ok := ParseRetryCooldown(tt.header, now)
			if ok != tt.ok || delay != tt.delay {
				t.Fatalf("delay = %v, %v; want %v, %v", delay, ok, tt.delay, tt.ok)
			}
		})
	}
}

func TestParseRetryCooldownQuotaExhausted(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		header http.Header
		delay  time.Duration
		ok     bool
	}{
		{name: "tokens exhausted", header: http.Header{"X-Ratelimit-Remaining-Tokens": []string{"0"}}, delay: defaultQuotaCooldown, ok: true},
		{name: "requests exhausted", header: http.Header{"X-Ratelimit-Remaining-Requests": []string{"0"}}, delay: defaultQuotaCooldown, ok: true},
		{name: "lowercase names", header: http.Header{"x-ratelimit-remaining-tokens": []string{"0"}}, delay: defaultQuotaCooldown, ok: true},
		{name: "ows", header: http.Header{"X-Ratelimit-Remaining-Requests": []string{"  0\t"}}, delay: defaultQuotaCooldown, ok: true},
		{name: "tokens remain", header: http.Header{"X-Ratelimit-Remaining-Tokens": []string{"5"}}, ok: false},
		{name: "requests remain", header: http.Header{"X-Ratelimit-Remaining-Requests": []string{"100"}}, ok: false},
		{name: "tokens remain but requests exhausted", header: http.Header{"X-Ratelimit-Remaining-Tokens": []string{"5"}, "X-Ratelimit-Remaining-Requests": []string{"0"}}, delay: defaultQuotaCooldown, ok: true},
		{name: "malformed", header: http.Header{"X-Ratelimit-Remaining-Tokens": []string{"lots"}}, ok: false},
		{name: "negative", header: http.Header{"X-Ratelimit-Remaining-Tokens": []string{"-1"}}, ok: false},
		{name: "explicit ms wins", header: http.Header{"Retry-After-Ms": []string{"2500"}, "X-Ratelimit-Remaining-Tokens": []string{"0"}}, delay: 2500 * time.Millisecond, ok: true},
		{name: "explicit retry-after wins", header: http.Header{"Retry-After": []string{"30"}, "X-Ratelimit-Remaining-Requests": []string{"0"}}, delay: 30 * time.Second, ok: true},
		{name: "invalid retry-after falls back to quota", header: http.Header{"Retry-After": []string{"soon"}, "X-Ratelimit-Remaining-Tokens": []string{"0"}}, delay: defaultQuotaCooldown, ok: true},
		{name: "unrelated headers", header: http.Header{"X-Ratelimit-Limit-Tokens": []string{"1000"}}, ok: false},
		{name: "none", header: http.Header{}, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay, ok := ParseRetryCooldown(tt.header, now)
			if ok != tt.ok || delay != tt.delay {
				t.Fatalf("delay = %v, %v; want %v, %v", delay, ok, tt.delay, tt.ok)
			}
		})
	}
}

func msHeader(value string) http.Header {
	return http.Header{"Retry-After-Ms": []string{value}}
}

func afterHeader(value string) http.Header {
	return http.Header{"Retry-After": []string{value}}
}

func msAndAfter(ms, after string) http.Header {
	return http.Header{"Retry-After-Ms": []string{ms}, "Retry-After": []string{after}}
}

func TestCooldownDelayFromError(t *testing.T) {
	if _, ok := CooldownDelayFromError(nil); ok {
		t.Fatal("nil error reports advice")
	}
	if _, ok := CooldownDelayFromError(fmt.Errorf("boom")); ok {
		t.Fatal("plain error reports advice")
	}
	wrapped := withCooldownError(fmt.Errorf("translate response: bad"), 4*time.Second, true)
	delay, ok := CooldownDelayFromError(wrapped)
	if !ok || delay != 4*time.Second {
		t.Fatalf("delay = %v, %v; want 4s, true", delay, ok)
	}
	if got := wrapped.Error(); got != "translate response: bad" {
		t.Fatalf("error = %q", got)
	}
	if withCooldownError(fmt.Errorf("x"), 0, true) == nil {
		t.Fatal("zero delay must not be wrapped")
	}
}

func TestExecuteUpstreamCapturesAdviceJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After-Ms", "1500")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	}))
	defer server.Close()
	adapter := New()
	res, err := adapter.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  "openai-compatible",
		BaseURL:       server.URL,
		APIKey:        "key",
		UpstreamModel: "m",
		Body:          []byte(`{"model":"m","messages":[]}`),
		Inbound:       httptest.NewRequest(http.MethodPost, "/", nil),
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !res.HasRetryDelay || res.RetryDelay != 1500*time.Millisecond {
		t.Fatalf("result advice = %v, %v", res.RetryDelay, res.HasRetryDelay)
	}
}

func TestExecuteUpstreamCapturesAdviceErrorAndNonJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "6")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`upstream exploded`))
	}))
	defer server.Close()
	adapter := New()
	res, err := adapter.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  "openai-compatible",
		BaseURL:       server.URL,
		APIKey:        "key",
		UpstreamModel: "m",
		Body:          []byte(`{"model":"m","messages":[]}`),
		Inbound:       httptest.NewRequest(http.MethodPost, "/", nil),
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !res.HasRetryDelay || res.RetryDelay != 6*time.Second {
		t.Fatalf("result advice = %v, %v", res.RetryDelay, res.HasRetryDelay)
	}
}

func TestExecuteUpstreamCapturesAdviceTranslatedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "11")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit","message":"slow down"}}`))
	}))
	defer server.Close()
	adapter := New()
	res, err := adapter.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  "anthropic",
		BaseURL:       server.URL,
		APIKey:        "key",
		UpstreamModel: "m",
		Body:          []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`),
		Inbound:       httptest.NewRequest(http.MethodPost, "/", nil),
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res == nil || !res.HasRetryDelay || res.RetryDelay != 11*time.Second {
		t.Fatalf("translated result advice = %+v", res)
	}
	if !strings.Contains(string(res.Body), "slow down") {
		t.Fatalf("translated body = %s", res.Body)
	}
}

func TestExecuteUpstreamCarriesAdviceOnTranslateFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After-Ms", "4000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json-at-all`))
	}))
	defer server.Close()
	adapter := New()
	res, err := adapter.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  "anthropic",
		BaseURL:       server.URL,
		APIKey:        "key",
		UpstreamModel: "m",
		Body:          []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`),
		Inbound:       httptest.NewRequest(http.MethodPost, "/", nil),
	})
	if err == nil {
		t.Fatalf("expected translate error, got %+v", res)
	}
	delay, ok := CooldownDelayFromError(err)
	if !ok || delay != 4*time.Second {
		t.Fatalf("error advice = %v, %v; want 4s, true (%v)", delay, ok, err)
	}
}

func TestExecuteUpstreamCapturesAdviceStreamingHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Retry-After-Ms", "900")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	adapter := New()
	res, err := adapter.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  "openai-compatible",
		BaseURL:       server.URL,
		APIKey:        "key",
		UpstreamModel: "m",
		Body:          []byte(`{"model":"m","stream":true,"messages":[]}`),
		Inbound:       httptest.NewRequest(http.MethodPost, "/", nil),
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !res.Streaming || !res.HasRetryDelay || res.RetryDelay != 900*time.Millisecond {
		t.Fatalf("streaming result advice = %+v", res)
	}
	_ = res.StreamBody.Close()
}

func TestExecuteUpstreamNoAdviceWithoutHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	adapter := New()
	res, err := adapter.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  "openai-compatible",
		BaseURL:       server.URL,
		APIKey:        "key",
		UpstreamModel: "m",
		Body:          []byte(`{"model":"m","messages":[]}`),
		Inbound:       httptest.NewRequest(http.MethodPost, "/", nil),
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if res.HasRetryDelay {
		t.Fatalf("unexpected advice %+v", res)
	}
}

func TestSyntheticCooldownResultHeaders(t *testing.T) {
	res := SyntheticCooldownResult(6 * time.Second)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if got := res.Header.Get("Retry-After"); got != "6" {
		t.Fatalf("Retry-After = %q", got)
	}
	if got := res.Header.Get("Retry-After-Ms"); got != "6000" {
		t.Fatalf("retry-after-ms = %q", got)
	}
	if !strings.Contains(string(res.Body), `"upstream_rate_limited"`) || !strings.Contains(string(res.Body), "retry after 6000ms") {
		t.Fatalf("body = %s", res.Body)
	}
	sub := SyntheticCooldownResult(150 * time.Millisecond)
	if got := sub.Header.Get("Retry-After"); got != "1" {
		t.Fatalf("sub-second Retry-After = %q", got)
	}
	if got := sub.Header.Get("Retry-After-Ms"); got != "150" {
		t.Fatalf("sub-second retry-after-ms = %q", got)
	}
	zero := SyntheticCooldownResult(0)
	if got := zero.Header.Get("Retry-After-Ms"); got != "1" {
		t.Fatalf("zero retry-after-ms = %q", got)
	}
}
