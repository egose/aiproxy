package observability

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeHTTPMethod(t *testing.T) {
	for _, method := range []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace,
	} {
		if got := NormalizeHTTPMethod(method); got != method {
			t.Fatalf("NormalizeHTTPMethod(%q) = %q, want %q", method, got, method)
		}
	}
	for _, method := range []string{"", "CUSTOM1", "custom", "get", "FOO", "POSTX", "QUERY", "PURGE", "LINK"} {
		if got := NormalizeHTTPMethod(method); got != "UNKNOWN" {
			t.Fatalf("NormalizeHTTPMethod(%q) = %q, want UNKNOWN", method, got)
		}
	}
}

func TestRecordHTTPNormalizesExtensionMethodsAcrossFamilies(t *testing.T) {
	m := NewMetrics()
	for i := 0; i < 200; i++ {
		method := fmt.Sprintf("CUSTOM%d", i)
		m.RecordHTTP(method, "unknown", http.StatusNotFound, 0.001)
		m.RecordHTTPSize(method, "unknown", http.StatusNotFound, 10, 20)
		m.RecordHTTPStream(method, "unknown", http.StatusOK, 0.001)
		m.RecordHTTPError(method, "unknown", http.StatusNotFound, "not_found")
	}
	m.RecordHTTP(http.MethodGet, "unknown", http.StatusNotFound, 0.001)
	m.RecordHTTP(http.MethodPost, "unknown", http.StatusNotFound, 0.001)
	m.RecordHTTP(http.MethodHead, "unknown", http.StatusNotFound, 0.001)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "CUSTOM") {
		t.Fatalf("metrics output contains raw extension method label")
	}
	for _, series := range []string{
		`aiproxy_http_requests_total{method="UNKNOWN"`,
		`aiproxy_http_request_duration_seconds_bucket{method="UNKNOWN"`,
		`aiproxy_http_request_body_bytes_bucket{method="UNKNOWN"`,
		`aiproxy_http_response_body_bytes_bucket{method="UNKNOWN"`,
		`aiproxy_http_stream_responses_total{method="UNKNOWN"`,
		`aiproxy_http_stream_duration_seconds_bucket{method="UNKNOWN"`,
		`aiproxy_http_errors_total{error_type="not_found",method="UNKNOWN"`,
	} {
		if !strings.Contains(body, series) {
			t.Fatalf("metrics output missing UNKNOWN series %s\n%s", series, body)
		}
	}
	for _, method := range []string{"GET", "POST", "HEAD"} {
		if !strings.Contains(body, `method="`+method+`"`) {
			t.Fatalf("metrics output missing standard method %q\n%s", method, body)
		}
	}
}
