package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
)

func TestFormatCostTiers(t *testing.T) {
	cases := map[float64]string{
		0:         "$0.00",
		0.0000123: "$0.0000",
		0.004:     "$0.0040",
		1.234:     "$1.23",
		45.67:     "$45.67",
		1234.5:    "$1,235",
	}
	for in, want := range cases {
		if got := formatCost(in); got != want {
			t.Errorf("formatCost(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSummaryCostTextPricedAndUnpriced(t *testing.T) {
	prices := map[string]*config.ModelPricing{
		"openai/gpt-4o-mini": {InputPerMillion: 1, OutputPerMillion: 2, CachedPerMillion: 0.5},
	}
	s := accounting.Summary{Model: "openai/gpt-4o-mini", PromptTokens: 1000000, CompletionTokens: 1000000, CachedTokens: 500000, CacheReadTokens: 500000}
	if got := summaryCostText(s, prices); got != "$2.75" {
		t.Errorf("cost = %q, want $2.75 (0.5*1 + 1*2 + 0.5*0.5)", got)
	}
	if got := summaryCostText(accounting.Summary{Model: "other/m"}, prices); got != "-" {
		t.Errorf("unpriced = %q, want -", got)
	}
	if got := summaryCostText(s, nil); got != "-" {
		t.Errorf("no prices = %q, want -", got)
	}
}

func TestAliasCostAttributesByUpstreamWeight(t *testing.T) {
	prices := map[string]*config.ModelPricing{
		"dxc-openshift/muse-spark-1.3-contributor-free":  {InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002},
		"gold-openshift/muse-spark-1.3-contributor-free": {InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002},
	}
	aliases := []config.Alias{{Name: "muse-spark-1.3-contributor-free", Targets: []config.AliasTarget{
		{Provider: "dxc-openshift", Model: "muse-spark-1.3-contributor-free"},
		{Provider: "gold-openshift", Model: "muse-spark-1.3-contributor-free"},
	}}}
	upstream := []accounting.UpstreamSummary{
		{Provider: "dxc-openshift", Model: "muse-spark-1.3-contributor-free", Operation: "responses", StatusCode: 200, Count: 9},
		{Provider: "gold-openshift", Model: "muse-spark-1.3-contributor-free", Operation: "responses", StatusCode: 200, Count: 3},
	}
	s := accounting.Summary{Model: "alias/muse-spark-1.3-contributor-free", Operation: "responses", StatusCode: 200, Count: 12, PromptTokens: 4000000, CompletionTokens: 2000, TotalTokens: 4002000, CachedTokens: 3000000}
	cost, ok := summaryCostWithAliases(s, prices, aliases, upstream)
	if !ok {
		t.Fatal("alias row must be priced from targets")
	}
	want := (1.0*0.1 + 0.002*0.002 + 0.0) + (3.0*0.1 + 0.0 + 0.0)
	_ = want
	half, _ := prices["dxc-openshift/muse-spark-1.3-contributor-free"].Cost(3000000, 1500, 2250000, 0, 0)
	quarter, _ := prices["gold-openshift/muse-spark-1.3-contributor-free"].Cost(1000000, 500, 750000, 0, 0)
	if cost < half+quarter-1e-9 || cost > half+quarter+1e-9 {
		t.Fatalf("cost = %v, want 3:1 weighted %v", cost, half+quarter)
	}
	if got := summaryCostTextWithAliases(s, prices, aliases, upstream); got == "-" {
		t.Fatal("alias cost text must not be dash")
	}
}

func TestAliasCostFallsBackToEvenSplit(t *testing.T) {
	prices := map[string]*config.ModelPricing{
		"a/m": {InputPerMillion: 2},
		"b/m": {InputPerMillion: 4},
	}
	aliases := []config.Alias{{Name: "x", Targets: []config.AliasTarget{{Provider: "a", Model: "m"}, {Provider: "b", Model: "m"}}}}
	s := accounting.Summary{Model: "alias/x", Operation: "chat", StatusCode: 200, Count: 2, PromptTokens: 2000000, TotalTokens: 2000000}
	cost, ok := summaryCostWithAliases(s, prices, aliases, nil)
	if !ok {
		t.Fatal("alias row must be priced with even split")
	}
	want := 1.0*2 + 1.0*4
	if cost < want-1e-9 || cost > want+1e-9 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
}

func TestAliasCostAttributesByUpstreamTokens(t *testing.T) {
	prices := map[string]*config.ModelPricing{
		"dxc/m":  {InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002},
		"gold/m": {InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002},
	}
	aliases := []config.Alias{{Name: "x", Targets: []config.AliasTarget{
		{Provider: "dxc", Model: "m"},
		{Provider: "gold", Model: "m"},
	}}}
	upstream := []accounting.UpstreamSummary{
		{Provider: "dxc", Model: "m", Operation: "responses", StatusCode: 200, Count: 9,
			PromptTokens: 90000, CompletionTokens: 900, TotalTokens: 90900, CachedTokens: 10000},
		{Provider: "gold", Model: "m", Operation: "responses", StatusCode: 200, Count: 3,
			PromptTokens: 3000000, CompletionTokens: 300, TotalTokens: 3000300, CachedTokens: 2900000},
	}
	s := accounting.Summary{Model: "alias/x", Operation: "responses", StatusCode: 200, Count: 12,
		PromptTokens: 3090000, CompletionTokens: 1200, TotalTokens: 3091200, CachedTokens: 2910000}
	dxcWant, _ := prices["dxc/m"].Cost(90000, 900, 10000, 0, 0)
	goldWant, _ := prices["gold/m"].Cost(3000000, 300, 2900000, 0, 0)
	cost, ok := summaryCostWithAliases(s, prices, aliases, upstream)
	if !ok {
		t.Fatal("alias row must be priced from upstream tokens")
	}
	if cost < dxcWant+goldWant-1e-9 || cost > dxcWant+goldWant+1e-9 {
		t.Fatalf("cost = %v, want upstream-token sum %v", cost, dxcWant+goldWant)
	}
	dxc, ok := providerRowCost("dxc", s, prices, aliases, upstream)
	if !ok {
		t.Fatal("dxc must claim alias share")
	}
	if dxc < dxcWant-1e-9 || dxc > dxcWant+1e-9 {
		t.Fatalf("dxc = %v, want actual upstream cost %v, not count share", dxc, dxcWant)
	}
	gold, ok := providerRowCost("gold", s, prices, aliases, upstream)
	if !ok {
		t.Fatal("gold must claim alias share")
	}
	if gold < goldWant-1e-9 || gold > goldWant+1e-9 {
		t.Fatalf("gold = %v, want actual upstream cost %v, not count share", gold, goldWant)
	}
	if dxc+gold < cost-1e-9 || dxc+gold > cost+1e-9 {
		t.Fatalf("shares %v + %v must sum to alias total %v", dxc, gold, cost)
	}
}

func TestAliasCostUnpricedWithoutTargets(t *testing.T) {
	s := accounting.Summary{Model: "alias/unknown", PromptTokens: 10}
	if _, ok := summaryCostWithAliases(s, map[string]*config.ModelPricing{}, nil, nil); ok {
		t.Fatal("empty prices must not price alias")
	}
	if _, ok := summaryCostWithAliases(s, map[string]*config.ModelPricing{"a/m": {InputPerMillion: 1}}, []config.Alias{{Name: "other"}}, nil); ok {
		t.Fatal("unknown alias must not be priced")
	}
}

func TestProviderCostTextAttributesAliasShare(t *testing.T) {
	prices := map[string]*config.ModelPricing{
		"dxc/m":  {InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002},
		"gold/m": {InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002},
	}
	aliases := []config.Alias{{Name: "x", Targets: []config.AliasTarget{
		{Provider: "dxc", Model: "m"},
		{Provider: "gold", Model: "m"},
	}}}
	upstream := []accounting.UpstreamSummary{
		{Provider: "dxc", Model: "m", Operation: "responses", StatusCode: 200, Count: 9},
		{Provider: "gold", Model: "m", Operation: "responses", StatusCode: 200, Count: 3},
	}
	summaries := []accounting.Summary{
		{Model: "alias/x", Operation: "responses", StatusCode: 200, Count: 12, PromptTokens: 4000000, CompletionTokens: 2000, TotalTokens: 4002000, CachedTokens: 3000000},
	}
	full, ok := summaryCostWithAliases(summaries[0], prices, aliases, upstream)
	if !ok {
		t.Fatal("alias row must be priced")
	}
	dxc, ok := providerRowCost("dxc", summaries[0], prices, aliases, upstream)
	if !ok {
		t.Fatal("dxc must claim alias share")
	}
	gold, ok := providerRowCost("gold", summaries[0], prices, aliases, upstream)
	if !ok {
		t.Fatal("gold must claim alias share")
	}
	if dxc+gold < full-1e-9 || dxc+gold > full+1e-9 {
		t.Fatalf("shares %v + %v must sum to alias total %v", dxc, gold, full)
	}
	if dxc < gold {
		t.Fatalf("dxc served 3:1 but claims less: dxc=%v gold=%v", dxc, gold)
	}
	if got := providerCostTextWithAliases("other", summaries, prices, aliases, upstream); got != "-" {
		t.Fatalf("uninvolved provider = %q, want -", got)
	}
	if got := providerCostTextWithAliases("dxc", summaries, prices, aliases, upstream); got == "-" {
		t.Fatal("dxc provider cost must not be dash")
	}
}

func TestPaneBoxKeepsRowsInsideBorder(t *testing.T) {
	_, inner := paneBox(120, 12, false)
	if inner != 118 {
		t.Fatalf("inner = %d, want 118 (content = Width - borders)", inner)
	}
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{Model: "alias/m", Operation: "responses", StatusCode: 200,
		PromptTokens: 682573, CompletionTokens: 385, TotalTokens: 682958, CachedTokens: 676962})
	price := &config.ModelPricing{InputPerMillion: 0.1, OutputPerMillion: 0.2, CachedPerMillion: 0.002}
	snap := &RuntimeSnapshot{
		Version: "test", Address: ":x", AuthMode: "none", StartTime: time.Now(),
		Providers: []config.Provider{{Name: "dxc", Models: []config.Model{{Name: "m", Pricing: price}}}},
		Aliases:   []config.Alias{{Name: "m", Targets: []config.AliasTarget{{Provider: "dxc", Model: "m"}}}},
		Usage:     usage,
		Health:    &remoteHealth{states: map[string]bool{}},
	}
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, statsHeight: 20, width: 120}
	for _, l := range strings.Split(renderUsage(m, 120, 14), "\n") {
		plain := ansiSeq.ReplaceAllString(l, "")
		if strings.Contains(plain, "alias/") && !strings.Contains(plain, "$") {
			t.Fatalf("usage data row lost COST inside border: %q", plain)
		}
	}
}

func TestProviderCostTextSumsPricedModelsOnly(t *testing.T) {
	prices := map[string]*config.ModelPricing{
		"openai/a": {InputPerMillion: 1, OutputPerMillion: 1},
	}
	summaries := []accounting.Summary{
		{Model: "openai/a", PromptTokens: 1000000, CompletionTokens: 1000000, TotalTokens: 2000000},
		{Model: "openai/b", PromptTokens: 1000000, TotalTokens: 1000000},
		{Model: "_unresolved_model", PromptTokens: 999},
	}
	if got := providerCostText("openai", summaries, prices); got != "$2.00" {
		t.Errorf("provider cost = %q, want $2.00", got)
	}
	if got := providerCostText("zen", summaries, prices); got != "-" {
		t.Errorf("unpriced provider = %q, want -", got)
	}
}
