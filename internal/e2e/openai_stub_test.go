package e2e

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type recordedCall struct {
	Path          string
	Authorization string
	ContentType   string
	Body          string
}

type openAIStub struct {
	server    *httptest.Server
	mu        sync.Mutex
	chatBody  string
	embedBody string
	respBody  string
	imageBody string
	audioBody string
	calls     []recordedCall
}

func newOpenAIStub(t *testing.T, chatBody, embedBody, respBody, imageBody, audioBody string) *openAIStub {
	t.Helper()
	s := &openAIStub{
		chatBody:  chatBody,
		embedBody: embedBody,
		respBody:  respBody,
		imageBody: imageBody,
		audioBody: audioBody,
	}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read stub request body: %v", err)
		}
		s.mu.Lock()
		s.calls = append(s.calls, recordedCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			ContentType:   r.Header.Get("Content-Type"),
			Body:          string(body),
		})
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(s.chatBody))
		case "/v1/embeddings":
			_, _ = w.Write([]byte(s.embedBody))
		case "/v1/responses":
			_, _ = w.Write([]byte(s.respBody))
		case "/v1/images/generations":
			_, _ = w.Write([]byte(s.imageBody))
		case "/v1/audio/transcriptions":
			_, _ = w.Write([]byte(s.audioBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *openAIStub) URL() string {
	return s.server.URL
}

func (s *openAIStub) Calls() []recordedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedCall, len(s.calls))
	copy(out, s.calls)
	return out
}
