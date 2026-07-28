package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type geminiRecordedCall struct {
	Path        string
	ContentType string
	APIKey      string
	Body        string
}

type geminiStub struct {
	server       *httptest.Server
	mu           sync.Mutex
	embedBody    string
	responseBody string
	calls        []geminiRecordedCall
}

func newGeminiStub(t *testing.T, embedBody, responseBody string) *geminiStub {
	t.Helper()
	s := &geminiStub{embedBody: embedBody, responseBody: responseBody}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read stub request body: %v", err)
		}
		s.mu.Lock()
		s.calls = append(s.calls, geminiRecordedCall{
			Path:        r.URL.RequestURI(),
			ContentType: r.Header.Get("Content-Type"),
			APIKey:      r.Header.Get("x-goog-api-key"),
			Body:        string(body),
		})
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1beta/models/text-embedding-004:embedContent", "/v1beta/models/text-embedding-004:batchEmbedContents":
			_, _ = w.Write([]byte(s.embedBody))
		case "/v1beta/models/gemini-2.5-pro:generateContent":
			_, _ = w.Write([]byte(s.responseBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *geminiStub) URL() string {
	return s.server.URL
}

func (s *geminiStub) Calls() []geminiRecordedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]geminiRecordedCall, len(s.calls))
	copy(out, s.calls)
	return out
}
