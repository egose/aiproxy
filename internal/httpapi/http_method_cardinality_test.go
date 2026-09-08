package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
)

var httpMethodLabelRe = regexp.MustCompile(`method="([^"]*)"`)

func distinctRequestMethods(t *testing.T, body string) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "aiproxy_http_requests_total{") {
			continue
		}
		m := httpMethodLabelRe.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		seen[m[1]] = true
	}
	return seen
}

func TestHandlerHTTPMethodMetricCardinalityIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name string
		auth config.Auth
	}{
		{name: "auth none", auth: config.Auth{Mode: config.AuthModeNone}},
		{name: "bearer static", auth: config.Auth{
			Mode: config.AuthModeBearerStatic,
			Clients: map[string]config.Client{
				"ci": {Name: "ci", Token: "tok"},
			},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newRT()
			rt.Listener = config.Listener{Address: "127.0.0.1:0"}
			metrics := observability.NewMetrics()
			usage := accounting.NewAggregator()
			health := providerhealth.New(nil, config.ProviderHealth{})
			health.SetProviders(rt.Catalog)
			h := NewHandler(Dependencies{
				Resolver:   modelresolver.New(rt),
				Adapter:    &stubAdapter{},
				Auth:       auth.NewAuthenticator(tc.auth),
				Authorizer: auth.NewAuthorizer(tc.auth),
				Catalog:    rt.Catalog,
				Metrics:    metrics,
				Health:     health,
				Usage:      usage,
				Dashboard:  dashrpc.NewRuntimeSource(config.Dashboard{Token: dashboardTestToken, Enabled: true}, "test", rt.Listener.Address, string(tc.auth.Mode), time.Now(), rt.Catalog, usage, health, nil),
			})

			const extensions = 300
			for i := 0; i < extensions; i++ {
				method := fmt.Sprintf("CUSTOMMETHOD%d", i)
				for _, path := range []string{"/no-such-path", "/healthz", dashrpc.SnapshotPath} {
					w := httptest.NewRecorder()
					r := httptest.NewRequest(method, path, nil)
					h.ServeHTTP(w, r)
				}
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			h.ServeHTTP(w, r)
			w = httptest.NewRecorder()
			r = httptest.NewRequest(http.MethodHead, "/healthz", nil)
			h.ServeHTTP(w, r)

			w = httptest.NewRecorder()
			r = httptest.NewRequest(http.MethodGet, "/metrics", nil)
			metrics.Handler().ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("metrics status = %d", w.Code)
			}
			body := w.Body.String()
			if strings.Contains(body, "CUSTOMMETHOD") {
				t.Fatalf("metrics output contains raw extension method label")
			}
			for _, series := range []string{
				`aiproxy_http_requests_total{method="UNKNOWN"`,
				`aiproxy_http_request_duration_seconds_bucket{method="UNKNOWN"`,
				`aiproxy_http_request_body_bytes_bucket{method="UNKNOWN"`,
				`aiproxy_http_response_body_bytes_bucket{method="UNKNOWN"`,
			} {
				if strings.Contains(body, strings.SplitN(series, "{", 2)[0]) && !strings.Contains(body, series) {
					t.Fatalf("missing UNKNOWN series %s\n%s", series, body)
				}
			}
			if strings.Contains(body, "aiproxy_http_errors_total") {
				found := false
				for _, line := range strings.Split(body, "\n") {
					if strings.HasPrefix(line, "aiproxy_http_errors_total{") && strings.Contains(line, `method="UNKNOWN"`) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("missing UNKNOWN method in aiproxy_http_errors_total\n%s", body)
				}
			}
			if !strings.Contains(body, `aiproxy_http_requests_total{method="UNKNOWN"`) {
				t.Fatalf("missing UNKNOWN request counter series\n%s", body)
			}
			if !strings.Contains(body, `aiproxy_http_request_duration_seconds_bucket{method="UNKNOWN"`) {
				t.Fatalf("missing UNKNOWN request latency histogram series\n%s", body)
			}
			for _, want := range []string{`method="GET"`, `method="HEAD"`} {
				if !strings.Contains(body, want) {
					t.Fatalf("metrics output missing standard label %s\n%s", want, body)
				}
			}
			methods := distinctRequestMethods(t, body)
			if len(methods) > 4 {
				t.Fatalf("unbounded request method labels: %v", methods)
			}
			for method := range methods {
				switch method {
				case "GET", "HEAD", "POST", "UNKNOWN":
				default:
					t.Fatalf("unexpected method label %q", method)
				}
			}
		})
	}
}
