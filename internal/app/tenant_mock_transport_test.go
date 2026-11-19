package app

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/egose/aiproxy/internal/upstreamhttp"
	"github.com/egose/aiproxy/internal/upstreampolicy"
)

type tenantMockTransport struct {
	upstreamhttp.Transport
	policy  *upstreampolicy.Policy
	targets map[string]string
}

func (t *tenantMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.policy.CheckURL(req.URL); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	target, ok := t.targets[req.URL.Hostname()]
	if !ok {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, errors.New("synthetic tenant origin has no mock upstream")
	}
	base, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme, clone.URL.Host, clone.Host = base.Scheme, base.Host, base.Host
	return t.Transport.RoundTrip(clone)
}

func tenantMockTransportFactory(targets map[string]string) func(upstreamhttp.Options) (upstreamhttp.Transport, error) {
	return func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		policy := options.OutboundPolicy
		if policy == nil {
			return upstreamhttp.NewTransport(options)
		}
		options.OutboundPolicy = nil
		transport, err := upstreamhttp.NewTransport(options)
		if err != nil {
			return nil, err
		}
		return &tenantMockTransport{Transport: transport, policy: policy, targets: targets}, nil
	}
}
