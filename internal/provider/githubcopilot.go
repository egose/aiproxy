package provider

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

func (a *adapter) doGitHubCopilot(ctx context.Context, r Request) (*Result, error) {
	if r.Operation != OpChatCompletions {
		return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
	}
	if r.CopilotToken == "" {
		return nil, ErrInvalidRequest{Message: "github-copilot credential is not configured; run login first"}
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	rewritten, err := rewriteModel(body, r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("rewrite model: %v", err)}
	}
	base := r.BaseURL
	if base == "" {
		base = copilotlogin.DefaultBaseURL
	}
	target := joinBaseURLAndPath(base, copilotlogin.ChatCompletionsPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(rewritten))
	if err != nil {
		return nil, err
	}
	copilotlogin.ApplyHeaders(req, r.CopilotToken, r.Version, body)
	return executeUpstream(r, req, openAIPassthroughHandlers(r, copilotlogin.IsStreamingBody(body)))
}
