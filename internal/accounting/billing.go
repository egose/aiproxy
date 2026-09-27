package accounting

import "time"

type BillingSnapshot struct {
	AsOf             time.Time         `json:"as_of"`
	RetentionSeconds int64             `json:"retention_seconds"`
	BucketSeconds    int64             `json:"bucket_seconds"`
	Usage            []Summary         `json:"usage"`
	Upstream         []UpstreamSummary `json:"upstream"`
}

func (a *Aggregator) BillingSnapshot() *BillingSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.nowTime(time.Time{})
	a.pruneLocked(now)
	counts := make(map[billingKey]aggregateEntry)
	for _, bucket := range a.buckets {
		for key, entry := range bucket {
			counts[key] = counts[key].add(entry)
		}
	}
	usage, upstream := summarizeBilling(counts)
	return &BillingSnapshot{AsOf: now, RetentionSeconds: int64(a.retention / time.Second),
		BucketSeconds: int64(a.effectiveBucketDuration() / time.Second), Usage: usage, Upstream: upstream}
}
