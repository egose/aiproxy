package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

type upstreamModel struct {
	ID          string
	DisplayName string
}

const (
	upstreamModelMaxPages      = 100
	upstreamModelMaxEntries    = 10_000
	upstreamModelMaxPageBytes  = 8 << 20
	upstreamModelMaxTotalBytes = 32 << 20
	upstreamModelListTimeout   = 2 * time.Minute
)

type upstreamModelDiscovery struct {
	client  *http.Client
	pages   int
	entries int
	bytes   int64
	cursors map[string]struct{}
}

func listUpstreamModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamModelListTimeout)
	defer cancel()
	discovery := &upstreamModelDiscovery{client: upstreamHTTPClient(provider), cursors: make(map[string]struct{})}
	models, err := discovery.list(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("upstream model discovery incomplete: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("upstream model discovery incomplete: %w", err)
	}
	return models, nil
}

func (d *upstreamModelDiscovery) list(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	switch provider.Type {
	case config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible, config.ProviderTypeZenMux, config.ProviderTypeOpenRouter:
		return d.listOpenAIStyleModels(ctx, provider, false)
	case config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo:
		return d.listOpenAIStyleModels(ctx, provider, true)
	case config.ProviderTypeAnthropic:
		return d.listAnthropicModels(ctx, provider)
	case config.ProviderTypeGemini:
		return d.listGeminiModels(ctx, provider)
	case config.ProviderTypeGitHubCopilot:
		return d.listGitHubCopilotModels(ctx, provider)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", provider.Type)
	}
}

func (d *upstreamModelDiscovery) readPage(req *http.Request, provider config.Provider) ([]byte, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if d.pages >= upstreamModelMaxPages {
		return nil, fmt.Errorf("page limit exceeded (%d pages)", upstreamModelMaxPages)
	}
	remaining := int64(upstreamModelMaxTotalBytes) - d.bytes
	if remaining <= 0 {
		return nil, fmt.Errorf("aggregate byte limit exceeded (%d bytes)", upstreamModelMaxTotalBytes)
	}
	d.pages++
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	defer resp.Body.Close()
	limit := min(int64(upstreamModelMaxPageBytes), remaining)
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	if int64(len(body)) > limit {
		if remaining < upstreamModelMaxPageBytes {
			return nil, fmt.Errorf("aggregate byte limit exceeded (%d bytes)", upstreamModelMaxTotalBytes)
		}
		return nil, fmt.Errorf("page byte limit exceeded (%d bytes)", upstreamModelMaxPageBytes)
	}
	d.bytes += int64(len(body))
	if resp.StatusCode != http.StatusOK {
		hint := ""
		if provider.Type == config.ProviderTypeGitHubCopilot && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			hint = " (re-run `aiproxy login github-copilot`, then restart or SIGHUP)"
		}
		return nil, fmt.Errorf("upstream %s returned status %d: %s%s", req.URL, resp.StatusCode, truncateUpstreamBody(body), hint)
	}
	return body, nil
}

func (d *upstreamModelDiscovery) addEntries(count int) error {
	if count > upstreamModelMaxEntries-d.entries {
		return fmt.Errorf("model entry limit exceeded (%d entries)", upstreamModelMaxEntries)
	}
	d.entries += count
	return nil
}

func (d *upstreamModelDiscovery) continueWith(cursor string) error {
	if _, seen := d.cursors[cursor]; seen {
		return fmt.Errorf("cursor cycle in upstream pagination")
	}
	d.cursors[cursor] = struct{}{}
	return nil
}

func upstreamHTTPClient(provider config.Provider) *http.Client {
	timeout := provider.UpstreamHeaderTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout, CheckRedirect: upstreamhttp.CheckRedirect}
}

func upstreamBaseURL(provider config.Provider) string {
	if provider.BaseURL != "" {
		return provider.BaseURL
	}
	switch provider.Type {
	case config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible:
		return "https://api.openai.com"
	case config.ProviderTypeZenMux:
		return "https://zenmux.ai/api/v1"
	case config.ProviderTypeOpenRouter:
		return "https://openrouter.ai/api/v1"
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

func (d *upstreamModelDiscovery) listOpenAIStyleModels(ctx context.Context, provider config.Provider, opencodeHeaders bool) ([]upstreamModel, error) {
	target := joinUpstreamPath(upstreamBaseURL(provider), "/v1/models")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}
	req.Header.Set("User-Agent", providerCLIUserAgent(provider))
	if opencodeHeaders {
		if session, err := newCLIopencodeSession(); err == nil {
			req.Header.Set("x-opencode-session", session)
		}
	}
	body, err := d.readPage(req, provider)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode upstream models: %w", err)
	}
	if err := d.addEntries(len(parsed.Data)); err != nil {
		return nil, err
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

func (d *upstreamModelDiscovery) listAnthropicModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	var out []upstreamModel
	afterID := ""
	for {
		target := strings.TrimRight(upstreamBaseURL(provider), "/") + "/v1/models?limit=1000"
		if afterID != "" {
			target += "&after_id=" + url.QueryEscape(afterID)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-api-key", provider.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("User-Agent", providerCLIUserAgent(provider))
		body, err := d.readPage(req, provider)
		if err != nil {
			return nil, err
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
		if err := d.addEntries(len(parsed.Data)); err != nil {
			return nil, err
		}
		if parsed.HasMore && (parsed.LastID == "" || len(parsed.Data) == 0 || parsed.Data[len(parsed.Data)-1].ID != parsed.LastID) {
			return nil, fmt.Errorf("inconsistent Anthropic continuation: has_more requires a nonempty page and last_id matching its final entry")
		}
		for _, m := range parsed.Data {
			if m.ID == "" {
				continue
			}
			out = append(out, upstreamModel{ID: m.ID, DisplayName: m.DisplayName})
		}
		if !parsed.HasMore {
			return out, nil
		}
		if err := d.continueWith(parsed.LastID); err != nil {
			return nil, err
		}
		afterID = parsed.LastID
	}
}

func (d *upstreamModelDiscovery) listGeminiModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
	var out []upstreamModel
	pageToken := ""
	for {
		target := strings.TrimRight(upstreamBaseURL(provider), "/") + "/v1beta/models?pageSize=100"
		if pageToken != "" {
			target += "&pageToken=" + url.QueryEscape(pageToken)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", provider.APIKey)
		req.Header.Set("User-Agent", providerCLIUserAgent(provider))
		body, err := d.readPage(req, provider)
		if err != nil {
			return nil, err
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
		if err := d.addEntries(len(parsed.Models)); err != nil {
			return nil, err
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
		if err := d.continueWith(parsed.NextPageToken); err != nil {
			return nil, err
		}
		pageToken = parsed.NextPageToken
	}
}

func (d *upstreamModelDiscovery) listGitHubCopilotModels(ctx context.Context, provider config.Provider) ([]upstreamModel, error) {
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
	if provider.UserAgent != "" {
		req.Header.Set("User-Agent", provider.UserAgent)
	}
	body, err := d.readPage(req, provider)
	if err != nil {
		return nil, err
	}
	type model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	}
	var parsed []model
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("[")) {
		err = json.Unmarshal(body, &parsed)
	} else {
		var object struct {
			Data []model `json:"data"`
		}
		err = json.Unmarshal(body, &object)
		parsed = object.Data
	}
	if err != nil {
		return nil, fmt.Errorf("decode upstream models: %w", err)
	}
	if err := d.addEntries(len(parsed)); err != nil {
		return nil, err
	}
	out := make([]upstreamModel, 0, len(parsed))
	for _, m := range parsed {
		if m.ID == "" {
			continue
		}
		out = append(out, upstreamModel{ID: m.ID, DisplayName: m.DisplayName})
	}
	return out, nil
}

func providerCLIUserAgent(provider config.Provider) string {
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
