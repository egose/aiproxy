package dashboard

import (
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
)

func TestTokensTextSplitsPromptCompletionCached(t *testing.T) {
	if got := tokensText(accounting.Summary{}); got != "~" {
		t.Errorf("zero summary = %q, want ~", got)
	}
	if got := tokensText(accounting.Summary{TotalTokens: 42}); got != "42" {
		t.Errorf("legacy total-only = %q, want 42", got)
	}
	if got := tokensText(accounting.Summary{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}); got != "100/20" {
		t.Errorf("split = %q, want 100/20", got)
	}
	if got := tokensText(accounting.Summary{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30}); got != "100/20 (30c)" {
		t.Errorf("split+cached = %q, want 100/20 (30c)", got)
	}
	if got := tokensText(accounting.Summary{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30, CacheCreationTokens: 10, CacheReadTokens: 20}); got != "100/20 (10w+20r)" {
		t.Errorf("split+creation/read = %q, want 100/20 (10w+20r)", got)
	}
	if got := tokensText(accounting.Summary{PromptTokens: 5, CompletionTokens: 4, TotalTokens: 20}); !strings.Contains(got, "20") || !strings.Contains(got, "5/4") {
		t.Errorf("authoritative total must stay visible, got %q", got)
	}
}

func TestProviderTokensTextMirrorsSplit(t *testing.T) {
	if got := providerTokensText(accounting.ProviderSummary{}); got != "~" {
		t.Errorf("zero provider = %q, want ~", got)
	}
	if got := providerTokensText(accounting.ProviderSummary{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30}); got != "100/20 (30c)" {
		t.Errorf("provider split+cached = %q", got)
	}
	if got := providerTokensText(accounting.ProviderSummary{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30, CacheCreationTokens: 10, CacheReadTokens: 20}); got != "100/20 (10w+20r)" {
		t.Errorf("provider split+creation/read = %q", got)
	}
}
