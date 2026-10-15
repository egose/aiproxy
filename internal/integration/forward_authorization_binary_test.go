//go:build integration && linux

package integration

import (
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"testing"
)

func TestBinaryForwardAuthorizationReload(t *testing.T) {
	upstream := newUpstreamStub(t,
		upstreamResponse{Body: `{"choices":[]}`},
		upstreamResponse{ContentType: "text/event-stream", Body: "data: [DONE]\n\n"},
		upstreamResponse{Body: `{"choices":[]}`},
	)
	addr := freeAddr(t)
	configuration := func(credentials string) string {
		return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" { mode = "none" }
provider "openai-compatible" "test" {
  base_url = %q
  %s
  model "m" {}
}
alias "a" {
  algorithm = "round_robin"
  target {
    provider = "test"
    model = "m"
  }
}`, addr, upstream.URL(), credentials)
	}
	path := writeConfig(t, configuration(`api_key = "configured"`))
	srv := startBinaryServer(t, path, addr)
	if status, body := postChat(t, srv, "client-token", `{"model":"test/m","messages":[]}`); status != http.StatusOK {
		t.Fatalf("initial status=%d body=%s", status, body)
	}
	if calls := upstream.WaitCalls(t, 1); calls[0].Authorization != "Bearer configured" {
		t.Fatal("default mode did not use the configured key")
	}
	writeConfigPath(t, path, configuration(`forward_authorization = true`))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	srv.WaitLogContains(t, "config reloaded")
	if status, body := postChat(t, srv, "", `{"model":"alias/a","messages":[]}`); status != http.StatusBadRequest {
		t.Fatalf("missing authorization status=%d body=%s", status, body)
	}
	if len(upstream.Calls()) != 1 {
		t.Fatal("missing authorization reached upstream")
	}
	if status, body := postChat(t, srv, "client-token", `{"model":"alias/a","messages":[],"stream":true}`); status != http.StatusOK || !strings.Contains(body, "[DONE]") {
		t.Fatalf("forwarded streaming status=%d body=%s", status, body)
	}
	if calls := upstream.WaitCalls(t, 2); calls[1].Authorization != "Bearer client-token" {
		t.Fatal("reloaded alias did not forward client authorization")
	}
	writeConfigPath(t, path, configuration("forward_authorization = true\napi_key = \"conflict\""))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	srv.WaitLogContains(t, "config reload failed")
	if status, body := postChat(t, srv, "client-token", `{"model":"test/m","messages":[]}`); status != http.StatusOK {
		t.Fatalf("after rejected reload status=%d body=%s", status, body)
	}
	if calls := upstream.WaitCalls(t, 3); calls[2].Authorization != "Bearer client-token" {
		t.Fatal("rejected reload did not preserve forwarding")
	}
}
