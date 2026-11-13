package tlsprofile

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestClientHelloExportMeasuredRecipe(t *testing.T) {
	exported, err := ExportClientHello(captureFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := ParseCustom(exported)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := ParseCustom(fixtureData(t))
	if err != nil || profile.Identity() != baseline.Identity() || !bytes.Equal(profile.CanonicalJSON(), baseline.CanonicalJSON()) {
		t.Fatal("export changed the measured native-shaped recipe")
	}
	if bytes.Contains(exported, []byte("localhost")) || bytes.Contains(exported, []byte(`"key_exchange":`)) || bytes.Contains(exported, []byte("ServerName")) || bytes.Contains(exported, []byte("SessionId")) {
		t.Fatal("export retained captured SNI, session or real key material")
	}
	var bare utls.ClientHelloSpec
	if json.Unmarshal(exported, &bare) == nil {
		t.Fatal("the pinned library unexpectedly accepts MLKEM without the private symbol adapter")
	}
	for sample := 0; sample < 3; sample++ {
		spec, err := profile.Materialize()
		if err != nil {
			t.Fatal(err)
		}
		for _, extension := range spec.Extensions {
			if shares, ok := extension.(*utls.KeyShareExtension); ok {
				for _, share := range shares.KeyShares {
					if share.Data != nil {
						t.Fatal("real key exchanges were imported instead of generated")
					}
				}
			}
		}
		client := buildHello(t, spec, "localhost")
		raw := captureRecords(client.HandshakeState.Hello.Raw)
		normalized, err := NormalizeClientHello(raw)
		if err != nil || !bytes.Equal(normalized, captureFixture(t)) {
			t.Fatalf("fresh materialization did not reproduce the normalized measured record: %v", err)
		}
	}
}

func TestClientHelloConcurrentExportAndNormalization(t *testing.T) {
	fixture := captureFixture(t)
	profile, err := ParseCustom(fixtureData(t))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for index := 0; index < 32; index++ {
		workers.Go(func() {
			normalized, err := NormalizeClientHello(fixture)
			if err != nil || !bytes.Equal(normalized, fixture) {
				t.Error("concurrent normalization changed the fixture")
				return
			}
			exported, err := ExportClientHello(fixture)
			if err != nil {
				t.Error(err)
				return
			}
			loaded, err := ParseCustom(exported)
			if err != nil || loaded.Identity() != profile.Identity() {
				t.Error("concurrent export changed the immutable recipe")
			}
			clear(normalized)
		})
	}
	workers.Wait()
	if !bytes.Equal(fixture, captureFixture(t)) {
		t.Fatal("concurrent helpers changed caller-owned input")
	}
}

func TestClientHelloExportRejectsUnsupportedData(t *testing.T) {
	fixture := captureFixture(t)
	for _, id := range []uint16{41, 42, 44, 27, 28, 34, 13172, 17513, 17613, 65037, 0xee00} {
		t.Run(strings.Join([]string{"extension", big.NewInt(int64(id)).String()}, "-"), func(t *testing.T) {
			raw := captureExtensionPayload(t, fixture, id, []byte{1, 2, 3}, true)
			if exported, err := ExportClientHello(raw); err == nil || exported != nil {
				t.Fatal("unsupported/resumption extension was exported")
			}
			if _, err := NormalizeClientHello(raw); err == nil {
				t.Fatal("unsupported/resumption data was normalized")
			}
		})
	}
	tests := []struct {
		name    string
		id      uint16
		payload []byte
	}{
		{"ticket", 35, []byte("synthetic-resumption-data")},
		{"renegotiation", 65281, []byte{1, 1}},
		{"extended master secret", 23, []byte{0}},
		{"SCT", 18, []byte{0}},
		{"OCSP", 5, []byte{1, 0, 0, 0, 1, 0}},
		{"points", 11, []byte{1, 1}},
		{"PSK mode", 45, []byte{1, 0}},
		{"versions", 43, []byte{2, 3, 1}},
		{"ALPN", 16, []byte{0, 3, 2, 'h', '3'}},
		{"key bytes", 51, []byte{0, 5, 0, 29, 0, 1, 0}},
		{"SNI type", 0, []byte{0, 4, 1, 0, 1, 'a'}},
		{"SNI extra entry", 0, []byte{0, 8, 0, 0, 1, 'a', 0, 0, 1, 'b'}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := captureExtensionPayload(t, fixture, test.id, test.payload, false)
			if _, err := ExportClientHello(raw); err == nil {
				t.Fatal("unsupported extension-specific data was silently discarded")
			}
		})
	}
}

func TestClientHelloExportDoesNotInferPadding(t *testing.T) {
	for _, length := range []int{0, 1, 32, 4096} {
		raw := captureExtensionPayload(t, captureFixture(t), 21, make([]byte, length), true)
		if _, err := ExportClientHello(raw); err == nil || !strings.Contains(err.Error(), "cannot be inferred") {
			t.Fatal("capture inferred a padding policy")
		}
		if normalized, err := NormalizeClientHello(raw); err != nil || !bytes.Equal(normalized, raw) {
			t.Fatal("comparison erased explicit padding or its length")
		}
	}
	for _, data := range [][]byte{{1}, make([]byte, 4097)} {
		raw := captureExtensionPayload(t, captureFixture(t), 21, data, true)
		if _, err := NormalizeClientHello(raw); err == nil {
			t.Fatal("invalid padding normalized successfully")
		}
	}
}

func TestClientHelloNormalizationEnforcesTheSupportedSchema(t *testing.T) {
	fixture := captureFixture(t)
	tests := []struct {
		name    string
		id      uint16
		payload []byte
	}{
		{"duplicate groups", 10, []byte{0, 4, 0, 29, 0, 29}},
		{"unknown group", 10, []byte{0, 2, 0, 42}},
		{"SHA1 only", 13, []byte{0, 2, 2, 1}},
		{"unknown signature", 13, []byte{0, 2, 0, 1}},
		{"h2 only", 16, []byte{0, 3, 2, 'h', '2'}},
		{"duplicate ALPN", 16, append([]byte{0, 18, 8}, append([]byte("http/1.1"), append([]byte{8}, []byte("http/1.1")...)...)...)},
		{"duplicate versions", 43, []byte{4, 3, 4, 3, 4}},
		{"share missing from groups", 51, append([]byte{0, 137, 0, 25, 0, 133}, make([]byte, 133)...)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := captureExtensionPayload(t, fixture, test.id, test.payload, false)
			if _, err := NormalizeClientHello(raw); err == nil {
				t.Fatal("normalization accepted unsupported or contradictory input")
			}
		})
	}
	hello, _ := parseCapturedHello(fixture)
	first := hello.keyShares[0]
	mlkem := hello.handshake[first.span.from-4 : first.span.to]
	payload := append([]byte{byte(len(mlkem) >> 8), byte(len(mlkem))}, mlkem...)
	if _, err := NormalizeClientHello(captureExtensionPayload(t, fixture, 51, payload, false)); err == nil {
		t.Fatal("MLKEM-only capture was accepted without the required x25519 fallback")
	}
}

func TestClientHelloExportGREASEIsDummyAndNotNormalized(t *testing.T) {
	recipe := fixtureRecipe(t)
	recipe["cipher_suites"] = append([]any{"GREASE"}, recipe["cipher_suites"].([]any)...)
	groups := recipeExtension(t, recipe, "supported_groups")
	groups["named_group_list"] = append([]any{"GREASE"}, groups["named_group_list"].([]any)...)
	shares := recipeExtension(t, recipe, "key_share")
	shares["client_shares"] = append([]any{map[string]any{"group": "GREASE"}}, shares["client_shares"].([]any)...)
	versions := recipeExtension(t, recipe, "supported_versions")
	versions["versions"] = append([]any{"GREASE"}, versions["versions"].([]any)...)
	recipe["extensions"] = append([]any{map[string]any{"name": "GREASE"}}, recipe["extensions"].([]any)...)
	profile, err := ParseCustom(recipeData(t, recipe))
	if err != nil {
		t.Fatal(err)
	}
	client := buildHello(t, materialize(t, profile), "localhost")
	raw := captureRecords(client.HandshakeState.Hello.Raw)
	exported, err := ExportClientHello(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(exported, []byte(`"key_exchange": [`)) || !bytes.Contains(exported, []byte(`"GREASE"`)) {
		t.Fatal("GREASE dummy key exchange was not exported in native numeric-array shape")
	}
	reloaded, err := ParseCustom(exported)
	if err != nil || reloaded.Identity() != profile.Identity() {
		t.Fatalf("GREASE export changed the recipe: %v", err)
	}
	normalized, err := NormalizeClientHello(raw)
	if err != nil {
		t.Fatal(err)
	}
	hello, _ := parseCapturedHello(normalized)
	if !captureGREASE(hello.summary.CipherSuites[0]) || !captureGREASE(hello.summary.Groups[0]) ||
		!captureGREASE(hello.summary.ExtensionIDs[0]) || !captureGREASE(hello.summary.KeyShares[0].Group) {
		t.Fatal("GREASE values were replaced during byte comparison")
	}
	for _, share := range hello.keyShares {
		if captureGREASE(share.group) {
			if !slices.Equal(hello.handshake[share.span.from:share.span.to], []byte{0}) {
				t.Fatal("GREASE dummy data was normalized")
			}
		}
	}
}

func TestClientHelloExportTLS12AndNoALPN(t *testing.T) {
	recipe := fixtureRecipe(t)
	removeExtensions(recipe, "supported_versions", "key_share", "psk_key_exchange_modes", "application_layer_protocol_negotiation")
	delete(recipe, "min_vers")
	delete(recipe, "max_vers")
	profile, err := ParseCustom(recipeData(t, recipe))
	if err != nil {
		t.Fatal(err)
	}
	client := buildHello(t, materialize(t, profile), "localhost")
	exported, err := ExportClientHello(captureRecords(client.HandshakeState.Hello.Raw))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := ParseCustom(exported)
	if err != nil || loaded.Identity() != profile.Identity() {
		t.Fatalf("TLS 1.2/no-ALPN recipe did not round trip: %v", err)
	}
}

func TestClientHelloExportVerifiedTLS(t *testing.T) {
	exported, err := ExportClientHello(captureFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := ParseCustom(exported)
	if err != nil {
		t.Fatal(err)
	}
	certificate, roots := captureTestCertificate(t)
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		for _, condition := range []string{"trusted", "untrusted", "wrong-host"} {
			t.Run(big.NewInt(int64(version)).String()+"/"+condition, func(t *testing.T) {
				listener, err := net.Listen("tcp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				serverConfig := &tls.Config{
					Certificates: []tls.Certificate{certificate}, MinVersion: version, MaxVersion: version,
					NextProtos: []string{"http/1.1"}, CipherSuites: []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
				}
				done := make(chan error, 1)
				go func() {
					connection, err := listener.Accept()
					if err != nil {
						done <- err
						return
					}
					defer connection.Close()
					connection.SetDeadline(time.Now().Add(3 * time.Second))
					done <- tls.Server(connection, serverConfig).Handshake()
				}()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				connection, err := (&net.Dialer{}).DialContext(ctx, "tcp4", listener.Addr().String())
				if err != nil {
					t.Fatal(err)
				}
				defer connection.Close()
				config := &utls.Config{ServerName: "localhost", RootCAs: roots}
				if condition == "untrusted" {
					config.RootCAs = x509.NewCertPool()
				} else if condition == "wrong-host" {
					config.ServerName = "wrong.invalid"
				}
				client := utls.UClient(connection, config, utls.HelloCustom)
				if err := client.ApplyPreset(materialize(t, profile)); err != nil {
					t.Fatal(err)
				}
				err = client.HandshakeContext(ctx)
				connection.Close()
				serverErr := <-done
				if condition != "trusted" {
					if err == nil {
						t.Fatal("invalid trust or hostname was accepted")
					}
					return
				}
				if err != nil || serverErr != nil {
					t.Fatalf("verified local TLS failed: client %v, server %v", err, serverErr)
				}
				state := client.ConnectionState()
				if state.Version != version || state.NegotiatedProtocol != "http/1.1" || len(state.VerifiedChains) == 0 {
					t.Fatal("exported recipe did not negotiate verified TLS with its measured ALPN")
				}
			})
		}
	}
}

func captureTestCertificate(t testing.TB) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}
