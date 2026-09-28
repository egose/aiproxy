package dashboard

import (
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/usagecost"
)

func summaryCostText(s accounting.Summary, prices map[string]*config.ModelPricing) string {
	return summaryCostTextWithAliases(s, prices, nil, nil)
}

func summaryCostTextWithAliases(s accounting.Summary, prices map[string]*config.ModelPricing, aliases []config.Alias, upstream []accounting.UpstreamSummary) string {
	cost, ok := summaryCostWithAliases(s, prices, aliases, upstream)
	if !ok {
		return "-"
	}
	return formatCost(cost)
}

func summaryCostWithAliases(s accounting.Summary, prices map[string]*config.ModelPricing, _ []config.Alias, upstream []accounting.UpstreamSummary) (float64, bool) {
	return usagecost.Cost(s, prices, upstream)
}

func providerCostText(provider string, summaries []accounting.Summary, prices map[string]*config.ModelPricing) string {
	return providerCostTextWithAliases(provider, summaries, prices, nil, nil)
}

func providerRowCost(provider string, s accounting.Summary, prices map[string]*config.ModelPricing, _ []config.Alias, upstream []accounting.UpstreamSummary) (float64, bool) {
	return usagecost.ProviderCost(provider, []accounting.Summary{s}, prices, upstream)
}

func providerCostTextWithAliases(provider string, summaries []accounting.Summary, prices map[string]*config.ModelPricing, _ []config.Alias, upstream []accounting.UpstreamSummary) string {
	cost, ok := usagecost.ProviderCost(provider, summaries, prices, upstream)
	if !ok {
		return "-"
	}
	return formatCost(cost)
}
