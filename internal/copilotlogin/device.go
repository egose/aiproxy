package copilotlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DeviceCodeURL     = "https://github.com/login/device/code"
	TokenURL          = "https://github.com/login/oauth/access_token"
	VerificationURL   = "https://github.com/login/device"
	DeviceGrantType   = "urn:ietf:params:oauth:grant-type:device_code"
	DefaultScope      = "read:user"
	DefaultInterval   = 5 * time.Second
	SlowDownIncrement = 5 * time.Second
	DefaultExpiry     = 900 * time.Second
	HTTPTimeout       = 15 * time.Second
	MaxResponseBytes  = 256 * 1024
)

var (
	ErrAccessDenied = errors.New("authorization denied by user")
	ErrExpired      = errors.New("device codes expired; request new codes and try again")
	ErrRedirect     = errors.New("refused redirect from authorization server")
)

type DeviceCode struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

type TokenResult struct {
	AccessToken string
	TokenType   string
	Scope       string
}

type Client struct {
	HTTP          *http.Client
	DeviceCodeURL string
	TokenURL      string
	Now           func() time.Time
	Sleep         func(ctx context.Context, d time.Duration) error
}

func New() *Client {
	return &Client{
		HTTP:          defaultHTTPClient(),
		DeviceCodeURL: DeviceCodeURL,
		TokenURL:      TokenURL,
		Now:           time.Now,
		Sleep:         sleepWithContext,
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: HTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTPClient()
}

func (c *Client) deviceCodeURL() string {
	if c.DeviceCodeURL != "" {
		return c.DeviceCodeURL
	}
	return DeviceCodeURL
}

func (c *Client) tokenURL() string {
	if c.TokenURL != "" {
		return c.TokenURL
	}
	return TokenURL
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	return sleepWithContext(ctx, d)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func validateClientID(clientID string) error {
	trimmed := strings.TrimSpace(clientID)
	if trimmed == "" {
		return errors.New("client ID is required")
	}
	if trimmed != clientID || strings.ContainsAny(clientID, " \t\r\n") {
		return errors.New("invalid client ID: must not contain whitespace")
	}
	if len(clientID) > 256 {
		return errors.New("invalid client ID: too long")
	}
	return nil
}

func (c *Client) RequestCode(ctx context.Context, clientID, scope string) (DeviceCode, error) {
	if err := validateClientID(clientID); err != nil {
		return DeviceCode{}, err
	}
	if strings.TrimSpace(scope) == "" {
		scope = DefaultScope
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("scope", scope)
	body, err := postForm(ctx, c.httpClient(), c.deviceCodeURL(), form)
	if err != nil {
		return DeviceCode{}, err
	}
	var parsed struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       *int   `json:"expires_in"`
		Interval        *int   `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return DeviceCode{}, errors.New("malformed device-code response")
	}
	if parsed.Error != "" {
		return DeviceCode{}, fmt.Errorf("device-code request failed: %s", redactErrorCode(parsed.Error))
	}
	if parsed.DeviceCode == "" || parsed.UserCode == "" || parsed.VerificationURI == "" {
		return DeviceCode{}, errors.New("malformed device-code response: missing required field")
	}
	expiresIn := DefaultExpiry
	if parsed.ExpiresIn != nil {
		if *parsed.ExpiresIn <= 0 {
			return DeviceCode{}, errors.New("malformed device-code response: invalid expires_in")
		}
		expiresIn = time.Duration(*parsed.ExpiresIn) * time.Second
	}
	interval := DefaultInterval
	if parsed.Interval != nil {
		if *parsed.Interval < 0 {
			return DeviceCode{}, errors.New("malformed device-code response: invalid interval")
		}
		if *parsed.Interval > 0 {
			interval = time.Duration(*parsed.Interval) * time.Second
		}
	}
	return DeviceCode{
		DeviceCode:      parsed.DeviceCode,
		UserCode:        parsed.UserCode,
		VerificationURI: parsed.VerificationURI,
		ExpiresIn:       expiresIn,
		Interval:        interval,
	}, nil
}

func (c *Client) Poll(ctx context.Context, clientID, deviceCode string, interval time.Duration, deadline time.Time) (TokenResult, error) {
	if err := validateClientID(clientID); err != nil {
		return TokenResult{}, err
	}
	if strings.TrimSpace(deviceCode) == "" {
		return TokenResult{}, errors.New("device code is required")
	}
	if interval <= 0 {
		interval = DefaultInterval
	}
	for {
		if err := ctx.Err(); err != nil {
			return TokenResult{}, err
		}
		if !deadline.IsZero() && !c.now().Before(deadline) {
			return TokenResult{}, ErrExpired
		}
		if err := c.sleep(ctx, interval); err != nil {
			return TokenResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return TokenResult{}, err
		}
		if !deadline.IsZero() && !c.now().Before(deadline) {
			return TokenResult{}, ErrExpired
		}
		result, action, nextInterval, err := c.pollOnce(ctx, clientID, deviceCode, interval)
		if err != nil {
			return TokenResult{}, err
		}
		switch action {
		case pollDone:
			return result, nil
		case pollContinue:
			interval = nextInterval
			continue
		}
	}
}

type pollAction int

const (
	pollDone pollAction = iota
	pollContinue
)

func (c *Client) pollOnce(ctx context.Context, clientID, deviceCode string, interval time.Duration) (TokenResult, pollAction, time.Duration, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", DeviceGrantType)
	body, err := postForm(ctx, c.httpClient(), c.tokenURL(), form)
	if err != nil {
		return TokenResult{}, pollDone, interval, err
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
		Interval    *int   `json:"interval"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return TokenResult{}, pollDone, interval, errors.New("malformed token response")
	}
	if parsed.AccessToken != "" {
		return TokenResult{AccessToken: parsed.AccessToken, TokenType: parsed.TokenType, Scope: parsed.Scope}, pollDone, interval, nil
	}
	code := strings.TrimSpace(parsed.Error)
	if code == "" {
		return TokenResult{}, pollDone, interval, errors.New("malformed token response: missing access_token")
	}
	switch code {
	case "authorization_pending":
		return TokenResult{}, pollContinue, interval, nil
	case "slow_down":
		next := interval + SlowDownIncrement
		if parsed.Interval != nil && *parsed.Interval > 0 {
			candidate := time.Duration(*parsed.Interval) * time.Second
			if candidate > next {
				next = candidate
			}
		}
		return TokenResult{}, pollContinue, next, nil
	case "access_denied":
		return TokenResult{}, pollDone, interval, ErrAccessDenied
	case "expired_token", "token_expired":
		return TokenResult{}, pollDone, interval, ErrExpired
	default:
		return TokenResult{}, pollDone, interval, fmt.Errorf("authorization failed: %s", redactErrorCode(code))
	}
}

func postForm(ctx context.Context, client *http.Client, target string, form url.Values) ([]byte, error) {
	if err := checkOAuthTarget(target); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		if isRedirectRefusal(err) {
			return nil, ErrRedirect
		}
		var urlErr *url.Error
		if errors.As(err, &urlErr) && urlErr.Err == http.ErrUseLastResponse {
			return nil, ErrRedirect
		}
		return nil, fmt.Errorf("authorization request: %w", redactTransportError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, ErrRedirect
	}
	limited := io.LimitReader(resp.Body, MaxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read authorization response: %w", redactTransportError(err))
	}
	if len(body) > MaxResponseBytes {
		return nil, errors.New("authorization response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, desc := sniffOAuthError(body)
		if code != "" {
			return append([]byte(nil), body...), nil
		}
		_ = desc
		return nil, fmt.Errorf("authorization request failed with status %d", resp.StatusCode)
	}
	return body, nil
}

func sniffOAuthError(body []byte) (string, string) {
	var parsed struct {
		Error     string `json:"error"`
		ErrorDesc string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", ""
	}
	return strings.TrimSpace(parsed.Error), parsed.ErrorDesc
}

func checkOAuthTarget(target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return errors.New("invalid authorization endpoint")
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && isLoopbackHost(u.Hostname()) {
		return nil
	}
	return errors.New("invalid authorization endpoint")
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if host == "127.0.0.1" || host == "::1" {
		return true
	}
	if strings.HasPrefix(host, "127.") {
		return true
	}
	return false
}

func isRedirectRefusal(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, http.ErrUseLastResponse) {
		return true
	}
	msg := err.Error()
	return bytes.Contains([]byte(msg), []byte("redirect")) && bytes.Contains([]byte(msg), []byte("refus"))
}

func redactErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return "unknown"
	}
	if len(code) > 64 {
		return code[:64]
	}
	return code
}

func redactTransportError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	for _, secret := range []string{"gho_", "ghu_", "github_pat_", "ghp_", "access_token", "device_code", "refresh_token"} { // pragma: allowlist secret
		if strings.Contains(lower, secret) {
			return errors.New("transport error")
		}
	}
	if len(msg) > 200 {
		return errors.New(msg[:200])
	}
	return err
}
