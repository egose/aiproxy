package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/store"
)

func TestCopilotDatabaseRuntimeAndViews(t *testing.T) {
	a, path, _, request := catalogRuntimeFixture(t)
	observedAuth := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		observedAuth <- r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" || body.Model != "base-model" || r.Header.Get("X-Initiator") != "user" {
			t.Error("incorrect Copilot dispatch or credential isolation")
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[]}\n\ndata: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[]}`)
		}
	}))
	t.Cleanup(upstream.Close)
	ctx := context.Background()
	var child store.DBProvider
	for _, name := range []string{"base", "child"} {
		cred, err := copilotlogin.NewCredential("client-id", name+"-private-token", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		enc, err := store.EncryptCopilotCredential(cred, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		row := store.DBProvider{Name: name, Type: "github-copilot", Enabled: true, BaseURL: upstream.URL, CopilotCredentialEncrypted: enc}
		models := []store.DBProviderModel{{Name: "m", UpstreamName: "base-model", Capabilities: []string{"chat"}}}
		if name == "child" {
			row.Extends, row.BaseURL = "base", ""
			models = nil
		}
		if err := a.adminStore.CreateProviderAggregate(ctx, &row, models); err != nil {
			t.Fatal(err)
		}
		if name == "child" {
			child = row
		}
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	check := func(app *App) {
		t.Helper()
		for _, name := range []string{"base", "child"} {
			for _, stream := range []bool{false, true} {
				r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[],"stream":%v}`, name+"/m", stream)))
				r.Header.Set("Authorization", "Bearer static-fixture")
				w := httptest.NewRecorder()
				app.Server.Handler.ServeHTTP(w, r)
				if w.Code != 200 || (stream && !strings.Contains(w.Body.String(), "[DONE]")) {
					t.Fatalf("dispatch = %d %s", w.Code, w.Body.String())
				}
				if <-observedAuth != "Bearer "+name+"-private-token" {
					t.Fatal("derived provider inherited the base credential")
				}
			}
		}
	}
	check(a)
	for _, endpoint := range []string{"/_internal/admin/providers", "/_internal/admin/providers/child"} {
		w := request("GET", endpoint, "")
		if w.Code != 200 {
			t.Fatalf("view = %d %s", w.Code, w.Body.String())
		}
		for _, secret := range []string{"base-private-token", "child-private-token", base64.StdEncoding.EncodeToString(child.CopilotCredentialEncrypted), "copilot_credential_encrypted", "access_token", "refresh_token"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("admin view exposed credential")
			}
		}
	}
	if w := request("PUT", "/_internal/admin/providers/child", `{"display_name":"Updated"}`); w.Code != 200 {
		t.Fatalf("unrelated edit = %d %s", w.Code, w.Body.String())
	}
	stored, err := a.adminStore.GetProvider(ctx, "child")
	if err != nil || !bytes.Equal(stored.CopilotCredentialEncrypted, child.CopilotCredentialEncrypted) {
		t.Fatal("unrelated edit changed credential bytes")
	}
	stored.CopilotCredentialEncrypted = []byte("private-corrupt-blob")
	if err := a.adminStore.UpdateProviderAggregate(ctx, &stored, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("corrupt reload must fail safely")
	}
	check(a)
	stored.CopilotCredentialEncrypted = child.CopilotCredentialEncrypted
	if err := a.adminStore.UpdateProviderAggregate(ctx, &stored, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	check(restarted)
}
