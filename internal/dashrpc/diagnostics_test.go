package dashrpc

import (
	"strings"
	"testing"
)

func TestDiagnosticURLsFailClosedAndRemainIdempotent(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"https://user:password@example.test:8443/v1/models?key=secret#secret", "https://example.test:8443/v1/models"},
		{"/health?key=secret#secret", "/health"},
		{"https://example.test/v1/path-secret/models", "https://example.test/v1/%5Bredacted%5D/models"},
		{"https://example.test/v1/%73ecret", "https://example.test/v1/%5Bredacted%5D"},
		{"http://[::1]:8080/readyz", "http://[::1]:8080/readyz"},
		{"https://%secret", "[redacted URL]"},
		{"mailto:secret", "[redacted URL]"},
		{"secret", "[redacted URL]"},
		{"", ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got := DiagnosticURL(tc.raw)
			if got != tc.want || DiagnosticURL(got) != got {
				t.Fatalf("URL %q -> %q -> %q, want %q", tc.raw, got, DiagnosticURL(got), tc.want)
			}
		})
	}
}

func TestDiagnosticReasonsNeverReturnUntrustedErrorText(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"Get https://u:secret@host/secret?key=secret: context deadline exceeded", "timeout"},
		{"lookup secret: no such host", "DNS lookup failed"},
		{"dial secret: connection refused", "connection refused"},
		{"tls: secret x509 certificate", "TLS error"},
		{"upstream error containing secret", "transport error"},
		{"unexpected status 503 (want 200)", "unexpected status 503 (want 200)"},
		{"unexpected status 503 (want 200) secret", "transport error"},
		{"body mismatch", "body mismatch"},
		{"ok", "ok"},
	} {
		got := DiagnosticReason(tc.raw)
		if got != tc.want || strings.Contains(got, "secret") || DiagnosticReason(got) != got {
			t.Fatalf("reason %q -> %q, want %q", tc.raw, got, tc.want)
		}
	}
}
