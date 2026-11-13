package dbmerge

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/egose/aiproxy/internal/tlsprofile"
)

func tlsMergeCustomProfile(t *testing.T) tlsprofile.Profile {
	t.Helper()
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildProviderTLSClientHelloBuiltins(t *testing.T) {
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		for _, enabled := range []bool{false, true} {
			row := store.DBProvider{Name: "managed", Type: "opencode-zen", TLSClientHello: selector, Enabled: enabled}
			p, err := BuildProvider(row, []store.DBProviderModel{{Name: "m", Protocol: "chat", Capabilities: []string{"chat"}}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			want := selector
			if want == "" {
				want = "native"
			}
			profile, err := tlsprofile.ResolveBuiltin(want)
			if err != nil || p.TLSClientHello != selector || p.HasTLSClientHello != (selector != "") || p.TLSProfile.Identity() != profile.Identity() || p.TLSClientHelloInherited || p.HasTLSClientHelloSpecFile || p.TLSClientHelloSpecFile != "" {
				t.Fatalf("built-in descriptor/metadata mismatch: selector=%s enabled=%t err=%v", selector, enabled, err)
			}
			if err := config.ValidateDynamicProvider(p); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestBuildProviderTLSClientHelloInheritance(t *testing.T) {
	for _, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		bases := testBases()
		base := bases["base"]
		base.TLSClientHello, base.HasTLSClientHello = selector, selector != ""
		base, err := config.ResolveDynamicProviderTLSProfile(base)
		if err != nil {
			t.Fatal(err)
		}
		bases["base"] = base
		p, err := BuildProvider(store.DBProvider{Name: "derived", Type: "openai", Extends: "base"}, nil, bases)
		if err != nil || !p.TLSClientHelloInherited || p.TLSClientHello != selector || p.HasTLSClientHello != base.HasTLSClientHello || p.TLSProfile.Identity() != base.TLSProfile.Identity() {
			t.Fatalf("inherited descriptor/metadata mismatch: selector=%s err=%v", selector, err)
		}
		if !p.TLSProfile.IsNative() {
			spec, err := p.TLSProfile.Materialize()
			if err != nil {
				t.Fatal(err)
			}
			spec.CipherSuites[0] = 0
			other, err := base.TLSProfile.Materialize()
			if err != nil || other.CipherSuites[0] == 0 {
				t.Fatal("derived profile materialization mutated the base")
			}
		}
	}
}

func TestBuildProviderTLSClientHelloRejectsStoredInvalidRows(t *testing.T) {
	for _, selector := range []string{"native", "chrome", "custom", "unknown", " "} {
		_, err := BuildProvider(store.DBProvider{Name: "derived", Type: "openai", Extends: "missing", TLSClientHello: selector}, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "derived provider cannot declare tls_client_hello") {
			t.Fatalf("stored local declaration must fail before cloning: %s %v", selector, err)
		}
	}
	for _, selector := range []string{"custom", "Chrome", "firefox", "bun", "boringssl", "private-source.json"} {
		_, err := BuildProvider(store.DBProvider{Name: "managed", Type: "openai", TLSClientHello: selector}, nil, nil)
		if err == nil || strings.Contains(err.Error(), "private-source.json") {
			t.Fatalf("invalid stored selector accepted or leaked: %v", err)
		}
	}
	bases := testBases()
	base := bases["base"]
	base.TLSClientHello, base.HasTLSClientHello = "custom", true
	base.TLSClientHelloSpecFile, base.HasTLSClientHelloSpecFile = "unread-private-source.json", true
	base.TLSProfile = tlsMergeCustomProfile(t)
	bases["base"] = base
	_, err := BuildProvider(store.DBProvider{Name: "derived", Type: "openai", Extends: "base"}, nil, bases)
	if err == nil || !strings.Contains(err.Error(), "static-config-only") || strings.Contains(err.Error(), "unread-private-source") {
		t.Fatalf("managed inheritance must reject resolved static custom without source I/O: %v", err)
	}
	base.TLSClientHelloSpecFile, base.HasTLSClientHelloSpecFile = "", false
	base.TLSClientHello, base.HasTLSClientHello = "native", true
	bases["base"] = base
	if _, err := BuildProvider(store.DBProvider{Name: "derived", Type: "openai", Extends: "base"}, nil, bases); err == nil {
		t.Fatal("custom effective descriptor hidden behind native metadata was accepted")
	}
}

func TestMergeCatalogTLSClientHelloActivationAndCorruption(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testDBURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	base := testBases()["base"]
	base.Name = fmt.Sprintf("tls-base-%d", time.Now().UnixNano())
	base.TLSClientHello, base.HasTLSClientHello = "chrome", true
	base, err = config.ResolveDynamicProviderTLSProfile(base)
	if err != nil {
		t.Fatal(err)
	}
	rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{base}, nil, nil)}
	rows := []store.DBProvider{
		{Name: fmt.Sprintf("tls-direct-%d", time.Now().UnixNano()), Type: "opencode-zen", TLSClientHello: "safari", Enabled: true},
		{Name: fmt.Sprintf("tls-native-%d", time.Now().UnixNano()), Type: "opencode-zen", Enabled: true},
		{Name: fmt.Sprintf("tls-derived-%d", time.Now().UnixNano()), Type: "openai", Extends: base.Name},
	}
	for i := range rows {
		models := []store.DBProviderModel{{Name: "m", Protocol: "chat", Capabilities: []string{"chat"}}}
		if rows[i].Extends != "" {
			models = nil
		}
		if err := st.CreateProviderAggregate(ctx, &rows[i], models); err != nil {
			t.Fatal(err)
		}
		id := rows[i].ID
		t.Cleanup(func() { _ = st.DeleteProvider(ctx, id) })
	}
	merged, err := MergeCatalog(ctx, st, rt)
	if err != nil {
		t.Fatal(err)
	}
	providers := make(map[string]config.Provider)
	for _, p := range append(merged.Providers, merged.DisabledProviders...) {
		providers[p.Name] = p
	}
	if providers[rows[0].Name].TLSProfile.CanonicalName() != "safari-16.0" || !providers[rows[1].Name].TLSProfile.IsNative() || !providers[rows[2].Name].TLSClientHelloInherited || providers[rows[2].Name].TLSProfile.Identity() != base.TLSProfile.Identity() {
		t.Fatal("merged catalog did not attach effective native/builtin/inherited profiles")
	}
	rows[0].TLSClientHello = "native"
	if err := st.UpdateProvider(ctx, &rows[0]); err != nil {
		t.Fatal(err)
	}
	merged, err = MergeCatalog(ctx, st, rt)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range merged.Providers {
		if p.Name == rows[0].Name && !p.TLSProfile.IsNative() {
			t.Fatal("subsequent merge retained the old profile")
		}
	}
	for _, selector := range []string{"native", "custom", "firefox"} {
		rows[2].TLSClientHello = selector
		if err := st.UpdateProvider(ctx, &rows[2]); err != nil {
			t.Fatal(err)
		}
		if _, err := MergeCatalog(ctx, st, rt); err == nil {
			t.Fatal("corrupt stored derived declaration did not reject merge")
		}
		unchanged, _ := rt.Catalog.Provider(base.Name)
		if unchanged.TLSProfile.Identity() != base.TLSProfile.Identity() {
			t.Fatal("failed merge mutated static runtime profile")
		}
	}
	rows[2].TLSClientHello = ""
	if err := st.UpdateProvider(ctx, &rows[2]); err != nil {
		t.Fatal(err)
	}
	base.TLSClientHello, base.HasTLSClientHello = "custom", true
	base.TLSProfile = tlsMergeCustomProfile(t)
	rt.Catalog = config.NewCatalog([]config.Provider{base}, nil, nil)
	if _, err := MergeCatalog(ctx, st, rt); err == nil || !strings.Contains(err.Error(), "static-config-only") {
		t.Fatalf("stored incompatible static custom inheritance must reject merge: %v", err)
	}
}
