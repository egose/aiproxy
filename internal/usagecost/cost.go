package usagecost

import (
	"strings"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
)

func Cost(s accounting.Summary, prices map[string]*config.ModelPricing, upstream []accounting.UpstreamSummary) (float64, bool) {
	if p := prices[s.Model]; p != nil {
		return ModelCost(p, s.PromptTokens, s.CompletionTokens, s.CachedTokens, s.CacheCreationTokens, s.CacheReadTokens)
	}
	return AliasCost(s, prices, upstream)
}

func ModelCost(p *config.ModelPricing, prompt, completion, cached, write, read int64) (float64, bool) {
	if !p.HasRates() {
		return 0, false
	}
	cached = min(max(cached, 0), max(prompt, 0))
	write = min(max(write, 0), cached)
	read = min(max(read, 0), cached-write)
	if (prompt > cached && p.InputPerMillion <= 0) ||
		(completion > 0 && p.OutputPerMillion <= 0) ||
		(cached > write+read && p.CachedPerMillion <= 0) ||
		(write > 0 && p.CacheWritePerMillion <= 0 && p.InputPerMillion <= 0) ||
		(read > 0 && p.CachedPerMillion <= 0 && p.InputPerMillion <= 0) {
		return 0, false
	}
	return p.Cost(prompt, completion, cached, write, read)
}

func attributed(s accounting.Summary, upstream []accounting.UpstreamSummary) ([]accounting.UpstreamSummary, bool) {
	if !strings.HasPrefix(s.Model, "alias/") {
		return nil, false
	}
	sum := accounting.Summary{Tenant: s.Tenant, Client: s.Client, Model: s.Model, Operation: s.Operation, StatusCode: s.StatusCode}
	var entries []accounting.UpstreamSummary
	for _, u := range upstream {
		if u.PublicModel != s.Model || u.Tenant != s.Tenant || u.Client != s.Client || u.Operation != s.Operation || u.StatusCode != s.StatusCode {
			continue
		}
		entries = append(entries, u)
		sum.Count += u.Count
		sum.PromptTokens += u.PromptTokens
		sum.CompletionTokens += u.CompletionTokens
		sum.TotalTokens += u.TotalTokens
		sum.CachedTokens += u.CachedTokens
		sum.CacheCreationTokens += u.CacheCreationTokens
		sum.CacheReadTokens += u.CacheReadTokens
	}
	return entries, sum == s && sum.Count > 0
}

func AliasCost(s accounting.Summary, prices map[string]*config.ModelPricing, upstream []accounting.UpstreamSummary) (float64, bool) {
	entries, ok := attributed(s, upstream)
	if !ok {
		return 0, false
	}
	var total float64
	for _, u := range entries {
		cost, ok := upstreamCost(u, prices)
		if !ok {
			return 0, false
		}
		total += cost
	}
	return total, true
}

func upstreamCost(u accounting.UpstreamSummary, prices map[string]*config.ModelPricing) (float64, bool) {
	return ModelCost(prices[u.Provider+"/"+u.Model], u.PromptTokens, u.CompletionTokens, u.CachedTokens, u.CacheCreationTokens, u.CacheReadTokens)
}

func ProviderCost(provider string, summaries []accounting.Summary, prices map[string]*config.ModelPricing, upstream []accounting.UpstreamSummary) (float64, bool) {
	var total float64
	used := false
	for _, s := range summaries {
		if strings.HasPrefix(s.Model, "alias/") {
			entries, ok := attributed(s, upstream)
			if !ok {
				return 0, false
			}
			for _, u := range entries {
				if u.Provider != provider {
					continue
				}
				cost, ok := upstreamCost(u, prices)
				if !ok {
					return 0, false
				}
				total += cost
				used = true
			}
		} else if accounting.EventProvider(accounting.Event{Model: s.Model}) == provider && !strings.HasPrefix(s.Model, "_") {
			cost, ok := Cost(s, prices, nil)
			if !ok {
				return 0, false
			}
			total += cost
			used = true
		}
	}
	return total, used
}
