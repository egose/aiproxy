package upstreamhttp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"

	"github.com/egose/aiproxy/internal/upstreampolicy"
)

const maxDNSAnswers = 64

var ErrPublicAddress = errors.New("tenant upstream DNS must resolve exclusively to public IP addresses")

type restrictedTransport struct {
	Transport
	policy *upstreampolicy.Policy
}

func withOutboundPolicy(transport Transport, policy *upstreampolicy.Policy) Transport {
	if policy == nil {
		return transport
	}
	return &restrictedTransport{Transport: transport, policy: policy}
}

func (t *restrictedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.policy.CheckURL(req.URL); err != nil {
		closeRequestBody(req)
		return nil, err
	}
	if req.Host != "" {
		origin, err := upstreampolicy.Origin(req.URL)
		hostOrigin, hostErr := upstreampolicy.Origin(&url.URL{Scheme: req.URL.Scheme, Host: req.Host})
		if err != nil || hostErr != nil || origin != hostOrigin {
			closeRequestBody(req)
			return nil, upstreampolicy.ErrDestination
		}
	}
	return t.Transport.RoundTrip(req)
}

func publicDialContext(policy *upstreampolicy.Policy, lookup func(context.Context, string, string) ([]netip.Addr, error), dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil, upstreampolicy.ErrDestination
		}
		if err := policy.CheckURL(&url.URL{Scheme: "https", Host: address}); err != nil {
			return nil, err
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, upstreampolicy.ErrDestination
		}
		ctx, cancel := context.WithTimeout(ctx, tcpDialTimeout)
		defer cancel()
		addresses := []netip.Addr{}
		if ip, err := netip.ParseAddr(host); err == nil {
			addresses = append(addresses, ip)
		} else {
			addresses, err = lookup(ctx, "ip", host)
			if err != nil {
				return nil, ErrPublicAddress
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(addresses) == 0 || len(addresses) > maxDNSAnswers {
			return nil, ErrPublicAddress
		}
		for _, ip := range addresses {
			if !upstreampolicy.IsPublicIP(ip) {
				return nil, ErrPublicAddress
			}
		}
		var lastErr error
		seen := make(map[netip.Addr]bool, len(addresses))
		for _, ip := range addresses {
			ip = ip.Unmap()
			if seen[ip] || (network == "tcp4" && !ip.Is4()) || (network == "tcp6" && !ip.Is6()) {
				continue
			}
			seen[ip] = true
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			conn, err := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				if err := ctx.Err(); err != nil {
					_ = conn.Close()
					return nil, err
				}
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = ErrPublicAddress
		}
		return nil, lastErr
	}
}
