package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
)

type upstreamModel struct {
	ID          string
	DisplayName string
}

func listUpstreamModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	switch provider.Type {
	case config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible:
		return listOpenAIStyleModels(ctx, provider, false)
	case config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo:
		return listOpenAIStyleModels(ctx, provider, true)
	case config.ProviderTypeAnthropic:
		return listAnthropicModels(ctx, provider)
	case config.ProviderTypeGemini:
		return listGeminiModels(ctx, provider)
	case config.ProviderTypeGitHubCopilot:
		return listGitHubCopilotModels(ctx, provider)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", provider.Type)
	}
}

func upstreamHTTPClient(provider config.Provider) *http.Client {
	timeout := provider.UpstreamHeaderTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func upstreamBaseURL(provider config.Provider) string {
	if provider.BaseURL != "" {
		return provider.BaseURL
	}
	switch provider.Type {
	case config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible:
		return "https://api.openai.com"
	case config.ProviderTypeAnthropic:
		return "https://api.anthropic.com"
	case config.ProviderTypeGemini:
		return "https://generativelanguage.googleapis.com"
	case config.ProviderTypeOpenCodeZen:
		return "https://opencode.ai/zen/v1"
	case config.ProviderTypeOpenCodeGo:
		return "https://opencode.ai/zen/go/v1"
	case config.ProviderTypeGitHubCopilot:
		return copilotlogin.DefaultBaseURL
	default:
		return ""
	}
}

func joinUpstreamPath(baseURL, path string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") && strings.HasPrefix(path, "/v1/") {
		return baseURL + strings.TrimPrefix(path, "/v1")
	}
	return baseURL + path
}

func listOpenAIStyleModels(ctx context.Context, provider config.Provider, opencodeHeaders bool) ([]upstreamModel, error) {
	target := joinUpstreamPath(upstreamBaseURL(provider), "/v1/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	if opencodeHeaders {
		req.Header.Set("User-Agent", opencodeCLIUserAgent(provider))
		if session, err := newCLIopencodeSession(); err == nil {
			req.Header.Set("x-opencode-session", session)
		}
	}
	resp, err := upstreamHTTPClient(provider).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s returned status %d: %s", target, resp.StatusCode, truncateUpstreamBody(body))
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode upstream models: %w", err)
	}
	out := make([]upstreamModel, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID == "" {
			continue
		}
		out = append(out, upstreamModel{ID: m.ID})
	}
	return out, nil
}

func listAnthropicModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	var out []upstreamModel
	afterID := ""
	for {
		target := strings.TrimRight(upstreamBaseURL(provider), "/") + "/v1/models?limit=1000"
		if afterID != "" {
			target += "&after_id=" + afterID
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", provider.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		resp, err := upstreamHTTPClient(provider).Do(req)
		if err != nil {
			return nil, fmt.Errorf("upstream call: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read upstream body: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("upstream %s returned status %d: %s", target, resp.StatusCode, truncateUpstreamBody(body))
		}
		var parsed struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode upstream models: %w", err)
		}
		for _, m := range parsed.Data {
			if m.ID == "" {
				continue
			}
			out = append(out, upstreamModel{ID: m.ID, DisplayName: m.DisplayName})
		}
		if !parsed.HasMore || parsed.LastID == "" {
			return out, nil
		}
		afterID = parsed.LastID
	}
}

func listGeminiModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	var out []upstreamModel
	pageToken := ""
	for {
		target := strings.TrimRight(upstreamBaseURL(provider), "/") + "/v1beta/models?pageSize=100"
		if pageToken != "" {
			target += "&pageToken=" + pageToken
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", provider.APIKey)
		resp, err := upstreamHTTPClient(provider).Do(req)
		if err != nil {
			return nil, fmt.Errorf("upstream call: %w", err)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read upstream body: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("upstream %s returned status %d: %s", target, resp.StatusCode, truncateUpstreamBody(body))
		}
		var parsed struct {
			Models []struct {
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode upstream models: %w", err)
		}
		for _, m := range parsed.Models {
			id := strings.TrimPrefix(m.Name, "models/")
			if id == "" {
				continue
			}
			out = append(out, upstreamModel{ID: id, DisplayName: m.DisplayName})
		}
		if parsed.NextPageToken == "" {
			return out, nil
		}
		pageToken = parsed.NextPageToken
	}
}

func listGitHubCopilotModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	if provider.CopilotToken == "" {
		return nil, fmt.Errorf("github-copilot credential is not configured; run login first")
	}
	base := upstreamBaseURL(provider)
	if base == "" {
		base = copilotlogin.DefaultBaseURL
	}
	target := strings.TrimRight(base, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	copilotlogin.ApplyModelsHeaders(req, provider.CopilotToken, version)
	resp, err := upstreamHTTPClient(provider).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("upstream %s returned status %d: %s (re-run `aiproxy login github-copilot`, then restart or SIGHUP)", target, resp.StatusCode, truncateUpstreamBody(body))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s returned status %d: %s", target, resp.StatusCode, truncateUpstreamBody(body))
	}
	var objectParsed struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &objectParsed); err != nil {
		return nil, fmt.Errorf("decode upstream models: %w", err)
	}
	if len(objectParsed.Data) > 0 {
		out := make([]upstreamModel, 0, len(objectParsed.Data))
		for _, m := range objectParsed.Data {
			if m.ID == "" {
				continue
			}
			out = append(out, upstreamModel{ID: m.ID, DisplayName: m.DisplayName})
		}
		return out, nil
	}
	var arrayParsed []struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(body, &arrayParsed); err != nil {
		return nil, fmt.Errorf("decode upstream models: %w", err)
	}
	out := make([]upstreamModel, 0, len(arrayParsed))
	for _, m := range arrayParsed {
		if m.ID == "" {
			continue
		}
		out = append(out, upstreamModel{ID: m.ID, DisplayName: m.DisplayName})
	}
	return out, nil
}

func opencodeCLIUserAgent(provider config.Provider) string {
	if provider.UserAgent != "" {
		return provider.UserAgent
	}
	return "aiproxy/" + version
}

func newCLIopencodeSession() (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return "ses_" + hex.EncodeToString(entropy[:]), nil
}

func truncateUpstreamBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 500 {
		return s[:500] + "…"
	}
	if s == "" {
		return "empty response"
	}
	return s
}
