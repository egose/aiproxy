package healthcheck

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/upstreamhttp"
	"github.com/egose/aiproxy/internal/upstreampolicy"
	"github.com/google/uuid"
)

func TestTenantProbePolicyPreparedForAbsoluteAndRelativeTargets(t *testing.T) {
	policy, _ := upstreampolicy.New([]string{"https://api.example.com", "https://probe.example.com:8443"})
	for _, selector := range []string{"native", "chrome", "safari"} {
		for _, path := range []string{"/ready", "https://probe.example.com:8443/ready"} {
			p := testProvider("tenant", "https://api.example.com/v1", fastCheck(path))
			p.DatabaseManaged, p.Workspace, p.OutboundPolicy, p.TLSClientHello = true, identity.InWorkspace(uuid.New()), policy, selector
			p.APIKey, p.Healthcheck.SendAuthorization = "own-probe-fixture", true
			factories, calls := 0, 0
			m := New(nil, nil, "fixture")
			defer m.Close()
			m.factory = func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
				factories++
				if options.OutboundPolicy != policy {
					t.Fatal("probe lost the immutable tenant policy snapshot")
				}
				return probeTransportFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if options.OutboundPolicy.CheckURL(req.URL) != nil || req.Header.Get("Authorization") != "Bearer own-probe-fixture" {
						t.Fatal("probe used an unapproved destination or wrong credential")
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ready"))}, nil
				}), nil
			}
			if code, _, err := m.probe(context.Background(), p); err != nil || code != 200 || factories != 1 || calls != 1 {
				t.Fatalf("approved probe %s %s failed: %v", selector, path, err)
			}
			for _, target := range []string{"https://127.0.0.1/ready", "http://api.example.com/ready", "https://probe.example.com/ready", "https://unapproved.example.com/ready"} {
				forbidden := p
				hc := *p.Healthcheck
				hc.Path = target
				forbidden.Healthcheck = &hc
				beforeFactories, beforeCalls := factories, calls
				if _, _, err := m.probe(context.Background(), forbidden); err == nil || factories != beforeFactories || calls != beforeCalls {
					t.Fatal("forbidden absolute probe prepared a transport or made an outbound call")
				}
			}
			changed := p
			changed.OutboundPolicy, _ = upstreampolicy.New([]string{"https://api.example.com", "https://probe.example.com:8443", "https://other.example.com"})
			if fingerprint(changed) == fingerprint(p) {
				t.Fatal("a changed policy would keep a trusted/stale probe transport")
			}
		}
	}
}
