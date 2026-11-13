package upstreamhttp

import (
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

func TestProfileForProviderUsesCachedCustomAndCanonicalBuiltins(t *testing.T) {
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		provider := config.Provider{Name: "test", TLSClientHello: selector}
		profile, err := ProfileForProvider(provider)
		wantSelector := selector
		if wantSelector == "" {
			wantSelector = "native"
		}
		want, _ := tlsprofile.ResolveBuiltin(wantSelector)
		if err != nil || profile.Identity() != want.Identity() {
			t.Fatalf("selector %q: %s %v", selector, profile.CanonicalName(), err)
		}
	}
	custom := transportTestProfile(t, "custom-h1")
	provider := config.Provider{Name: "test", TLSClientHello: "custom", TLSClientHelloSpecFile: "unavailable-private-source.json", TLSProfile: custom, TLSClientHelloInherited: true}
	profile, err := ProfileForProvider(provider)
	if err != nil || profile.Identity() != custom.Identity() {
		t.Fatal("cached static custom profile was reopened or rejected")
	}
}

func TestProfileForProviderRejectsUnresolvedAndMismatchedMetadata(t *testing.T) {
	custom := transportTestProfile(t, "custom-h1")
	chrome, _ := tlsprofile.ResolveBuiltin("chrome")
	for _, provider := range []config.Provider{
		{TLSClientHello: "unsupported"},
		{TLSClientHello: "", HasTLSClientHello: true},
		{TLSClientHello: "native", HasTLSClientHelloSpecFile: true},
		{TLSClientHello: "custom", TLSClientHelloSpecFile: "private-source"},
		{TLSClientHello: "native", TLSClientHelloSpecFile: "private-source", TLSProfile: custom},
		{TLSClientHello: "custom", TLSProfile: custom},
		{TLSClientHello: "safari", TLSProfile: chrome},
		{TLSProfile: chrome},
	} {
		provider.Name = "test"
		if _, err := ProfileForProvider(provider); err == nil || strings.Contains(err.Error(), "private-source") {
			t.Fatalf("invalid profile fell back or echoed source: %v", err)
		}
	}
}
