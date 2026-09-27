package dashrpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthenticatedClientCancellationAllPaneOperations(t *testing.T) {
	for _, operation := range []string{"payload-list", "payload-detail", "block-list", "block-detail", "block-decision"} {
		t.Run(operation, func(t *testing.T) {
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get(AuthHeaderName) != AuthScheme+"fixture" {
					t.Error("missing authentication")
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			client := NewClient(server.URL, "fixture")
			defer client.HTTP.CloseIdleConnections()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "payload-list":
					_, err = client.FetchPayloads(ctx, 10, false)
				case "payload-detail":
					_, err = client.FetchPayload(ctx, "id")
				case "block-list":
					_, err = client.FetchBlocks(ctx)
				case "block-detail":
					_, err = client.FetchBlock(ctx, "id")
				case "block-decision":
					_, err = client.DecideBlock(ctx, "id", "deny", []string{"hash"})
				}
				result <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not start")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("lost body-read cancellation: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("body read survived cancellation")
			}
		})
	}
}
