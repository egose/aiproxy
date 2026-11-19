package upstreampolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const MaxOrigins = 128

var ErrDestination = errors.New("tenant upstream destination requires an exact operator-approved public HTTPS origin")

type Policy struct {
	origins  map[string]struct{}
	identity string
}

func New(origins []string) (*Policy, error) {
	if len(origins) > MaxOrigins {
		return nil, fmt.Errorf("allowed_upstream_origins supports at most %d origins", MaxOrigins)
	}
	p := &Policy{origins: make(map[string]struct{}, len(origins))}
	canonical := make([]string, 0, len(origins))
	for _, raw := range origins {
		u, err := url.Parse(raw)
		if err != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || strings.TrimSpace(raw) != raw {
			return nil, fmt.Errorf("allowed_upstream_origins entries must be HTTPS origins without paths, queries, fragments or userinfo")
		}
		origin, err := Origin(u)
		if err != nil {
			return nil, fmt.Errorf("allowed_upstream_origins: %w", err)
		}
		if _, exists := p.origins[origin]; exists {
			return nil, errors.New("allowed_upstream_origins contains a duplicate origin")
		}
		p.origins[origin] = struct{}{}
		canonical = append(canonical, origin)
	}
	if len(canonical) > 0 {
		sort.Strings(canonical)
		sum := sha256.Sum256([]byte(strings.Join(canonical, "\x00")))
		p.identity = hex.EncodeToString(sum[:])
	}
	return p, nil
}

func (p *Policy) Identity() string {
	if p == nil {
		return "trusted"
	}
	if p.identity == "" {
		return "public-https-v1:deny"
	}
	return "public-https-v1:" + p.identity
}

func (p *Policy) CheckURL(u *url.URL) error {
	if p == nil {
		return nil
	}
	origin, err := Origin(u)
	if err != nil {
		return ErrDestination
	}
	if _, approved := p.origins[origin]; !approved {
		return ErrDestination
	}
	return nil
}

func (p *Policy) Check(raw string) error {
	if p == nil {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ErrDestination
	}
	return p.CheckURL(u)
}

func Origin(u *url.URL) (string, error) {
	if u == nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return "", ErrDestination
	}
	authority, err := url.Parse("https://" + u.Host)
	if err != nil || authority.Host != u.Host || authority.User != nil || strings.HasSuffix(u.Host, ":") {
		return "", ErrDestination
	}
	host := strings.ToLower(u.Hostname())
	if ip, err := netip.ParseAddr(host); err == nil {
		if !IsPublicIP(ip) || (ip.Is6() && !strings.HasPrefix(u.Host, "[")) {
			return "", ErrDestination
		}
		host = ip.Unmap().String()
	} else if !publicHostname(host) {
		return "", ErrDestination
	}
	port := 443
	if raw := u.Port(); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1 || port > 65535 {
			return "", ErrDestination
		}
	}
	return "https://" + net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func publicHostname(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".home.arpa", ".onion"} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return false
		}
	}
	numeric := true
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if c < '0' || c > '9' {
				numeric = false
			}
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return !numeric
}

var nonPublicIPv4 = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

var nonPublicIPv6 = []netip.Prefix{
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

func IsPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	if ip.Is4() {
		if ip == netip.MustParseAddr("168.63.129.16") {
			return false
		}
		for _, prefix := range nonPublicIPv4 {
			if prefix.Contains(ip) {
				return false
			}
		}
		return true
	}
	if !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range nonPublicIPv6 {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}
