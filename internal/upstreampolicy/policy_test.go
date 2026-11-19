package upstreampolicy

import (
	"net/netip"
	"strings"
	"testing"
)

func TestExactPublicHTTPSOrigins(t *testing.T) {
	p, err := New([]string{"https://API.example.com", "https://probe.example.com:8443", "https://8.8.8.8"})
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://api.example.com/v1", "https://api.example.com:443/v1/models?after=next", "https://probe.example.com:8443/ready", "https://8.8.8.8/"} {
		if err := p.Check(raw); err != nil {
			t.Fatalf("approved URL %s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"http://api.example.com", "https://api.example.com:8443", "https://sub.api.example.com",
		"https://api.example.com.attacker.com", "https://api.example.com.", "https://api.example.com@attacker.com",
		"https://user@api.example.com", "https://api.example.com/#fragment", "https://localhost",
		"https://127.0.0.1", "https://[::1]", "https://metadata.google.internal", "https://2130706433",
		"https://127.1", "https://[fe80::1%25eth0]", "https://api.example.com:", "https://api.example.com:0",
		"https://api.example.com:65536", "https://*.example.com",
	} {
		if err := p.Check(raw); err == nil {
			t.Fatalf("unapproved/unsafe URL accepted: %s", raw)
		}
	}
	for _, raw := range []string{
		"https://api.example.com/", "https://api.example.com?x=1", "https://api.example.com?",
		"https://api.example.com#fragment", "https://api.example.com#", "https://user@api.example.com", "http://api.example.com",
		"https://10.0.0.1", "https://[2001:db8::1]", "https://api.local", "https://home.arpa", "https://server",
		" https://api.example.com", "https://api.example.com ", "https://*.example.com",
	} {
		if _, err := New([]string{raw}); err == nil {
			t.Fatalf("invalid approval accepted: %s", raw)
		}
	}
	if _, err := New([]string{"https://api.example.com", "https://API.example.com:443"}); err == nil {
		t.Fatal("equivalent duplicate approvals accepted")
	}
	if _, err := New(strings.Fields(strings.Repeat("https://api.example.com ", MaxOrigins+1))); err == nil {
		t.Fatal("unbounded approval list accepted")
	}
	empty, _ := New(nil)
	var zero Policy
	var trusted *Policy
	if empty.Check("https://api.example.com") == nil || zero.Check("https://api.example.com") == nil ||
		empty.Identity() != zero.Identity() || empty.Identity() == trusted.Identity() {
		t.Fatal("empty policy did not deny or aliased trusted transport identity")
	}
	if trusted.Check("http://trusted-internal/%invalid") != nil {
		t.Fatal("nil policy changed trusted URL handling")
	}
	reordered, _ := New([]string{"https://8.8.8.8", "https://probe.example.com:8443", "https://api.example.com:443"})
	if p.Identity() != reordered.Identity() {
		t.Fatal("canonical approval order changed policy identity")
	}
}

func TestPublicIPClassification(t *testing.T) {
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2001:4860:4860::8888", "2606:4700:4700::1111", "::ffff:8.8.8.8"} {
		if !IsPublicIP(netip.MustParseAddr(raw)) {
			t.Fatalf("public IP rejected: %s", raw)
		}
	}
	for _, raw := range []string{
		"0.0.0.0", "0.1.2.3", "10.0.0.1", "100.64.0.1", "100.100.100.200", "127.0.0.1",
		"169.254.169.254", "169.254.170.2", "172.16.0.1", "172.31.255.255", "192.0.0.9",
		"192.0.2.1", "192.88.99.1", "192.168.0.1", "198.18.0.1", "198.19.255.255", "198.51.100.1",
		"203.0.113.1", "224.0.0.1", "239.255.255.250", "240.0.0.1", "255.255.255.255", "168.63.129.16",
		"::", "::1", "::ffff:127.0.0.1", "::ffff:169.254.169.254", "64:ff9b::a00:1", "64:ff9b:1::1",
		"100::1", "2001::1", "2001:2::1", "2001:10::1", "2001:20::1", "2001:db8::1",
		"2002:7f00:1::1", "3fff::1", "fc00::1", "fd00:ec2::254", "fe80::1", "fe80::1%eth0", "ff02::1", "4000::1",
	} {
		if IsPublicIP(netip.MustParseAddr(raw)) {
			t.Fatalf("non-public/metadata IP accepted: %s", raw)
		}
	}
	if IsPublicIP(netip.Addr{}) {
		t.Fatal("invalid IP accepted")
	}
}
