package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type anthropicRecordedCall struct {
	Path    string
	APIKey  string
	Version string
	Body    string
}

type anthropicStub struct {
	server            *httptest.Server
	mu                sync.Mutex
	messageBody       string
	messageStreamBody string
	calls             []anthropicRecordedCall
}

func newAnthropicStub(t *testing.T, messageBody, messageStreamBody string) *anthropicStub {
	t.Helper()
	s := &anthropicStub{messageBody: messageBody, messageStreamBody: messageStreamBody}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read stub request body: %v", err)
		}
		s.mu.Lock()
		s.calls = append(s.calls, anthropicRecordedCall{
			Path:    r.URL.Path,
			APIKey:  r.Header.Get("x-api-key"),
			Version: r.Header.Get("anthropic-version"),
			Body:    string(body),
		})
		s.mu.Unlock()

		switch r.URL.Path {
		case "/v1/messages":
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(s.messageStreamBody))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(s.messageBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *anthropicStub) URL() string { return s.server.URL }

func (s *anthropicStub) Calls() []anthropicRecordedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]anthropicRecordedCall, len(s.calls))
	copy(out, s.calls)
	return out
}
