//go:build integration && linux

package integration

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

func TestBinaryTLSProfileCLIContract(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	guard := newBinaryTLSProxy(t, nil)
	fixture := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("configuration validation attempted upstream HTTP")
	}))
	env := binaryTLSChildEnv(t, trust, guard.URL(), nil)
	dir, cwd := t.TempDir(), t.TempDir()
	writeBinaryTLSFile(t, filepath.Join(dir, "profiles", "bun.json"), binaryTLSRecipe(t, false))
	writeBinaryTLSFile(t, filepath.Join(dir, "profiles", "private-source-malformed.json"), []byte(`{"synthetic-private-content":true}`))
	writeBinaryTLSFile(t, filepath.Join(dir, "profiles", "private-source-oversized.json"), bytes.Repeat([]byte{' '}, tlsprofile.MaxSpecBytes+1))
	fifo := filepath.Join(dir, "profiles", "private-source-fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"hcl", "json"} {
		t.Run(format+"/invalid/root-default", func(t *testing.T) {
			text := binaryTLSValidationConfig(t, format, "127.0.0.1:0", fixture.URL(), nil, nil)
			if format == "json" {
				var root map[string]json.RawMessage
				if err := json.Unmarshal([]byte(text), &root); err != nil {
					t.Fatal(err)
				}
				root["tls_client_hello"] = json.RawMessage(`"native"`)
				data, err := json.Marshal(root)
				if err != nil {
					t.Fatal(err)
				}
				text = string(data)
			} else {
				text = "tls_client_hello = \"native\"\n" + text
			}
			path := filepath.Join(dir, "config", "config."+format)
			writeBinaryTLSFile(t, path, []byte(text))
			if out, err := runBinaryTLSCLI(t, cwd, env, "validate", "--config", path); err == nil || !strings.Contains(out, "provider-only") {
				t.Fatalf("root default bypass: %v\n%s", err, out)
			}
		})
		for _, selector := range []string{"", "native", "chrome", "chrome-133", "safari", "safari-16.0", "custom"} {
			name := selector
			if name == "" {
				name = "omitted-native"
			}
			t.Run(format+"/valid/"+name, func(t *testing.T) {
				fields := map[string]any{}
				if selector != "" {
					fields["tls_client_hello"] = selector
				}
				if selector == "custom" {
					fields["tls_client_hello_spec_file"] = "../profiles/./bun.json"
				}
				path := filepath.Join(dir, "config", "config."+format)
				writeBinaryTLSFile(t, path, []byte(binaryTLSValidationConfig(t, format, "127.0.0.1:0", fixture.URL(), fields, nil)))
				out, err := runBinaryTLSCLI(t, cwd, env, "validate", "--config", path)
				if err != nil {
					t.Fatalf("valid %s configuration: %v\n%s", selector, err, out)
				}
			})
		}
		cases := []struct {
			name    string
			fields  map[string]any
			derived map[string]any
			want    string
		}{
			{"empty-selector", map[string]any{"tls_client_hello": ""}, nil, "tls_client_hello"},
			{"wrong-case", map[string]any{"tls_client_hello": "Chrome"}, nil, "tls_client_hello"},
			{"deferred-firefox", map[string]any{"tls_client_hello": "firefox-120"}, nil, "tls_client_hello"},
			{"unsupported-pinned-version", map[string]any{"tls_client_hello": "chrome-155"}, nil, "tls_client_hello"},
			{"bun-is-not-an-alias", map[string]any{"tls_client_hello": "bun"}, nil, "tls_client_hello"},
			{"boringssl-is-not-a-profile", map[string]any{"tls_client_hello": "boringssl"}, nil, "tls_client_hello"},
			{"null-selector", map[string]any{"tls_client_hello": nil}, nil, "non-null string"},
			{"custom-missing-reference", map[string]any{"tls_client_hello": "custom"}, nil, "tls_client_hello_spec_file"},
			{"noncustom-empty-reference", map[string]any{"tls_client_hello": "chrome", "tls_client_hello_spec_file": ""}, nil, "cannot be declared"},
			{"missing-file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "../profiles/private-source-missing.json"}, nil, "cannot inspect"},
			{"malformed-file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "../profiles/private-source-malformed.json"}, nil, "tls_client_hello_spec_file"},
			{"oversized-file", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "../profiles/private-source-oversized.json"}, nil, "65536 bytes"},
			{"directory", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "../profiles"}, nil, "regular file"},
			{"fifo-does-not-block", map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "../profiles/private-source-fifo"}, nil, "regular file"},
			{"disabled-invalid-profile", map[string]any{"enabled": false, "api_key": "", "tls_client_hello": "Chrome"}, nil, "tls_client_hello"},
			{"derived-explicit-native", nil, map[string]any{"tls_client_hello": "native"}, "derived provider cannot declare"},
			{"derived-empty-reference", nil, map[string]any{"tls_client_hello_spec_file": ""}, "derived provider cannot declare"},
		}
		for _, tc := range cases {
			t.Run(format+"/invalid/"+tc.name, func(t *testing.T) {
				path := filepath.Join(dir, "config", "config."+format)
				writeBinaryTLSFile(t, path, []byte(binaryTLSValidationConfig(t, format, "127.0.0.1:0", fixture.URL(), tc.fields, tc.derived)))
				start := time.Now()
				out, err := runBinaryTLSCLI(t, cwd, env, "validate", "--config", path)
				if err == nil || !strings.Contains(out, tc.want) {
					t.Fatalf("invalid profile was accepted or uncontrolled: %v\n%s", err, out)
				}
				for _, secret := range []string{"private-source", "synthetic-private-content", "synthetic-provider"} {
					if strings.Contains(out, secret) {
						t.Fatalf("validation exposed synthetic private data: %s", secret)
					}
				}
				if tc.name == "fifo-does-not-block" && time.Since(start) > 3*time.Second {
					t.Fatal("nonregular profile reference blocked the CLI")
				}
			})
		}
		for _, name := range []string{"disabled-custom", "derived-custom"} {
			t.Run(format+"/valid/"+name, func(t *testing.T) {
				fields := map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "../profiles/bun.json"}
				var derived map[string]any
				if name == "disabled-custom" {
					fields["enabled"], fields["api_key"] = false, ""
				} else {
					derived = map[string]any{}
				}
				path := filepath.Join(dir, "config", "config."+format)
				writeBinaryTLSFile(t, path, []byte(binaryTLSValidationConfig(t, format, "127.0.0.1:0", fixture.URL(), fields, derived)))
				if out, err := runBinaryTLSCLI(t, cwd, env, "validate", "--config", path); err != nil {
					t.Fatalf("valid disabled/inherited custom profile: %v\n%s", err, out)
				}
			})
		}
		for _, absolute := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/inline/absolute-%t", format, absolute), func(t *testing.T) {
				reference := "bun.json"
				if absolute {
					reference = filepath.Join(dir, "profiles", reference)
				}
				text := binaryTLSValidationConfig(t, format, "127.0.0.1:0", fixture.URL(), map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": reference}, nil)
				inlineEnv := binaryTLSChildEnv(t, trust, guard.URL(), map[string]string{"AIPROXY_CONFIG": text})
				out, err := runBinaryTLSCLI(t, filepath.Join(dir, "profiles"), inlineEnv, "validate")
				if (err == nil) != absolute || (!absolute && !strings.Contains(out, "absolute reference")) {
					t.Fatalf("inline reference depended on invocation cwd: %v\n%s", err, out)
				}
			})
		}
	}
	t.Run("serve-rejects-invalid-before-listening", func(t *testing.T) {
		addr := freeAddr(t)
		path := filepath.Join(dir, "config", "invalid-serve.hcl")
		writeBinaryTLSFile(t, path, []byte(binaryTLSValidationConfig(t, "hcl", addr, fixture.URL(), map[string]any{"tls_client_hello": "custom", "tls_client_hello_spec_file": "private-source-missing"}, nil)))
		if out, err := runBinaryTLSCLI(t, cwd, env, "serve", "--config", path); err == nil || !strings.Contains(out, "tls_client_hello_spec_file") {
			t.Fatalf("serve accepted an unresolved profile: %v\n%s", err, out)
		}
	})
	fixture.observed.mu.Lock()
	defer fixture.observed.mu.Unlock()
	if len(fixture.observed.hellos) != 0 || len(fixture.observed.calls) != 0 {
		t.Fatal("configuration consumers contacted the provider")
	}
}

func TestBinaryTLSProfileConfigureConversionAndLexicalSource(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	guard := newBinaryTLSProxy(t, nil)
	fixture := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"configured-tls","choices":[]}`)
	}))
	env := binaryTLSChildEnv(t, trust, guard.URL(), nil)
	cwd := t.TempDir()
	t.Run("configure-convert-serve-reset", func(t *testing.T) {
		dir, addr := t.TempDir(), freeAddr(t)
		source := filepath.Join(dir, "profiles", "bun.json")
		recipe := binaryTLSRecipe(t, false)
		writeBinaryTLSFile(t, source, recipe)
		path := filepath.Join(dir, "config.hcl")
		writeBinaryTLSFile(t, path, []byte(binaryTLSConfig(addr, binaryTLSProvider("local", fixture.URL(), "synthetic-provider", "chrome", "", ""))))
		configure := func(extra ...string) (string, error) {
			args := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "local"}
			return runBinaryTLSCLI(t, cwd, env, append(args, extra...)...)
		}
		if out, err := configure("--tls-client-hello", "custom", "--tls-client-hello-spec-file", "profiles/./bun.json"); err != nil {
			t.Fatalf("configure source-relative custom: %v\n%s", err, out)
		}
		if out, err := configure("--display-name", "Preserved TLS"); err != nil {
			t.Fatalf("partial edit: %v\n%s", err, out)
		}
		runtime, err := config.LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		provider, _ := runtime.Catalog.Provider("local")
		if provider.TLSClientHello != "custom" || provider.TLSClientHelloSpecFile != "profiles/./bun.json" || provider.DisplayName != "Preserved TLS" || provider.APIKey != "synthetic-provider" {
			t.Fatal("binary partial edit lost declared spelling or unrelated provider fields")
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, flags := range [][]string{
			{"--tls-client-hello", "Chrome"},
			{"--tls-client-hello", "native", "--tls-client-hello-spec-file", "private-source"},
		} {
			if out, err := configure(flags...); err == nil || !strings.Contains(out, "tls_client_hello") || strings.Contains(out, "private-source") {
				t.Fatalf("invalid configure profile: %v\n%s", err, out)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected TLS edit changed the configuration")
			}
		}
		jsonPath := filepath.Join(t.TempDir(), "converted.json")
		if out, err := runBinaryTLSCLI(t, cwd, env, "convert", jsonPath, "--config", path); err != nil {
			t.Fatalf("HCL to relocated JSON: %v\n%s", err, out)
		}
		runtime, err = config.LoadFile(jsonPath)
		if err != nil {
			t.Fatal(err)
		}
		provider, _ = runtime.Catalog.Provider("local")
		if provider.TLSClientHelloSpecFile != source {
			t.Fatal("binary conversion did not resolve the custom reference for relocation")
		}
		hclPath := filepath.Join(t.TempDir(), "roundtrip.hcl")
		if out, err := runBinaryTLSCLI(t, cwd, env, "convert", hclPath, "--config", jsonPath); err != nil {
			t.Fatalf("relocated JSON to HCL: %v\n%s", err, out)
		}
		if out, err := runBinaryTLSCLI(t, cwd, env, "validate", "--config", hclPath); err != nil {
			t.Fatalf("validate relocated round trip: %v\n%s", err, out)
		}
		srv := startBinaryTLSServer(t, jsonPath, addr, cwd, env)
		if status, body := postChat(t, srv, "synthetic-caller", `{"model":"local/chat","messages":[]}`); status != http.StatusOK || !strings.Contains(body, "configured-tls") {
			t.Fatalf("converted custom binary inference: %d %s", status, body)
		}
		calls := fixture.observed.Calls("/v1/chat/completions")
		requireBinaryTLSProfile(t, "custom", recipe, fixture.observed.Hello(t, calls[len(calls)-1].remote), nil)
		srv.Stop(t)
		if out, err := configure("--tls-client-hello", "native"); err != nil {
			t.Fatalf("explicit native reset: %v\n%s", err, out)
		}
		after, err := os.ReadFile(path)
		if err != nil || bytes.Contains(after, []byte("tls_client_hello_spec_file")) {
			t.Fatal("native reset retained the stale custom reference")
		}
	})
	t.Run("config-symlink-uses-lexical-directory", func(t *testing.T) {
		dir, addr := t.TempDir(), freeAddr(t)
		realPath := filepath.Join(dir, "real", "config.hcl")
		linkPath := filepath.Join(dir, "links", "config.hcl")
		writeBinaryTLSFile(t, realPath, []byte(binaryTLSConfig(addr, binaryTLSProvider("local", fixture.URL(), "synthetic-provider", "custom", "bun.json", ""))))
		writeBinaryTLSFile(t, filepath.Join(dir, "real", "bun.json"), binaryTLSRecipe(t, false))
		recipe := binaryTLSRecipe(t, true)
		writeBinaryTLSFile(t, filepath.Join(dir, "links", "bun.json"), recipe)
		if err := os.Symlink(realPath, linkPath); err != nil {
			t.Fatal(err)
		}
		invalidInline := binaryTLSConfig(addr, binaryTLSProvider("local", fixture.URL(), "synthetic-provider", "custom", "missing-relative.json", ""))
		env := binaryTLSChildEnv(t, trust, guard.URL(), map[string]string{"AIPROXY_CONFIG": invalidInline})
		srv := startBinaryTLSServer(t, linkPath, addr, cwd, env)
		if status, body := postChat(t, srv, "", `{"model":"local/chat","messages":[]}`); status != http.StatusOK {
			t.Fatalf("lexical custom source: %d %s", status, body)
		}
		calls := fixture.observed.Calls("/v1/chat/completions")
		call := calls[len(calls)-1]
		if call.protoMajor != 2 {
			t.Fatal("binary followed the physical config target directory or inline env instead of the explicit lexical source")
		}
		requireBinaryTLSProfile(t, "custom", recipe, fixture.observed.Hello(t, call.remote), nil)
	})
}

func TestBinaryTLSProfileInferenceRouteMatrix(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		for _, alpn := range []string{"http/1.1", "h2"} {
			for _, route := range []string{"direct", "http-connect"} {
				t.Run(fmt.Sprintf("tls-%x/%s/%s", version, alpn, route), func(t *testing.T) {
					fixture := newBinaryTLSFixture(t, trust, version, alpn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"binary-tls","choices":[]}`)
					}))
					baseURL, host := fixture.URL(), "localhost"
					var targets map[string]string
					if route == "http-connect" {
						baseURL, host = fixture.ProxyOrigin(), binaryTLSProxyHost
						u, _ := url.Parse(baseURL)
						targets = map[string]string{u.Host: fixture.server.Listener.Addr().String()}
					}
					proxy := newBinaryTLSProxy(t, targets)
					native := binaryTLSNativeHello(t, fixture, trust, host)
					dir, addr := t.TempDir(), freeAddr(t)
					h1Recipe, h2Recipe := binaryTLSRecipe(t, false), binaryTLSRecipe(t, true)
					writeBinaryTLSFile(t, filepath.Join(dir, "profiles", "bun.json"), h1Recipe)
					writeBinaryTLSFile(t, filepath.Join(dir, "profiles", "h2-variant.json"), h2Recipe)
					providers := []struct {
						name, selector, source, family string
						recipe                         []byte
					}{
						{"default", "", "", "native", nil},
						{"native", "native", "", "native", nil},
						{"chrome", "chrome", "", "chrome", nil},
						{"chrome-pinned", "chrome-133", "", "chrome", nil},
						{"safari", "safari", "", "safari", nil},
						{"safari-pinned", "safari-16.0", "", "safari", nil},
						{"custom", "custom", "profiles/./bun.json", "custom-h1", h1Recipe},
						{"custom-h2", "custom", "profiles/h2-variant.json", "custom-h2", h2Recipe},
						{"derived", "custom", "", "custom-h1", h1Recipe},
					}
					var providerConfig strings.Builder
					for _, provider := range providers {
						if provider.name == "derived" {
							providerConfig.WriteString("provider \"openai-compatible\" \"derived\" {\n  extends = \"custom\"\n  api_key = \"synthetic-derived\"\n}\n")
							continue
						}
						providerConfig.WriteString(binaryTLSProvider(provider.name, baseURL, "synthetic-"+provider.name, provider.selector, provider.source, ""))
					}
					path := filepath.Join(dir, "config.hcl")
					writeBinaryTLSFile(t, path, []byte(binaryTLSConfig(addr, providerConfig.String())))
					srv := startBinaryTLSServer(t, path, addr, t.TempDir(), binaryTLSChildEnv(t, trust, proxy.URL(), nil))
					connections := make(map[string]string)
					suites := make(map[string][]uint16)
					for i, provider := range providers {
						payload := fmt.Sprintf(`{"model":%q,"messages":[]}`, provider.name+"/chat")
						if status, body := postChat(t, srv, "synthetic-caller", payload); status != http.StatusOK || !strings.Contains(body, "binary-tls") {
							t.Fatalf("provider %s: %d %s", provider.name, status, body)
						}
						calls := fixture.observed.Calls("/v1/chat/completions")
						if len(calls) != i+1 {
							t.Fatal("direct request silently retried or selected another provider")
						}
						call := calls[i]
						if call.method != http.MethodPost || call.authorization != "Bearer synthetic-"+provider.name || call.proxyAuthorization != "" || !strings.Contains(call.body, `"model":"upstream-chat"`) {
							t.Fatal("TLS profile changed credential isolation/model rewriting")
						}
						wantProto, wantALPN := 1, "http/1.1"
						if alpn == "h2" && provider.family != "custom-h1" {
							wantProto, wantALPN = 2, "h2"
						}
						if call.protoMajor != wantProto || call.alpn != wantALPN || call.version != version {
							t.Fatalf("actual negotiated HTTP/TLS: %+v, want HTTP/%d ALPN=%s TLS=%x", call, wantProto, wantALPN, version)
						}
						hello := fixture.observed.Hello(t, call.remote)
						if hello.serverName != host {
							t.Fatalf("runtime SNI = %q, want %q", hello.serverName, host)
						}
						requireBinaryTLSProfile(t, provider.selector, provider.recipe, hello, &native)
						if previous := connections[provider.family]; previous != "" && previous != call.remote {
							t.Fatal("equivalent/default/inherited profiles lost connection reuse")
						}
						for family, remote := range connections {
							if family != provider.family && remote == call.remote {
								t.Fatal("same timeout merged incompatible profile transports")
							}
						}
						connections[provider.family], suites[provider.family] = call.remote, hello.summary.CipherSuites
					}
					for _, pair := range [][2]string{{"native", "chrome"}, {"chrome", "safari"}, {"safari", "custom-h1"}} {
						if slices.Equal(suites[pair[0]], suites[pair[1]]) {
							t.Fatal("declared profiles did not produce distinct actual ClientHellos")
						}
					}
					if route == "http-connect" {
						requireBinaryTLSConnect(t, proxy, 5)
						for _, call := range proxy.Calls()[1:] {
							if call.headers.Get("User-Agent") != "" {
								t.Fatal("profile CONNECT forwarded provider/caller user-agent")
							}
						}
					} else {
						requireBinaryTLSConnect(t, proxy, 0)
					}
					srv.Stop(t)
					for _, remote := range connections {
						waitBinaryTLSCondition(t, "binary shutdown TLS socket cleanup", func() bool { return fixture.observed.Closed(remote) })
					}
				})
			}
		}
	}
}

func TestBinaryTLSProfileAliasAdmittedAcrossReload(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	guard := newBinaryTLSProxy(t, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	enterOnce := sync.OnceFunc(func() { close(entered) })
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()
	fixture := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer synthetic-primary-old" {
			enterOnce()
			select {
			case <-release:
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"error":"synthetic-primary-rejected"}`)
			case <-r.Context().Done():
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":%q,"choices":[]}`, r.Header.Get("Authorization"))
	}))
	dir, addr := t.TempDir(), freeAddr(t)
	recipe := binaryTLSRecipe(t, false)
	writeBinaryTLSFile(t, filepath.Join(dir, "profiles", "bun.json"), recipe)
	configuration := func(next bool) string {
		primary, backup, source, suffix := "chrome-133", "safari-16.0", "", "old"
		if next {
			primary, backup, source, suffix = "native", "custom", "profiles/bun.json", "new"
		}
		return binaryTLSConfig(addr,
			binaryTLSProvider("primary", fixture.URL(), "synthetic-primary-"+suffix, primary, "", "")+
				binaryTLSProvider("backup", fixture.URL(), "synthetic-backup-"+suffix, backup, source, "")) + `alias "route" {
  algorithm = "round_robin"
  target {
    provider = "primary"
    model = "chat"
  }
  target {
    provider = "backup"
    model = "chat"
  }
}
`
	}
	path := filepath.Join(dir, "config.hcl")
	writeBinaryTLSFile(t, path, []byte(configuration(false)))
	srv := startBinaryTLSServer(t, path, addr, t.TempDir(), binaryTLSChildEnv(t, trust, guard.URL(), nil))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type outcome struct {
		status int
		body   string
		err    error
	}
	done := make(chan outcome, 1)
	finished := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		releaseOnce()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("admitted alias HTTP worker did not finish")
		}
	})
	go func() {
		defer close(finished)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/v1/chat/completions", strings.NewReader(`{"model":"alias/route","messages":[]}`))
		if err != nil {
			done <- outcome{err: err}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := srv.client.Do(req)
		if err != nil {
			done <- outcome{err: err}
			return
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		done <- outcome{status: resp.StatusCode, body: string(body), err: err}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("binary did not admit the first alias attempt")
	}
	initial := fixture.observed.Calls("")[0]
	writeBinaryTLSFile(t, path, []byte(configuration(true)))
	reloadBinaryTLS(t, srv, true)
	if status, body := postChat(t, srv, "", `{"model":"backup/chat","messages":[]}`); status != http.StatusOK || !strings.Contains(body, "synthetic-backup-new") {
		t.Fatalf("new generation direct request: %d %s", status, body)
	}
	if fixture.observed.Closed(initial.remote) {
		t.Fatal("reload canceled the safely admitted alias request")
	}
	releaseOnce()
	select {
	case result := <-done:
		if result.err != nil || result.status != http.StatusOK || !strings.Contains(result.body, "synthetic-backup-old") {
			t.Fatalf("admitted alias failed or crossed generations: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("admitted alias failover did not complete")
	}
	calls := fixture.observed.Calls("/v1/chat/completions")
	if len(calls) != 3 || calls[0].authorization != "Bearer synthetic-primary-old" || calls[1].authorization != "Bearer synthetic-backup-new" || calls[2].authorization != "Bearer synthetic-backup-old" {
		t.Fatalf("alias attempt order/credential generation: %+v", calls)
	}
	requireBinaryTLSProfile(t, "chrome-133", nil, fixture.observed.Hello(t, calls[0].remote), nil)
	requireBinaryTLSProfile(t, "custom", recipe, fixture.observed.Hello(t, calls[1].remote), nil)
	requireBinaryTLSProfile(t, "safari-16.0", nil, fixture.observed.Hello(t, calls[2].remote), nil)
	for _, i := range []int{0, 2} {
		waitBinaryTLSCondition(t, "retired admitted alias TLS socket", func() bool { return fixture.observed.Closed(calls[i].remote) })
	}
}

func TestBinaryTLSProfileCustomReloadStreamingLifecycle(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	guard := newBinaryTLSProxy(t, nil)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()
	fixture := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		if !payload.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"before-reload\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case <-r.Context().Done():
		}
	}))
	dir, addr := t.TempDir(), freeAddr(t)
	source, path := filepath.Join(dir, "profiles", "bun.json"), filepath.Join(dir, "config.hcl")
	h1Recipe, h2Recipe := binaryTLSRecipe(t, false), binaryTLSRecipe(t, true)
	writeBinaryTLSFile(t, source, h1Recipe)
	text := binaryTLSConfig(addr, binaryTLSProvider("local", fixture.URL(), "synthetic-provider", "custom", "profiles/./bun.json", ""))
	text = strings.Replace(text, `upstream_header_timeout = "2s"`, `upstream_header_timeout = "100ms"`, 1)
	writeBinaryTLSFile(t, path, []byte(text))
	srv := startBinaryTLSServer(t, path, addr, t.TempDir(), binaryTLSChildEnv(t, trust, guard.URL(), nil))
	start := time.Now()
	resp := postChatStreaming(t, srv, `{"model":"local/chat","stream":true,"messages":[]}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("initial SSE status = %d", resp.StatusCode)
	}
	reader := bufio.NewReader(resp.Body)
	first, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(first, "before-reload") {
		t.Fatalf("initial SSE data: %s %v", first, err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	stream := fixture.observed.Calls("")[0]
	requireBinaryTLSProfile(t, "custom", h1Recipe, fixture.observed.Hello(t, stream.remote), nil)
	if status, body := postChat(t, srv, "", `{"model":"local/chat","messages":[]}`); status != http.StatusOK {
		t.Fatalf("initial independent idle connection: %d %s", status, body)
	}
	idle := fixture.observed.Calls("")[1]
	if idle.remote == stream.remote || bytes.Equal(fixture.observed.Hello(t, idle.remote).raw, fixture.observed.Hello(t, stream.remote).raw) {
		t.Fatal("new custom connection reused admitted stream socket/cryptographic bytes")
	}
	for _, h2 := range []bool{true, false, true} {
		recipe, wantProto := h1Recipe, 1
		if h2 {
			recipe, wantProto = h2Recipe, 2
		}
		writeBinaryTLSFile(t, source, recipe)
		reloadBinaryTLS(t, srv, true)
		if status, body := postChat(t, srv, "", `{"model":"local/chat","messages":[]}`); status != http.StatusOK {
			t.Fatalf("new work after same-path recipe reload: %d %s", status, body)
		}
		calls := fixture.observed.Calls("")
		current := calls[len(calls)-1]
		if current.protoMajor != wantProto || current.remote == idle.remote || current.remote == stream.remote {
			t.Fatal("same-path content change reused an incompatible/retired connection")
		}
		requireBinaryTLSProfile(t, "custom", recipe, fixture.observed.Hello(t, current.remote), nil)
		waitBinaryTLSCondition(t, "superseded idle TLS connection", func() bool { return fixture.observed.Closed(idle.remote) })
		if fixture.observed.Closed(stream.remote) {
			t.Fatal("successful reload terminated the admitted SSE")
		}
		idle = current
	}
	writeBinaryTLSFile(t, source, []byte(`{"synthetic-private-replacement":`))
	reloadBinaryTLS(t, srv, false)
	if fixture.observed.Closed(stream.remote) {
		t.Fatal("rejected reload terminated the admitted SSE")
	}
	if remaining := 200*time.Millisecond - time.Since(start); remaining > 0 {
		time.Sleep(remaining)
	}
	releaseOnce()
	tail, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(tail), "[DONE]") {
		t.Fatalf("SSE did not survive reloads/header deadline: %s %v", tail, err)
	}
	_ = resp.Body.Close()
	waitBinaryTLSCondition(t, "retired SSE connection after EOF", func() bool { return fixture.observed.Closed(stream.remote) })
	fixture.server.CloseClientConnections()
	waitBinaryTLSCondition(t, "forced fresh cached-profile connection", func() bool { return fixture.observed.Closed(idle.remote) })
	if status, body := postChat(t, srv, "", `{"model":"local/chat","messages":[]}`); status != http.StatusOK {
		t.Fatalf("failed reload lost cached custom bytes on a fresh connection: %d %s", status, body)
	}
	calls := fixture.observed.Calls("")
	cached := calls[len(calls)-1]
	if cached.remote == idle.remote || cached.protoMajor != 2 {
		t.Fatal("cached custom profile proof did not use a fresh h2 connection")
	}
	requireBinaryTLSProfile(t, "custom", h2Recipe, fixture.observed.Hello(t, cached.remote), nil)
	srv.Stop(t)
	waitBinaryTLSCondition(t, "current cached-profile socket on shutdown", func() bool { return fixture.observed.Closed(cached.remote) })
}

func TestBinaryTLSProfileCertificateVerification(t *testing.T) {
	for _, selector := range []string{"native", "chrome-133", "safari-16.0", "custom"} {
		for _, failure := range []string{"untrusted", "hostname"} {
			for _, route := range []string{"direct", "http-connect"} {
				t.Run(selector+"/"+failure+"/"+route, func(t *testing.T) {
					serverCert := newBinaryTLSCertificate(t)
					trust := newBinaryTLSCertificate(t)
					if failure == "hostname" {
						serverCert = newBinaryTLSCertificate(t, "wrong.integration.test")
						trust = serverCert
					}
					fixture := newBinaryTLSFixture(t, serverCert, tls.VersionTLS13, "h2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
						t.Error("unverified TLS reached provider HTTP application")
					}))
					baseURL := fixture.URL()
					var targets map[string]string
					if route == "http-connect" {
						baseURL = fixture.ProxyOrigin()
						u, _ := url.Parse(baseURL)
						targets = map[string]string{u.Host: fixture.server.Listener.Addr().String()}
					}
					proxy := newBinaryTLSProxy(t, targets)
					dir, addr, source := t.TempDir(), freeAddr(t), ""
					var recipe []byte
					if selector == "custom" {
						source, recipe = "bun.json", binaryTLSRecipe(t, false)
						writeBinaryTLSFile(t, filepath.Join(dir, source), recipe)
					}
					path := filepath.Join(dir, "config.hcl")
					writeBinaryTLSFile(t, path, []byte(binaryTLSConfig(addr, binaryTLSProvider("local", baseURL, "synthetic-provider", selector, source, ""))))
					srv := startBinaryTLSServer(t, path, addr, t.TempDir(), binaryTLSChildEnv(t, trust, proxy.URL(), nil))
					if status, body := postChat(t, srv, "synthetic-caller", `{"model":"local/chat","messages":[]}`); status != http.StatusBadGateway || !strings.Contains(body, "upstream_error") || strings.Contains(body, "synthetic-provider") {
						t.Fatalf("unsafe TLS was accepted or leaked credential: %d %s", status, body)
					}
					if len(fixture.observed.Calls("")) != 0 {
						t.Fatal("certificate/hostname failure sent provider HTTP traffic")
					}
					fixture.observed.mu.Lock()
					var hellos []binaryTLSHello
					for _, hello := range fixture.observed.hellos {
						hellos = append(hellos, hello)
					}
					fixture.observed.mu.Unlock()
					if len(hellos) != 1 {
						t.Fatalf("certificate failure changed profile/retried: %d ClientHellos", len(hellos))
					}
					requireBinaryTLSProfile(t, selector, recipe, hellos[0], nil)
					if failure == "untrusted" {
						srv.WaitLogContains(t, "certificate signed by unknown authority")
					} else {
						srv.WaitLogContains(t, "certificate is valid for wrong.integration.test")
					}
					wantTunnels := 0
					if route == "http-connect" {
						wantTunnels = 1
					}
					requireBinaryTLSConnect(t, proxy, wantTunnels)
				})
			}
		}
	}
}

func TestBinaryTLSProfileSelectedSignatureGuard(t *testing.T) {
	for _, route := range []string{"direct", "http-connect"} {
		t.Run(route, func(t *testing.T) {
			trust := newBinaryTLSCertificate(t)
			observed := newBinaryTLSObservations()
			var applicationCalls atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				applicationCalls.Add(1)
				_, _ = io.WriteString(w, `{"choices":[]}`)
			}))
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.Config.ReadHeaderTimeout = 3 * time.Second
			server.Listener = utls.NewListener(binaryTLSCaptureListener{Listener: server.Listener, observed: observed}, &utls.Config{
				Certificates: []utls.Certificate{{
					Certificate: trust.certificate.Certificate, PrivateKey: trust.certificate.PrivateKey,
					SupportedSignatureAlgorithms: []utls.SignatureScheme{utls.ECDSAWithSHA1},
				}},
				MinVersion: utls.VersionTLS12, MaxVersion: utls.VersionTLS12,
				CipherSuites: []uint16{utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}, NextProtos: []string{"http/1.1"},
				GetConfigForClient: func(info *utls.ClientHelloInfo) (*utls.Config, error) {
					remote := info.Conn.RemoteAddr().String()
					observed.mu.Lock()
					hello := observed.hellos[remote]
					hello.serverName, hello.versions = info.ServerName, slices.Clone(info.SupportedVersions)
					observed.hellos[remote] = hello
					observed.mu.Unlock()
					return nil, nil
				},
			})
			server.Start()
			t.Cleanup(server.Close)
			baseURL := "https://" + strings.Replace(server.Listener.Addr().String(), "127.0.0.1", "localhost", 1)
			var targets map[string]string
			if route == "http-connect" {
				baseURL = strings.Replace(baseURL, "localhost", binaryTLSProxyHost, 1)
				u, _ := url.Parse(baseURL)
				targets = map[string]string{u.Host: server.Listener.Addr().String()}
			}
			proxy := newBinaryTLSProxy(t, targets)
			addr := freeAddr(t)
			path := writeConfig(t, binaryTLSConfig(addr, binaryTLSProvider("local", baseURL, "synthetic-provider", "safari-16.0", "", "")))
			srv := startBinaryTLSServer(t, path, addr, t.TempDir(), binaryTLSChildEnv(t, trust, proxy.URL(), nil))
			if status, body := postChat(t, srv, "", `{"model":"local/chat","messages":[]}`); status != http.StatusBadGateway || !strings.Contains(body, "upstream_error") {
				t.Fatalf("selected SHA-1 reached inference: %d %s", status, body)
			}
			srv.WaitLogContains(t, "unsafe selected upstream TLS 1.2 signature 0x0203")
			if applicationCalls.Load() != 0 {
				t.Fatal("selected signature guard allowed provider HTTP application traffic")
			}
			observed.mu.Lock()
			var hellos []binaryTLSHello
			for _, hello := range observed.hellos {
				hellos = append(hellos, hello)
			}
			observed.mu.Unlock()
			if len(hellos) != 1 || !slices.Contains(hellos[0].summary.SignatureAlgorithms, uint16(utls.ECDSAWithSHA1)) {
				t.Fatal("signature guard fixture did not exercise an advertised legacy signature")
			}
			requireBinaryTLSProfile(t, "safari-16.0", nil, hellos[0], nil)
			wantTunnels := 0
			if route == "http-connect" {
				wantTunnels = 1
			}
			requireBinaryTLSConnect(t, proxy, wantTunnels)
		})
	}
}

func TestBinaryTLSProfileHealthAndModelDiscovery(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	guard := newBinaryTLSProxy(t, nil)
	for _, name := range []string{"native", "chrome-133", "safari-16.0", "custom-h1", "custom-h2-variant"} {
		t.Run(name, func(t *testing.T) {
			fixture := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/health":
					_, _ = io.WriteString(w, "ready")
				case "/v1/models":
					_, _ = io.WriteString(w, `{"data":[{"id":"upstream-chat"},{"id":"discovered-only"}]}`)
				case "/v1/chat/completions":
					_, _ = io.WriteString(w, `{"choices":[]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			native := binaryTLSNativeHello(t, fixture, trust, "localhost")
			dir, addr, source, selector := t.TempDir(), freeAddr(t), "", name
			var recipe []byte
			if strings.HasPrefix(name, "custom") {
				selector, source = "custom", "profiles/bun.json"
				recipe = binaryTLSRecipe(t, name == "custom-h2-variant")
				writeBinaryTLSFile(t, filepath.Join(dir, source), recipe)
			}
			extra := `  healthcheck {
    path = "/health"
    interval = "1s"
    timeout = "500ms"
    expected_body = "ready"
    send_authorization = true
  }
`
			path := filepath.Join(dir, "config.hcl")
			writeBinaryTLSFile(t, path, []byte(binaryTLSConfig(addr, binaryTLSProvider("local", fixture.URL(), "synthetic-provider", selector, source, extra))))
			env := binaryTLSChildEnv(t, trust, guard.URL(), nil)
			srv := startBinaryTLSServer(t, path, addr, t.TempDir(), env)
			health := fixture.observed.WaitCalls(t, "/v1/health", 1)[0]
			out, err := runBinaryTLSCLI(t, t.TempDir(), env, "models", "--config", path, "--provider", "local", "--upstream")
			if err != nil || !strings.Contains(out, "upstream-chat (configured as local/chat)") || !strings.Contains(out, "discovered-only (not in config)") || strings.Contains(out, "synthetic-provider") {
				t.Fatalf("binary upstream discovery: %v\n%s", err, out)
			}
			models := fixture.observed.Calls("/v1/models")
			if len(models) != 1 {
				t.Fatal("discovery changed operation-owned request count")
			}
			waitBinaryTLSCondition(t, "model discovery operation-owned TLS cleanup", func() bool { return fixture.observed.Closed(models[0].remote) })
			if status, body := postChat(t, srv, "synthetic-caller", `{"model":"local/chat","messages":[]}`); status != http.StatusOK {
				t.Fatalf("inference alongside health/discovery: %d %s", status, body)
			}
			inference := fixture.observed.Calls("/v1/chat/completions")[0]
			if health.method != http.MethodGet || models[0].method != http.MethodGet || inference.method != http.MethodPost {
				t.Fatal("health/discovery/inference changed HTTP operation")
			}
			for _, call := range []binaryTLSCall{health, models[0], inference} {
				if call.authorization != "Bearer synthetic-provider" || call.proxyAuthorization != "" || !strings.HasPrefix(call.userAgent, "aiproxy/") || call.version != tls.VersionTLS13 {
					t.Fatal("alternate outbound path changed auth/agent/TLS contract")
				}
				wantProto := 2
				if name == "custom-h1" {
					wantProto = 1
				}
				if call.protoMajor != wantProto {
					t.Fatal("alternate path did not follow negotiated profile ALPN")
				}
				requireBinaryTLSProfile(t, selector, recipe, fixture.observed.Hello(t, call.remote), &native)
			}
			if health.remote == inference.remote || health.remote == models[0].remote || inference.remote == models[0].remote {
				t.Fatal("health/discovery borrowed the inference transport generation")
			}
			if name == "custom-h1" {
				h2Recipe := binaryTLSRecipe(t, true)
				writeBinaryTLSFile(t, filepath.Join(dir, source), h2Recipe)
				reloadBinaryTLS(t, srv, true)
				waitBinaryTLSCondition(t, "reloaded custom health h2 probe", func() bool {
					calls := fixture.observed.Calls("/v1/health")
					return calls[len(calls)-1].protoMajor == 2
				})
				calls := fixture.observed.Calls("/v1/health")
				current := calls[len(calls)-1]
				requireBinaryTLSProfile(t, "custom", h2Recipe, fixture.observed.Hello(t, current.remote), nil)
				waitBinaryTLSCondition(t, "superseded health TLS connection", func() bool { return fixture.observed.Closed(health.remote) })
				writeBinaryTLSFile(t, filepath.Join(dir, source), []byte(`{"invalid-replacement":`))
				reloadBinaryTLS(t, srv, false)
				fixture.server.CloseClientConnections()
				before := len(fixture.observed.Calls("/v1/health"))
				calls = fixture.observed.WaitCalls(t, "/v1/health", before+1)
				cached := calls[len(calls)-1]
				if cached.remote == current.remote || cached.protoMajor != 2 {
					t.Fatal("failed reload did not retain cached custom health on a fresh connection")
				}
				requireBinaryTLSProfile(t, "custom", h2Recipe, fixture.observed.Hello(t, cached.remote), nil)
			}
			srv.Stop(t)
			for _, call := range fixture.observed.Calls("") {
				waitBinaryTLSCondition(t, "health/inference shutdown TLS cleanup", func() bool { return fixture.observed.Closed(call.remote) })
			}
		})
	}
	t.Run("models-whole-operation-body-timeout", func(t *testing.T) {
		fixture := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"id":"partial-model"}],`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
		dir := t.TempDir()
		recipe := binaryTLSRecipe(t, true)
		writeBinaryTLSFile(t, filepath.Join(dir, "h2-variant.json"), recipe)
		text := binaryTLSConfig("127.0.0.1:0", binaryTLSProvider("local", fixture.URL(), "synthetic-provider", "custom", "h2-variant.json", ""))
		text = strings.Replace(text, `upstream_header_timeout = "2s"`, `upstream_header_timeout = "150ms"`, 1)
		path := filepath.Join(dir, "config.hcl")
		writeBinaryTLSFile(t, path, []byte(text))
		start := time.Now()
		out, err := runBinaryTLSCLI(t, t.TempDir(), binaryTLSChildEnv(t, trust, guard.URL(), nil), "models", "--config", path, "--provider", "local", "--upstream")
		if err == nil || !strings.Contains(out, "context deadline exceeded") || !strings.Contains(out, "read upstream body") || strings.Contains(out, "partial-model") || time.Since(start) > 3*time.Second {
			t.Fatalf("profile discovery lost whole-operation timeout or printed partial results: %v\n%s", err, out)
		}
		calls := fixture.observed.Calls("/v1/models")
		if len(calls) != 1 || calls[0].protoMajor != 2 {
			t.Fatal("discovery timeout did not exercise actual custom h2 HTTP")
		}
		requireBinaryTLSProfile(t, "custom", recipe, fixture.observed.Hello(t, calls[0].remote), nil)
		waitBinaryTLSCondition(t, "timed-out discovery TLS cleanup", func() bool { return fixture.observed.Closed(calls[0].remote) })
	})
}
