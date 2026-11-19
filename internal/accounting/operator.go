package accounting

import "sort"

func (a *Aggregator) OperatorBillingSnapshot() *BillingSnapshot {
	snapshot := a.BillingSnapshot()
	snapshot.Usage = OperatorSummaries(snapshot.Usage)
	snapshot.Upstream = OperatorUpstreamSummaries(snapshot.Upstream)
	return snapshot
}

func OperatorSummaries(summaries []Summary) []Summary {
	counts := make(map[billingKey]aggregateEntry)
	for _, summary := range summaries {
		key := billingKey{summaryKey: summaryKey{
			tenant: summary.Tenant, client: summary.Client, model: summary.Model,
			operation: summary.Operation, statusCode: summary.StatusCode,
		}}
		counts[key] = counts[key].add(aggregateEntry{
			count: summary.Count, promptTokens: summary.PromptTokens, completionTokens: summary.CompletionTokens,
			totalTokens: summary.TotalTokens, cachedTokens: summary.CachedTokens,
			cacheCreationTokens: summary.CacheCreationTokens, cacheReadTokens: summary.CacheReadTokens,
		})
	}
	out, _ := summarizeBilling(counts)
	return out
}

func OperatorUpstreamSummaries(summaries []UpstreamSummary) []UpstreamSummary {
	counts := make(map[billingKey]aggregateEntry)
	for _, summary := range summaries {
		key := billingKey{summaryKey: summaryKey{
			tenant: summary.Tenant, client: summary.Client, model: summary.PublicModel,
			operation: summary.Operation, statusCode: summary.StatusCode,
		}, provider: summary.Provider, upstreamModel: summary.Model}
		counts[key] = counts[key].add(aggregateEntry{
			count: summary.Count, promptTokens: summary.PromptTokens, completionTokens: summary.CompletionTokens,
			totalTokens: summary.TotalTokens, cachedTokens: summary.CachedTokens,
			cacheCreationTokens: summary.CacheCreationTokens, cacheReadTokens: summary.CacheReadTokens,
		})
	}
	_, out := summarizeBilling(counts)
	return out
}

func (a *Aggregator) OperatorUpstreamSummaries() []UpstreamSummary {
	out := OperatorUpstreamSummaries(a.UpstreamSummaries())
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}
