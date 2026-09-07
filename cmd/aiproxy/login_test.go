package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/configedit"
	"github.com/egose/aiproxy/internal/copilotlogin"
)

type loginTestServer struct {
	srv          *httptest.Server
	tokenHandler func(w http.ResponseWriter, r *http.Request)
	calls        *atomic.Int32
}

func newLoginTestServer(t *testing.T, tokenHandler func(w http.ResponseWriter, r *http.Request)) *loginTestServer {
	t.Helper()
	calls := &atomic.Int32{}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dev-123", "user_code": "WDXA-XXXX",
			"verification_uri": "https://github.com/login/device",
			"expires_in":       900, "interval": 0,
		})
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		tokenHandler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &loginTestServer{srv: srv, calls: calls}
}

func (s *loginTestServer) client() *copilotlogin.Client {
	c := copilotlogin.New()
	c.HTTP = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	c.DeviceCodeURL = s.srv.URL + "/login/device/code"
	c.TokenURL = s.srv.URL + "/login/oauth/access_token"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func writeJSON(w http.ResponseWriter, v string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(v))
}

func TestLoginCopilotSuccess(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_success","token_type":"bearer","scope":"read:user"}`)
	})
	var stdout, stderr bytes.Buffer
	opts := loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets, Scope: "read:user"}
	if err := runLoginCopilot(context.Background(), &stdout, &stderr, opts, ts.client); err != nil {
		t.Fatalf("runLoginCopilot(): %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "https://github.com/login/device") || !strings.Contains(out, "WDXA-XXXX") {
		t.Fatalf("missing URL/code in %q", out)
	}
	if strings.Contains(out, "gho_success") || strings.Contains(stderr.String(), "gho_success") {
		t.Fatalf("token leaked in output")
	}
	if !strings.Contains(out, "saved credential") || !strings.Contains(out, "SIGHUP") {
		t.Fatalf("missing persistence/activation report in %q", out)
	}
	sidecar, err := copilotlogin.SidecarPath(secrets, "main")
	if err != nil {
		t.Fatalf("SidecarPath(): %v", err)
	}
	cred, err := copilotlogin.Load(secrets, "main")
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cred.AccessToken != "gho_success" || cred.ClientID != "Ov23test" || cred.RefreshToken != "gho_success" {
		t.Fatalf("cred = %+v", cred)
	}
	info, err := os.Stat(sidecar)
	if err != nil {
		t.Fatalf("Stat(): %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o", got)
	}
	raw, _ := os.ReadFile(sidecar)
	if strings.Contains(string(raw), "gho_success") == false {
		t.Fatalf("sidecar missing token (expected stored, not printed)")
	}
}

func TestLoginCopilotPendingThenSuccess(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	var n atomic.Int32
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			writeJSON(w, `{"error":"authorization_pending"}`)
			return
		}
		writeJSON(w, `{"access_token":"gho_x","token_type":"bearer"}`)
	})
	var stdout, stderr bytes.Buffer
	opts := loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}
	if err := runLoginCopilot(context.Background(), &stdout, &stderr, opts, ts.client); err != nil {
		t.Fatalf("runLoginCopilot(): %v", err)
	}
	if n.Load() != 3 {
		t.Fatalf("calls = %d", n.Load())
	}
}

func TestLoginCopilotDenialPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	old, _ := copilotlogin.NewCredential("Ov23old", "gho_old", time.Now())
	if err := copilotlogin.Save(secrets, "main", old); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"error":"access_denied"}`)
	})
	var stdout, stderr bytes.Buffer
	opts := loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}
	err := runLoginCopilot(context.Background(), &stdout, &stderr, opts, ts.client)
	if err == nil {
		t.Fatalf("expected denial error")
	}
	if strings.Contains(stdout.String(), "gho_") || strings.Contains(stderr.String(), "gho_") {
		t.Fatalf("token leaked")
	}
	got, err := copilotlogin.Load(secrets, "main")
	if err != nil || got.AccessToken != "gho_old" {
		t.Fatalf("existing not preserved: %+v %v", got, err)
	}
}

func TestLoginCopilotExpiry(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"error":"expired_token"}`)
	})
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}, ts.client)
	if err == nil {
		t.Fatalf("expected expiry error")
	}
	if _, lerr := copilotlogin.Load(secrets, "main"); lerr == nil {
		t.Fatalf("should not persist on expiry")
	}
}

func TestLoginCopilotCancellation(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"error":"authorization_pending"}`)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(ctx, &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}, ts.client)
	if err == nil {
		t.Fatalf("expected cancel error")
	}
	if _, lerr := copilotlogin.Load(secrets, "main"); lerr == nil {
		t.Fatalf("should not persist on cancel")
	}
}

func TestLoginCopilotMalformedAndOversized(t *testing.T) {
	dir := t.TempDir()
	// token malformed
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"unexpected":1}`)
	})
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: filepath.Join(dir, "tok.json")}, ts.client)
	if err == nil {
		t.Fatalf("malformed token: expected error")
	}
	// oversized device-code response
	big := strings.Repeat("x", copilotlogin.MaxResponseBytes+10)
	mux := http.NewServeMux()
	mux.HandleFunc("/login/device/code", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"device_code":"`+big+`"}`)
	})
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_x"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := copilotlogin.New()
	c.DeviceCodeURL = srv.URL + "/login/device/code"
	c.TokenURL = srv.URL + "/login/oauth/access_token"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	var stdout2, stderr2 bytes.Buffer
	err = runLoginCopilot(context.Background(), &stdout2, &stderr2,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: filepath.Join(dir, "big.json")},
		func() *copilotlogin.Client { return c })
	if err == nil {
		t.Fatalf("oversized: expected error")
	}
}

func TestLoginCopilotRedirectRefusal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := copilotlogin.New()
	c.DeviceCodeURL = srv.URL + "/x"
	c.TokenURL = srv.URL + "/y"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: filepath.Join(dir, "keys.json")},
		func() *copilotlogin.Client { return c })
	if err == nil {
		t.Fatalf("expected redirect refusal")
	}
}

func TestLoginCopilotNetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := copilotlogin.New()
	c.DeviceCodeURL = url + "/a"
	c.TokenURL = url + "/b"
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: filepath.Join(dir, "keys.json")},
		func() *copilotlogin.Client { return c })
	if err == nil {
		t.Fatalf("expected network error")
	}
}

func TestLoginCopilotPersistenceFailure(t *testing.T) {
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_x","token_type":"bearer"}`)
	})
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: ""}, ts.client)
	if err == nil {
		t.Fatalf("expected persistence error")
	}
}

func TestLoginCopilotRequiresClientID(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	for _, opts := range []loginCopilotOptions{
		{Credential: "main", SecretsPath: filepath.Join(dir, "keys.json")},
		{ClientID: "has space", Credential: "main", SecretsPath: filepath.Join(dir, "keys.json")},
		{ClientID: "Ov23test", SecretsPath: filepath.Join(dir, "keys.json")},
		{ClientID: "Ov23test", Credential: "Bad Name", SecretsPath: filepath.Join(dir, "keys.json")},
	} {
		if err := runLoginCopilot(context.Background(), &stdout, &stderr, opts, nil); err == nil {
			t.Fatalf("opts %+v succeeded", opts)
		}
	}
}

func TestLoginCopilotUnsafeDestRejected(t *testing.T) {
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_x","token_type":"bearer"}`)
	})
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	sidecar, _ := copilotlogin.SidecarPath(secrets, "main")
	if err := os.MkdirAll(filepath.Dir(sidecar), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(dir, "target")
	_ = os.WriteFile(target, []byte("t"), 0o600)
	_ = os.Symlink(target, sidecar)
	var stdout, stderr bytes.Buffer
	err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}, ts.client)
	if err == nil {
		t.Fatalf("expected symlink refusal")
	}
}

func TestLoginCopilotPreservesFlatMap(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	flat := "{\n  \"openai\": \"sk-test\"\n}\n"
	if err := os.WriteFile(secrets, []byte(flat), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_x","token_type":"bearer"}`)
	})
	var stdout, stderr bytes.Buffer
	if err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}, ts.client); err != nil {
		t.Fatalf("runLoginCopilot(): %v", err)
	}
	data, _ := os.ReadFile(secrets)
	if string(data) != flat {
		t.Fatalf("flat map modified: %q", data)
	}
}

func TestLoginConcurrentIndependentUpdates(t *testing.T) {
	dir := t.TempDir()
	secrets := filepath.Join(dir, "keys.json")
	if err := configedit.WriteSecretsUpdate(configedit.SecretsUpdate{Path: secrets, Key: "seed", Value: "v"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_x","token_type":"bearer"}`)
	})
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := []string{"alpha", "beta", "gamma", "delta"}[i]
			var stdout, stderr bytes.Buffer
			if err := runLoginCopilot(context.Background(), &stdout, &stderr,
				loginCopilotOptions{ClientID: "Ov23test", Credential: name, SecretsPath: secrets}, ts.client); err != nil {
				errs <- err
			}
		}(i)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := configedit.WriteSecretsUpdate(configedit.SecretsUpdate{
				Path: secrets, Key: "flat-key", Value: "flat-value",
			}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent writer: %v", err)
	}
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		if _, err := copilotlogin.Load(secrets, name); err != nil {
			t.Fatalf("Load(%q): %v", name, err)
		}
	}
	flat, err := configedit.ReadSecretsFile(secrets)
	if err != nil {
		t.Fatalf("ReadSecretsFile(): %v", err)
	}
	if flat["seed"] != "v" || flat["flat-key"] != "flat-value" {
		t.Fatalf("flat keys lost: %v", flat)
	}
}

func TestLoginRequiresNoProviderConfig(t *testing.T) {
	dir := t.TempDir()
	badConfig := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(badConfig, []byte("this is not valid hcl {{{"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	secrets := filepath.Join(dir, "keys.json")
	ts := newLoginTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"access_token":"gho_x","token_type":"bearer"}`)
	})
	var stdout, stderr bytes.Buffer
	if err := runLoginCopilot(context.Background(), &stdout, &stderr,
		loginCopilotOptions{ClientID: "Ov23test", Credential: "main", SecretsPath: secrets}, ts.client); err != nil {
		t.Fatalf("login should not require valid provider config: %v", err)
	}
}

func TestLoginCommandRegistered(t *testing.T) {
	cmd := newRootCommand()
	var found bool
	for _, sub := range cmd.Commands() {
		if sub.Name() == "login" {
			found = true
			var subFound bool
			for _, s := range sub.Commands() {
				if s.Name() == "github-copilot" {
					subFound = true
					if s.Flags().Lookup("client-id") == nil || s.Flags().Lookup("credential") == nil {
						t.Fatalf("missing required flags")
					}
					if s.Flags().Lookup("client-secret") != nil {
						t.Fatalf("must not request client secret")
					}
				}
			}
			if !subFound {
				t.Fatalf("login missing github-copilot subcommand")
			}
		}
	}
	if !found {
		t.Fatalf("root missing login command")
	}
}

func TestValidateDoesNotRunLogin(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(configPath, []byte("listener \"http\" \"public\" { address = \":0\" }\nauth \"main\" { mode = \"none\" }\nprovider \"openai\" \"openai\" {\n  api_key = \"sk-test\"\n  model \"gpt-4o-mini\" {}\n}\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"validate", "--config", configPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "copilot-") {
			t.Fatalf("validate created login artifact %q", e.Name())
		}
	}
}
