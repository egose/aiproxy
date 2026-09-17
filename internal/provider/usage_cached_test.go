package provider

import (
	"encoding/json"
	"testing"
)

func jsonUnmarshalForTest(t *testing.T, data []byte, v any) error {
	t.Helper()
	return json.Unmarshal(data, v)
}

func TestCachedUsageFromOpenAIChatBody(t *testing.T) {
	body := `{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":30}}}`
	u := usageFromBody([]byte(body))
	if u.PromptTokens != 100 || u.CompletionTokens != 20 || u.TotalTokens != 120 || u.CachedTokens != 30 {
		t.Fatalf("usage = %+v, want 100/20/120 cached 30", u)
	}
	if !u.Has() {
		t.Fatal("usage must be present")
	}
}

func TestCachedUsageClampedToPrompt(t *testing.T) {
	body := `{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":99}}}`
	u := usageFromBody([]byte(body))
	if u.CachedTokens != 10 {
		t.Fatalf("cached = %d, want clamped to prompt 10 (usage=%+v)", u.CachedTokens, u)
	}
}

func TestCachedUsageFromResponsesBody(t *testing.T) {
	body := `{"id":"resp_1","object":"response","status":"completed","output":[],"usage":{"input_tokens":50,"output_tokens":10,"total_tokens":60,"input_tokens_details":{"cached_tokens":15}}}`
	u := usageFromBody([]byte(body))
	if u.PromptTokens != 50 || u.CompletionTokens != 10 || u.TotalTokens != 60 || u.CachedTokens != 15 {
		t.Fatalf("usage = %+v, want 50/10/60 cached 15", u)
	}
}

func TestCachedUsageFromGeminiBody(t *testing.T) {
	body := `{"candidates":[],"usageMetadata":{"promptTokenCount":80,"candidatesTokenCount":20,"totalTokenCount":100,"cachedContentTokenCount":25}}`
	u := usageFromBody([]byte(body))
	if u.PromptTokens != 80 || u.CompletionTokens != 20 || u.TotalTokens != 100 || u.CachedTokens != 25 {
		t.Fatalf("usage = %+v, want 80/20/100 cached 25", u)
	}
}

func TestCachedUsageFromAnthropicBody(t *testing.T) {
	body := `{"id":"msg_1","role":"assistant","content":[],"stop_reason":"end_turn","usage":{"input_tokens":70,"output_tokens":20,"cache_creation_input_tokens":10,"cache_read_input_tokens":20}}`
	u := usageFromAnthropicBody([]byte(body))
	if u.PromptTokens != 100 || u.CompletionTokens != 20 || u.TotalTokens != 120 || u.CachedTokens != 30 {
		t.Fatalf("usage = %+v, want prompt 100 (70+30) /20/120 cached 30", u)
	}
	if u.CacheCreationTokens != 10 || u.CacheReadTokens != 20 {
		t.Fatalf("usage = %+v, want creation 10 read 20", u)
	}
}

func TestTranslatedAnthropicChatWireCarriesCachedDetails(t *testing.T) {
	body := `{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":70,"output_tokens":20,"cache_creation_input_tokens":10,"cache_read_input_tokens":20}}`
	wire, _, err := translateAnthropicResponse([]byte(body), "alias/x")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	var out struct {
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := jsonUnmarshalForTest(t, wire, &out); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	if out.Usage.PromptTokens != 100 || out.Usage.PromptTokensDetails.CachedTokens != 30 {
		t.Fatalf("wire usage = %+v, want prompt 100 cached 30", out.Usage)
	}
}

func TestTranslatedAnthropicResponsesWireCarriesCachedDetails(t *testing.T) {
	body := `{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":70,"output_tokens":20,"cache_creation_input_tokens":10,"cache_read_input_tokens":20}}`
	wire, err := translateAnthropicResponsesResponse([]byte(body), "alias/x")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	var out struct {
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			TotalTokens        int `json:"total_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := jsonUnmarshalForTest(t, wire, &out); err != nil {
		t.Fatalf("unmarshal wire: %v", err)
	}
	if out.Usage.InputTokens != 100 || out.Usage.InputTokensDetails.CachedTokens != 30 {
		t.Fatalf("wire usage = %+v, want input 100 cached 30", out.Usage)
	}
}

func TestCachedUsageMergesOnStream(t *testing.T) {
	s := NewStreamCompletion()
	s.SetUsage(Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30})
	s.Complete(nil, false)
	got := s.Wait().Usage
	if got.CachedTokens != 30 || got.PromptTokens != 100 || got.CompletionTokens != 20 || got.TotalTokens != 120 {
		t.Fatalf("stream usage = %+v", got)
	}
}
