package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestCopilotDiscoveryResponseShapes(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		count      int
		failure    bool
	}{
		{"empty_object", `{"data":[]}`, 0, false},
		{"empty_array", `[]`, 0, false},
		{"array", `[{"id":"one","display_name":"One"}]`, 1, false},
		{"object", `{"data":[{"id":"one","display_name":"One"}]}`, 1, false},
		{"array_exact", `[` + strings.Repeat(`{"id":"one","display_name":"One"},`, upstreamModelMaxEntries-1) + `{"id":"one","display_name":"One"}]`, upstreamModelMaxEntries, false},
		{"array_over", `[` + strings.Repeat(`{"id":"one"},`, upstreamModelMaxEntries) + `{"id":"one"}]`, 0, true},
		{"malformed", `[{"id":`, 0, true},
		{"invalid_field", `{"data":42}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, tc.body) }))
			defer srv.Close()
			models, err := listUpstreamModels(context.Background(), discoveryProvider(config.ProviderTypeGitHubCopilot, srv.URL))
			if tc.failure {
				if err == nil || models != nil {
					t.Fatalf("models=%d err=%v", len(models), err)
				}
				return
			}
			if err != nil || len(models) != tc.count {
				t.Fatalf("models=%d err=%v", len(models), err)
			}
			if tc.count > 0 && (models[0].ID != "one" || models[0].DisplayName != "One") {
				t.Fatalf("model=%+v", models[0])
			}
		})
	}
}
