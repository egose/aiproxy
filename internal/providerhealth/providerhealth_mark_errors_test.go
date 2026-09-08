package providerhealth

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

func scrapeMetrics(t *testing.T, m *observability.Metrics) string {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("metrics status = %d", w.Code)
	}
	return w.Body.String()
}

func TestTrackerMarkSuccessFailureIncrementsCounterOnce(t *testing.T) {
	metrics := observability.NewMetrics()
	failErr := errors.New("write denied")
	tracker := NewWithBackend(metrics, config.ProviderHealth{CacheTTL: defaultCacheTTL}, stubBackend{
		markSuccess: func(context.Context, string) error { return failErr },
		markFailure: func(context.Context, string, time.Duration) error { return failErr },
	})

	tracker.MarkSuccessContext(context.Background(), "openai")
	body := scrapeMetrics(t, metrics)
	if !strings.Contains(body, `aiproxy_provider_health_backend_errors_total{operation="mark_success"} 1`) {
		t.Fatalf("expected mark_success counter 1\n%s", body)
	}
	if strings.Contains(body, `operation="mark_failure"`) {
		t.Fatalf("mark_failure should not increment on mark success\n%s", body)
	}

	tracker.MarkFailureContext(context.Background(), "openai")
	body = scrapeMetrics(t, metrics)
	if !strings.Contains(body, `aiproxy_provider_health_backend_errors_total{operation="mark_failure"} 1`) {
		t.Fatalf("expected mark_failure counter 1\n%s", body)
	}
	if !strings.Contains(body, `aiproxy_provider_health_backend_errors_total{operation="mark_success"} 1`) {
		t.Fatalf("mark_success counter should remain 1\n%s", body)
	}

	tracker.MarkSuccessContext(context.Background(), "openai")
	body = scrapeMetrics(t, metrics)
	if !strings.Contains(body, `aiproxy_provider_health_backend_errors_total{operation="mark_success"} 2`) {
		t.Fatalf("expected mark_success counter 2 after second failure\n%s", body)
	}
}

func TestTrackerMarkSuccessNoErrorDoesNotIncrement(t *testing.T) {
	metrics := observability.NewMetrics()
	tracker := NewWithBackend(metrics, config.ProviderHealth{CacheTTL: defaultCacheTTL}, stubBackend{})
	tracker.MarkSuccessContext(context.Background(), "openai")
	tracker.MarkFailureContext(context.Background(), "openai")
	body := scrapeMetrics(t, metrics)
	if strings.Contains(body, "aiproxy_provider_health_backend_errors_total") {
		t.Fatalf("successful marks should not increment backend error counter\n%s", body)
	}
}

func TestTrackerFailedMarkKeepsCacheAndGaugeSemantics(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	metrics := observability.NewMetrics()
	failErr := errors.New("write denied")
	tracker := NewWithBackend(metrics, config.ProviderHealth{CacheTTL: defaultCacheTTL}, stubBackend{
		markSuccess: func(context.Context, string) error { return failErr },
		markFailure: func(context.Context, string, time.Duration) error { return failErr },
	})
	tracker.now = func() time.Time { return clock }
	tracker.cacheTTL = defaultCacheTTL

	tracker.MarkFailureContext(context.Background(), "openai")
	if healthy, ok := tracker.readCache("openai"); !ok || healthy {
		t.Fatalf("failed mark_failure must still cache unhealthy: healthy=%v ok=%v", healthy, ok)
	}
	if body := scrapeMetrics(t, metrics); !strings.Contains(body, `aiproxy_provider_healthy{name="openai"} 0`) {
		t.Fatalf("failed mark_failure must still set gauge 0\n%s", body)
	}

	tracker.MarkSuccessContext(context.Background(), "openai")
	if healthy, ok := tracker.readCache("openai"); !ok || !healthy {
		t.Fatalf("failed mark_success must still cache healthy: healthy=%v ok=%v", healthy, ok)
	}
	if body := scrapeMetrics(t, metrics); !strings.Contains(body, `aiproxy_provider_healthy{name="openai"} 1`) {
		t.Fatalf("failed mark_success must still set gauge 1\n%s", body)
	}
}

func TestTrackerWriteFailingReadableBackendObservableWithoutReadFailure(t *testing.T) {
	metrics := observability.NewMetrics()
	failErr := errors.New("write denied")
	tracker := NewWithBackend(metrics, config.ProviderHealth{CacheTTL: defaultCacheTTL}, stubBackend{
		markSuccess: func(context.Context, string) error { return failErr },
		markFailure: func(context.Context, string, time.Duration) error { return failErr },
		isHealthy: func(context.Context, string) (bool, error) {
			return true, nil
		},
	})

	tracker.MarkFailureContext(context.Background(), "openai")
	body := scrapeMetrics(t, metrics)
	if !strings.Contains(body, `aiproxy_provider_health_backend_errors_total{operation="mark_failure"} 1`) {
		t.Fatalf("write failure should be observable via mark_failure counter\n%s", body)
	}

	if !tracker.IsHealthyContext(context.Background(), "openai") {
		t.Fatal("readable backend should report healthy without read failure")
	}
	body = scrapeMetrics(t, metrics)
	if strings.Contains(body, `operation="is_healthy"`) || strings.Contains(body, `operation="snapshot"`) {
		t.Fatalf("successful read should not record read backend errors\n%s", body)
	}
	if !strings.Contains(body, `aiproxy_provider_health_backend_errors_total{operation="mark_failure"} 1`) {
		t.Fatalf("mark_failure counter should remain exactly 1 after successful read\n%s", body)
	}
}
