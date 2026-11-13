package tlsprofile

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	utls "github.com/refraction-networking/utls"
	"github.com/refraction-networking/utls/dicttls"
)

func fixtureData(t testing.TB) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fixtureRecipe(t testing.TB) map[string]any {
	t.Helper()
	var recipe map[string]any
	if err := json.Unmarshal(fixtureData(t), &recipe); err != nil {
		t.Fatal(err)
	}
	return recipe
}

func recipeData(t testing.TB, recipe map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func recipeExtension(t testing.TB, recipe map[string]any, name string) map[string]any {
	t.Helper()
	for _, value := range recipe["extensions"].([]any) {
		extension := value.(map[string]any)
		if extension["name"] == name {
			return extension
		}
	}
	t.Fatalf("missing test extension %s", name)
	return nil
}

func removeExtensions(recipe map[string]any, names ...string) {
	var remaining []any
	for _, value := range recipe["extensions"].([]any) {
		if !slices.Contains(names, value.(map[string]any)["name"].(string)) {
			remaining = append(remaining, value)
		}
	}
	recipe["extensions"] = remaining
}

func materialize(t testing.TB, profile Profile) *utls.ClientHelloSpec {
	t.Helper()
	spec, err := profile.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func buildHello(t testing.TB, spec *utls.ClientHelloSpec, serverName string) *utls.UConn {
	t.Helper()
	connection := utls.UClient(nil, &utls.Config{ServerName: serverName}, utls.HelloCustom)
	if err := connection.ApplyPreset(spec); err != nil {
		t.Fatal(err)
	}
	if err := connection.BuildHandshakeState(); err != nil {
		t.Fatal(err)
	}
	return connection
}

func TestBuiltinProfiles(t *testing.T) {
	if Native() != (Profile{}) || !Native().IsNative() || Native().CanonicalName() != "native" {
		t.Fatal("native and the zero profile must represent host Go TLS")
	}
	if spec, err := Native().Materialize(); err == nil || spec != nil {
		t.Fatal("native must not materialize a fork Go fingerprint")
	}
	if DependencyVersion != "v1.8.2" || SchemaVersion != "1" || PolicyVersion != "1" {
		t.Fatal("unexpected profile dependency/schema/policy version")
	}
	cases := []struct {
		selector string
		name     string
		id       utls.ClientHelloID
	}{
		{"native", "native", utls.ClientHelloID{}},
		{"chrome", "chrome-133", utls.HelloChrome_133},
		{"chrome-133", "chrome-133", utls.HelloChrome_133},
		{"safari", "safari-16.0", utls.HelloSafari_16_0},
		{"safari-16.0", "safari-16.0", utls.HelloSafari_16_0},
	}
	for _, tc := range cases {
		t.Run(tc.selector, func(t *testing.T) {
			profile, err := ResolveBuiltin(tc.selector)
			if err != nil || profile.CanonicalName() != tc.name || !strings.HasPrefix(profile.Identity(), tc.name+":") {
				t.Fatalf("builtin = %s, %v", profile.CanonicalName(), err)
			}
			canonical, err := ResolveBuiltin(tc.name)
			if err != nil || profile.Identity() != canonical.Identity() {
				t.Fatal("aliases must share the explicit preset identity")
			}
			if profile.IsNative() {
				return
			}
			spec := materialize(t, profile)
			want, err := utls.UTLSIdToSpec(tc.id)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(spec.CipherSuites, want.CipherSuites) || !slices.Equal(spec.CompressionMethods, want.CompressionMethods) || spec.TLSVersMin != want.TLSVersMin || spec.TLSVersMax != want.TLSVersMax {
				t.Fatal("bundled profile advertisements changed")
			}
			if len(spec.Extensions) != len(want.Extensions) {
				t.Fatal("bundled profile extensions changed")
			}
			if tc.name == "safari-16.0" && spec.TLSVersMin != utls.VersionTLS10 {
				t.Fatal("the transport, not the preset descriptor, must clamp Safari negotiation")
			}
			spec.CipherSuites[0] = 0
			for _, extension := range spec.Extensions {
				if sni, ok := extension.(*utls.SNIExtension); ok {
					sni.ServerName = "first.invalid"
				}
			}
			fresh := materialize(t, profile)
			if fresh.CipherSuites[0] == 0 {
				t.Fatal("builtins share mutable cipher storage")
			}
			for _, extension := range fresh.Extensions {
				if sni, ok := extension.(*utls.SNIExtension); ok && sni.ServerName != "" {
					t.Fatal("builtins share mutable SNI")
				}
			}
			if len(buildHello(t, fresh, "clienthello.invalid").HandshakeState.Hello.Raw) > MaxClientHelloBytes {
				t.Fatal("builtin ClientHello exceeds the configured bound")
			}
		})
	}
	selectors := BuiltinSelectors()
	if !slices.Equal(selectors, []string{"native", "chrome", "chrome-133", "safari", "safari-16.0"}) {
		t.Fatalf("selectors = %v", selectors)
	}
	selectors[0] = "changed"
	if BuiltinSelectors()[0] != "native" {
		t.Fatal("selector list is mutable shared state")
	}
	for _, selection := range []string{"", "Native", "Chrome", " chrome", "chrome-155", "chrome-120", "firefox", "firefox-120", "firefox-148", "safari-26.3", "android-11-okhttp", "edge", "ios", "randomized-alpn", "randomized-no-alpn", "bun", "boringssl", "custom", "private-selector-value"} {
		if _, err := ResolveBuiltin(selection); err == nil || strings.Contains(err.Error(), "private-selector-value") {
			t.Errorf("selector %q must fail without arbitrary data: %v", selection, err)
		}
	}
}

func TestCustomProfileFixtureAndPrivateMLKEM(t *testing.T) {
	groupsBefore := maps.Clone(dicttls.DictSupportedGroupsNameIndexed)
	groupValuesBefore := maps.Clone(dicttls.DictSupportedGroupsValueIndexed)
	ciphersBefore := maps.Clone(dicttls.DictCipherSuiteNameIndexed)
	extensionsBefore := maps.Clone(dicttls.DictExtTypeNameIndexed)
	if _, exists := groupsBefore["X25519MLKEM768"]; exists {
		t.Fatal("pinned dictionary changed; reassess the private adapter")
	}
	data := fixtureData(t)
	profile, err := ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	if profile.IsNative() || !profile.IsCustom() || profile.CanonicalName() != "custom" {
		t.Fatal("fixture must resolve to custom")
	}
	spec := materialize(t, profile)
	if len(spec.CipherSuites) != 17 || len(spec.Extensions) != 13 || spec.TLSVersMin != utls.VersionTLS12 || spec.TLSVersMax != utls.VersionTLS13 {
		t.Fatalf("unexpected fixture structure: %d suites, %d extensions, %d..%d", len(spec.CipherSuites), len(spec.Extensions), spec.TLSVersMin, spec.TLSVersMax)
	}
	if !slices.Contains(spec.CipherSuites, utls.TLS_RSA_WITH_AES_128_CBC_SHA) {
		t.Fatal("advertised-only legacy suite was stripped")
	}
	curves := spec.Extensions[3].(*utls.SupportedCurvesExtension).Curves
	if !slices.Equal(curves, []utls.CurveID{utls.X25519MLKEM768, utls.X25519, utls.CurveP256, utls.CurveP384}) {
		t.Fatalf("curves = %v", curves)
	}
	signatures := spec.Extensions[8].(*utls.SignatureAlgorithmsExtension).SupportedSignatureAlgorithms
	if !slices.Contains(signatures, utls.PKCS1WithSHA1) {
		t.Fatal("advertised-only SHA-1 scheme was stripped")
	}
	shares := spec.Extensions[10].(*utls.KeyShareExtension).KeyShares
	if shares[0].Group != utls.X25519MLKEM768 || shares[1].Group != utls.X25519 || shares[0].Data != nil || shares[1].Data != nil {
		t.Fatal("real key shares must remain generated per connection")
	}
	if spec.Extensions[0].(*utls.SNIExtension).ServerName != "" || len(spec.Extensions[5].(*utls.SessionTicketExtension).Ticket) != 0 {
		t.Fatal("fixture retained a hostname or session secret")
	}
	if !reflect.DeepEqual(groupsBefore, dicttls.DictSupportedGroupsNameIndexed) || !reflect.DeepEqual(groupValuesBefore, dicttls.DictSupportedGroupsValueIndexed) || !reflect.DeepEqual(ciphersBefore, dicttls.DictCipherSuiteNameIndexed) || !reflect.DeepEqual(extensionsBefore, dicttls.DictExtTypeNameIndexed) {
		t.Fatal("private adapter changed an upstream global dictionary")
	}
}

func TestCustomProfileCanonicalIdentity(t *testing.T) {
	profile, err := ParseCustom(fixtureData(t))
	if err != nil {
		t.Fatal(err)
	}
	recipe := fixtureRecipe(t)
	delete(recipe, "min_vers")
	delete(recipe, "max_vers")
	inferred, err := ParseCustom(recipeData(t, recipe))
	if err != nil || profile.Identity() != inferred.Identity() || !bytes.Equal(profile.CanonicalJSON(), inferred.CanonicalJSON()) {
		t.Fatalf("equivalent canonical recipes differ: %v", err)
	}
	for _, mutate := range []func(map[string]any){
		func(recipe map[string]any) {
			suites := recipe["cipher_suites"].([]any)
			suites[0], suites[1] = suites[1], suites[0]
		},
		func(recipe map[string]any) {
			extensions := recipe["extensions"].([]any)
			extensions[0], extensions[1] = extensions[1], extensions[0]
		},
		func(recipe map[string]any) {
			recipeExtension(t, recipe, "application_layer_protocol_negotiation")["protocol_name_list"] = []string{"h2", "http/1.1"}
		},
	} {
		changed := fixtureRecipe(t)
		mutate(changed)
		other, err := ParseCustom(recipeData(t, changed))
		if err != nil || other.Identity() == profile.Identity() {
			t.Fatalf("structural change must change identity: %v", err)
		}
	}
	if profile.Identity() == profileIdentity("custom", "changed") || profile.Identity() == profileIdentity("chrome-133", string(profile.CanonicalJSON())) {
		t.Fatal("identity does not distinguish content and profile policy")
	}
}

func TestCustomProfileImmutableMaterialization(t *testing.T) {
	data := fixtureData(t)
	profile, err := ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	identity := profile.Identity()
	clear(data)
	copyBytes := profile.CanonicalJSON()
	clear(copyBytes)
	first, second := materialize(t, profile), materialize(t, profile)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("fresh custom materializations differ before application")
	}
	first.CipherSuites[0] = 0
	first.CompressionMethods[0] = 1
	first.Extensions[0].(*utls.SNIExtension).ServerName = "changed.invalid"
	first.Extensions[3].(*utls.SupportedCurvesExtension).Curves[0] = utls.CurveP521
	first.Extensions[4].(*utls.SupportedPointsExtension).SupportedPoints[0] = 1
	first.Extensions[5].(*utls.SessionTicketExtension).Ticket = []byte{1, 2, 3}
	first.Extensions[6].(*utls.ALPNExtension).AlpnProtocols[0] = "changed"
	first.Extensions[8].(*utls.SignatureAlgorithmsExtension).SupportedSignatureAlgorithms[0] = 0
	first.Extensions[10].(*utls.KeyShareExtension).KeyShares[0].Data = []byte{1}
	first.Extensions[11].(*utls.PSKKeyExchangeModesExtension).Modes[0] = 0
	first.Extensions[12].(*utls.SupportedVersionsExtension).Versions[0] = 0
	if !reflect.DeepEqual(second, materialize(t, profile)) || profile.Identity() != identity {
		t.Fatal("spec mutations changed another materialization or identity")
	}
	a := buildHello(t, second, "first.invalid").HandshakeState.Hello
	b := buildHello(t, materialize(t, profile), "second.invalid").HandshakeState.Hello
	if a.ServerName != "first.invalid" || b.ServerName != "second.invalid" || bytes.Equal(a.Random, b.Random) || bytes.Equal(a.SessionId, b.SessionId) {
		t.Fatal("SNI/random/session values must come from each new connection")
	}
	if len(a.KeyShares[0].Data) != 1216 || len(a.KeyShares[1].Data) != 32 || bytes.Equal(a.KeyShares[0].Data, b.KeyShares[0].Data) || bytes.Equal(a.KeyShares[1].Data, b.KeyShares[1].Data) {
		t.Fatal("real MLKEM and classical shares must be freshly generated")
	}
	if materialize(t, profile).Extensions[10].(*utls.KeyShareExtension).KeyShares[0].Data != nil || profile.Identity() != identity {
		t.Fatal("dry-run or actual generated key data escaped into the immutable profile")
	}
}

func TestConcurrentProfileMaterialization(t *testing.T) {
	custom, err := ParseCustom(fixtureData(t))
	if err != nil {
		t.Fatal(err)
	}
	chrome, _ := ResolveBuiltin("chrome")
	safari, _ := ResolveBuiltin("safari")
	for _, profile := range []Profile{custom, chrome, safari} {
		t.Run(profile.CanonicalName(), func(t *testing.T) {
			identity := profile.Identity()
			var workers sync.WaitGroup
			for i := 0; i < 32; i++ {
				workers.Go(func() {
					spec, err := profile.Materialize()
					if err != nil {
						t.Error(err)
						return
					}
					connection := utls.UClient(nil, &utls.Config{ServerName: "parallel.invalid"}, utls.HelloCustom)
					if err := connection.ApplyPreset(spec); err != nil {
						t.Error(err)
						return
					}
					if err := connection.BuildHandshakeState(); err != nil {
						t.Error(err)
					}
					clear(spec.CipherSuites)
					if profile.Identity() != identity {
						t.Error("concurrent materialization changed identity")
					}
				})
			}
			workers.Wait()
			if materialize(t, profile).CipherSuites[0] == 0 {
				t.Fatal("concurrent specifications shared storage")
			}
		})
	}
}
