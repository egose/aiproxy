package dashboard

import (
	"fmt"

	"github.com/egose/aiproxy/internal/accounting"
)

type measurementViewer interface {
	RateSnapshot() *accounting.RateSnapshot
	BillingSnapshot() *accounting.BillingSnapshot
}

func retainedBilling(usage UsageViewer) *accounting.BillingSnapshot {
	if v, ok := usage.(measurementViewer); ok {
		return v.BillingSnapshot()
	}
	return nil
}

func retainedUpstream(usage UsageViewer) []accounting.UpstreamSummary {
	if b := retainedBilling(usage); b != nil {
		return b.Upstream
	}
	return nil
}

func retentionLabel(usage UsageViewer) string {
	if b := retainedBilling(usage); b != nil && b.RetentionSeconds > 0 {
		return fmt.Sprintf("rolling %gh/%ds", float64(b.RetentionSeconds)/3600, b.BucketSeconds)
	}
	return "window unavailable"
}

func rateLine(usage UsageViewer) string {
	v, ok := usage.(measurementViewer)
	if !ok {
		return "GLOBAL rates/15m graph unavailable (server snapshot lacks rate counters)"
	}
	r := v.RateSnapshot()
	if r == nil {
		return "GLOBAL rates/15m graph unavailable (server snapshot lacks rate counters)"
	}
	errPct := 0.0
	if r.Minute.Requests > 0 {
		errPct = 100 * float64(r.Minute.Errors) / float64(r.Minute.Requests)
	}
	var graph [15]int64
	for i, c := range r.Minutes {
		graph[i] = c.Requests
	}
	return fmt.Sprintf("GLOBAL req/s 1m %.2f 5m %.2f · err/1m %s (%.1f%%) · 429/1m %s · tok/1m %s · req/min 15m→ [%s] · %ds buckets end %s",
		float64(r.Minute.Requests)/60, float64(r.FiveMinutes.Requests)/300,
		comma(r.Minute.Errors), errPct, comma(r.Minute.Throttled), comma(r.Minute.Tokens), sparkline(graph), r.BucketSeconds, r.WindowEnd.Format("15:04:05"))
}

func providerScope(usage UsageViewer) string {
	if r, ok := usage.(*remoteUsage); ok && !r.providerStatsAvailable {
		return "GLOBAL provider counters unavailable"
	}
	return "Providers GLOBAL lifetime"
}

func providerCountersAvailable(usage UsageViewer) bool {
	r, ok := usage.(*remoteUsage)
	return !ok || r.providerStatsAvailable
}
