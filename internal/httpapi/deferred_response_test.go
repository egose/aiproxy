package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
)

func TestDeferredReservedResponseHeadersStrippedWithoutOptIn(t *testing.T) {
	for _, tc := range []struct {
		status    int
		streaming bool
	}{
		{http.StatusOK, false},
		{http.StatusForbidden, false},
		{http.StatusOK, true},
	} {
		t.Run(fmt.Sprintf("status=%d/stream=%t", tc.status, tc.streaming), func(t *testing.T) {
			body := []byte(`{"vendor":"exact"}`)
			contentType := "application/json"
			result := &provider.Result{
				StatusCode: tc.status,
				Header: http.Header{
					"X-Aiproxy-Deferred":         {"private"},
					"x-AIPROXY-DeFeRrEd-Version": {"1"},
					"X-Aiproxy-Deferred-Unknown": {"private"},
					"X-Aiproxy-Deferredish":      {"not-reserved"},
					"X-Vendor":                   {"original", "second"},
				},
			}
			if tc.streaming {
				body, contentType = []byte("id: original\ndata: exact\n\n"), "text/event-stream"
				result.Streaming, result.StreamBody = true, io.NopCloser(bytes.NewReader(body))
			} else {
				result.Body = body
			}
			result.Header.Set("Content-Type", contentType)
			handler := newHandler(t, newRT(), &stubAdapter{result: result})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
			handler.ServeHTTP(response, request)
			for name := range response.Header() {
				if config.IsDeferredHeader(name) {
					t.Fatalf("ordinary inference leaked reserved response header %q", name)
				}
			}
			if response.Code != tc.status || !bytes.Equal(response.Body.Bytes(), body) ||
				response.Header().Get("Content-Type") != contentType || response.Header().Get("X-Aiproxy-Deferredish") != "not-reserved" ||
				len(response.Header().Values("X-Vendor")) != 2 {
				t.Fatal("private-header filtering changed ordinary status/body/non-reserved headers")
			}
		})
	}
}
