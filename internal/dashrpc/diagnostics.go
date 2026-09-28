package dashrpc

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

type ProviderDiagnostics struct {
	HeaderTimeout string       `json:"header_timeout"`
	Probe         *ProbeConfig `json:"probe"`
}

type ProbeConfig struct {
	Path             string `json:"path"`
	Method           string `json:"method"`
	ExpectedStatus   int    `json:"expected_status"`
	Interval         string `json:"interval"`
	Timeout          string `json:"timeout"`
	FailureThreshold int    `json:"failure_threshold"`
	SuccessThreshold int    `json:"success_threshold"`
}

type ModelDetails struct {
	DisplayName  string              `json:"display_name"`
	UpstreamName string              `json:"upstream_name"`
	Protocol     string              `json:"protocol"`
	Capabilities []config.Capability `json:"capabilities"`
}

type Affinity struct {
	Enabled bool     `json:"enabled"`
	Headers []string `json:"headers"`
}

func ProviderMetadata(p config.Provider) *ProviderDiagnostics {
	d := &ProviderDiagnostics{HeaderTimeout: p.UpstreamHeaderTimeout.String()}
	if h := p.Healthcheck; h != nil {
		d.Probe = &ProbeConfig{Path: DiagnosticURL(h.Path), Method: h.Method,
			ExpectedStatus: h.ExpectedStatus, Interval: h.Interval.String(), Timeout: h.Timeout.String(),
			FailureThreshold: h.FailureThreshold, SuccessThreshold: h.SuccessThreshold}
	}
	return d
}

func DiagnosticURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		return "[redacted URL]"
	}
	if u.Host == "" && !strings.HasPrefix(raw, "/") {
		return "[redacted URL]"
	}
	u.User, u.RawQuery, u.Fragment, u.RawFragment, u.RawPath = nil, "", "", "", ""
	u.ForceQuery = false
	parts := strings.Split(u.Path, "/")
	for i, part := range parts {
		switch part {
		case "", "api", "v1", "v1beta", "v2", "v3", "health", "healthz", "ready", "readyz", "live", "livez", "status", "models", "chat", "completions", "responses", "zen", "go":
		default:
			parts[i] = "[redacted]"
		}
	}
	u.Path = strings.Join(parts, "/")
	return u.String()
}

var diagnosticStatus = regexp.MustCompile(`^unexpected status [0-9]{3} \(want [0-9]{3}\)$`)

func DiagnosticReason(message string) string {
	switch message {
	case "", "ok", "body mismatch", "no base_url for healthcheck", "transport error", "timeout", "DNS lookup failed", "connection refused", "TLS error":
		return message
	}
	if diagnosticStatus.MatchString(message) {
		return message
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "timeout"):
		return "timeout"
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "lookup "):
		return "DNS lookup failed"
	case strings.Contains(lower, "connection refused"):
		return "connection refused"
	case strings.Contains(lower, "tls"), strings.Contains(lower, "x509"):
		return "TLS error"
	default:
		return "transport error"
	}
}
