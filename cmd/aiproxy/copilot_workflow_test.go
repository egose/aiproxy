package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/copilotlogin"
)

func copilotWorkflowLocalTransport(t *testing.T) {
	t.Helper()
	previous := http.DefaultTransport
	transport := previous.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			t.Errorf("workflow refused unexpected non-loopback dial: %s", address)
			return nil, fmt.Errorf("non-loopback dial refused")
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, address)
	}
	http.DefaultTransport = transport
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		http.DefaultTransport = previous
	})
}

func TestCopilotMockProvisioningAndRecovery(t *testing.T) {
	copilotWorkflowLocalTransport(t)
	for _, failure := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			t.Setenv("AIPROXY_CONFIG", "")
			secrets := filepath.Join(dir, "keys.json")
			configPath := filepath.Join(dir, "config.hcl")
			seed := []byte("listener \"http\" \"public\" { address = \"127.0.0.1:0\" }\nauth \"main\" { mode = \"none\" }\n")
			if err := os.WriteFile(configPath, seed, 0o600); err != nil {
				t.Fatal(err)
			}
			var issuerCalls, upstreamCalls atomic.Int32
			var loginReply atomic.Value
			loginReply.Store(`{"access_token":"synthetic-original","token_type":"bearer"}`)
			issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				issuerCalls.Add(1)
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || r.Form.Get("client_id") != "synthetic-client" || r.Form.Get("client_secret") != "" || r.Header.Get("Authorization") != "" {
					t.Error("unexpected issuer method, identity or credential")
				}
				switch r.URL.Path {
				case "/login/device/code":
					if r.Form.Get("scope") != "read:user" {
						t.Error("unexpected scope")
					}
					writeJSON(w, `{"device_code":"synthetic-device","user_code":"TEST-CODE","verification_uri":"https://example.invalid/device","expires_in":900,"interval":1}`)
				case "/login/oauth/access_token":
					if r.Form.Get("device_code") != "synthetic-device" || r.Form.Get("grant_type") != copilotlogin.DeviceGrantType {
						t.Error("unexpected device grant")
					}
					writeJSON(w, loginReply.Load().(string))
				default:
					t.Error("unexpected issuer path")
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(issuer.Close)
			login := func(ctx context.Context, cancelPoll bool) error {
				t.Helper()
				var out, stderr bytes.Buffer
				err := runLoginCopilot(ctx, &out, &stderr, loginCopilotOptions{
					ClientID: "synthetic-client", Credential: "workflow", SecretsPath: secrets,
				}, func() *copilotlogin.Client {
					c := copilotlogin.New()
					c.DeviceCodeURL = issuer.URL + "/login/device/code"
					c.TokenURL = issuer.URL + "/login/oauth/access_token"
					now := time.Now()
					c.Now = func() time.Time { return now }
					c.Sleep = func(ctx context.Context, d time.Duration) error {
						if cancelPoll {
							return context.Canceled
						}
						now = now.Add(d)
						return ctx.Err()
					}
					return c
				})
				text := out.String() + stderr.String() + fmt.Sprint(err)
				for _, secret := range []string{"synthetic-original", "synthetic-rotated", "synthetic-device"} {
					if strings.Contains(text, secret) {
						t.Fatal("login exposed synthetic secret")
					}
				}
				if err == nil && (!strings.Contains(text, "saved credential") || !strings.Contains(text, "SIGHUP")) {
					t.Fatal("missing persistence/activation guidance")
				}
				return err
			}
			if err := login(context.Background(), false); err != nil {
				t.Fatal(err)
			}
			if raw, err := os.ReadFile(configPath); err != nil || !bytes.Equal(raw, seed) {
				t.Fatal("login modified config")
			}
			sidecar, err := copilotlogin.SidecarPath(secrets, "workflow")
			if err != nil {
				t.Fatal(err)
			}
			readSidecar := func() []byte {
				t.Helper()
				raw, err := os.ReadFile(sidecar)
				if err != nil {
					t.Fatal(err)
				}
				return raw
			}
			original := readSidecar()
			if info, err := os.Stat(sidecar); err != nil || info.Mode().Perm() != 0o600 {
				t.Fatal("sidecar must be 0600")
			}
			var revoked atomic.Bool
			var expectedToken atomic.Value
			expectedToken.Store("synthetic-original")
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				auth := r.Header.Get("Authorization")
				if auth != "Bearer "+expectedToken.Load().(string) {
					t.Error("consumer did not use expected persisted/runtime credential")
				}
				if !strings.HasPrefix(r.Header.Get("User-Agent"), "aiproxy/") || r.Header.Get("X-GitHub-Api-Version") != copilotlogin.APIVersion {
					t.Error("missing shared auth headers")
				}
				for _, header := range []string{"Cookie", "X-Api-Key", "X-Interaction-Id"} {
					if r.Header.Get(header) != "" {
						t.Errorf("forwarded forbidden %s", header)
					}
				}
				if revoked.Load() && auth == "Bearer synthetic-original" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(failure)
					_, _ = io.WriteString(w, `{"error":"synthetic-revoked"}`)
					return
				}
				switch r.Method + " " + r.URL.Path {
				case "GET /models":
					writeJSON(w, `{"data":[{"id":"upstream-chat"},{"id":"discovered-only"}]}`)
				case "POST /chat/completions":
					var body struct {
						Model  string `json:"model"`
						Stream bool   `json:"stream"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "upstream-chat" {
						t.Error("incorrect upstream model rewrite")
					}
					if r.Header.Get("Openai-Intent") != "conversation-edits" || r.Header.Get("X-Initiator") != "user" || r.Header.Get("Copilot-Vision-Request") != "" {
						t.Error("incorrect derived chat metadata")
					}
					if body.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"workflow-ok\"}}]}\n\ndata: [DONE]\n\n")
					} else {
						writeJSON(w, `{"choices":[{"message":{"role":"assistant","content":"workflow-ok"}}]}`)
					}
				default:
					t.Error("unexpected inference origin/path")
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(upstream.Close)
			command := func(args ...string) (string, error) {
				t.Helper()
				out, stderr, err := executeRootCommand("", args...)
				if strings.Contains(out+stderr+fmt.Sprint(err), "synthetic-original") || strings.Contains(out+stderr+fmt.Sprint(err), "synthetic-rotated") {
					t.Fatal("command exposed synthetic token")
				}
				return out + stderr, err
			}
			if out, err := command("configure", "provider", "--config", configPath, "--non-interactive", "--type", "github-copilot", "--name", "copilot", "--credential", "workflow", "--credential-path", secrets, "--base-url", upstream.URL, "--model", "public-chat", "--model-upstream", "public-chat=upstream-chat"); err != nil {
				t.Fatalf("configure: %v %s", err, out)
			}
			if out, err := command("validate", "--config", configPath); err != nil {
				t.Fatalf("validate: %v %s", err, out)
			}
			if issuerCalls.Load() != 2 || upstreamCalls.Load() != 0 || !bytes.Equal(original, readSidecar()) {
				t.Fatal("configure/validate performed networking or changed credential")
			}
			listing := func(wantStatus int) {
				t.Helper()
				before := upstreamCalls.Load()
				out, err := command("models", "--config", configPath, "--provider", "copilot", "--upstream")
				if wantStatus == 200 {
					if err != nil || !strings.Contains(out, "upstream-chat (configured as copilot/public-chat)") || !strings.Contains(out, "discovered-only (not in config)") {
						t.Fatalf("discovery: %v %s", err, out)
					}
				} else if err == nil || !strings.Contains(out, fmt.Sprint(wantStatus)) || !strings.Contains(out, "login") {
					t.Fatalf("missing listing re-login hint: %v %s", err, out)
				}
				if upstreamCalls.Load() != before+1 {
					t.Fatal("listing silently retried")
				}
			}
			listing(200)
			a, err := app.Build(context.Background(), app.BuildOptions{ConfigPath: configPath, Version: "workflow-test"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := a.Close(); err != nil {
					t.Error(err)
				}
			})
			server := httptest.NewServer(a.Server.Handler)
			t.Cleanup(server.Close)
			client := &http.Client{Timeout: 5 * time.Second}
			chat := func(wantStatus int) {
				t.Helper()
				for _, stream := range []bool{false, true} {
					before := upstreamCalls.Load()
					req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"copilot/public-chat","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream)))
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("Content-Type", "application/json")
					for _, h := range []string{"Authorization", "Cookie", "X-Api-Key", "X-Interaction-Id", "X-Initiator", "Copilot-Vision-Request"} {
						req.Header.Set(h, "synthetic-caller")
					}
					resp, err := client.Do(req)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil || resp.StatusCode != wantStatus {
						t.Fatalf("chat stream=%t: %d %s %v", stream, resp.StatusCode, raw, err)
					}
					if wantStatus == 200 {
						if !strings.Contains(string(raw), "workflow-ok") || (stream && (!strings.Contains(string(raw), "[DONE]") || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream"))) {
							t.Fatal("incomplete JSON/SSE response")
						}
					} else if string(raw) != `{"error":"synthetic-revoked"}` {
						t.Fatal("auth error not passed through verbatim")
					}
					if upstreamCalls.Load() != before+1 {
						t.Fatal("inference silently retried")
					}
				}
			}
			chat(200)
			before := upstreamCalls.Load()
			resp, err := client.Get(server.URL + "/v1/models")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != 200 || !strings.Contains(string(raw), "copilot/public-chat") || strings.Contains(string(raw), "discovered-only") || upstreamCalls.Load() != before {
				t.Fatal("discovery changed proxy-owned static inventory")
			}
			revoked.Store(true)
			chat(failure)
			listing(failure)
			if issuerCalls.Load() != 2 {
				t.Fatal("failure implicitly re-authorized")
			}
			loginReply.Store(`{"error":"access_denied"}`)
			if err := login(context.Background(), false); !errors.Is(err, copilotlogin.ErrAccessDenied) {
				t.Fatalf("denied re-login: %v", err)
			}
			if err := login(context.Background(), true); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled re-login: %v", err)
			}
			if !bytes.Equal(original, readSidecar()) {
				t.Fatal("failed/cancelled re-login replaced sidecar")
			}
			chat(failure)
			loginReply.Store(`{"access_token":"synthetic-rotated","token_type":"bearer"}`)
			backup := filepath.Join(dir, "original-sidecar.json")
			if err := os.Rename(sidecar, backup); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(backup, sidecar); err != nil {
				t.Fatal(err)
			}
			if err := login(context.Background(), false); err == nil || !strings.Contains(err.Error(), "persist credential") {
				t.Fatalf("expected actual persistence refusal after authorization: %v", err)
			}
			if !bytes.Equal(original, readSidecar()) {
				t.Fatal("failed persistence overwrote existing credential")
			}
			if err := os.Remove(sidecar); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(backup, sidecar); err != nil {
				t.Fatal(err)
			}
			chat(failure)
			if err := login(context.Background(), false); err != nil {
				t.Fatal(err)
			}
			cred, err := copilotlogin.Load(secrets, "workflow")
			if err != nil || cred.AccessToken != "synthetic-rotated" || cred.ClientID != "synthetic-client" {
				t.Fatal("re-login did not persist new credential")
			}
			rotated := readSidecar()
			chat(failure)
			expectedToken.Store("synthetic-rotated")
			listing(200)
			expectedToken.Store("synthetic-original")
			if err := os.WriteFile(sidecar, []byte(`{invalid`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := a.Reload(); err == nil {
				t.Fatal("invalid candidate activated")
			}
			chat(failure)
			if err := os.WriteFile(sidecar, rotated, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := a.Reload(); err != nil {
				t.Fatal(err)
			}
			expectedToken.Store("synthetic-rotated")
			chat(200)
			listing(200)
			if issuerCalls.Load() != 9 {
				t.Fatalf("unexpected implicit OAuth calls: %d", issuerCalls.Load())
			}
		})
	}
}
