package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/providerhealth"
)

type appDeferredReply struct {
	status int
	header http.Header
	body   []byte
}

func appDeferredControl(client *http.Client, base, method, path, token, generation string, body []byte) (appDeferredReply, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return appDeferredReply{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Aiproxy-Deferred-Version", "1")
	if generation != "" {
		request.Header.Set("X-Aiproxy-Deferred-Generation", generation)
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", deferred.MediaType)
	}
	response, err := client.Do(request)
	if err != nil {
		return appDeferredReply{}, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, deferred.MaxControlBodyBytes+1))
	if err != nil || len(data) > deferred.MaxControlBodyBytes {
		return appDeferredReply{}, fmt.Errorf("read finite deferred control: %w", err)
	}
	return appDeferredReply{status: response.StatusCode, header: response.Header.Clone(), body: data}, nil
}

func callAppDeferred(t *testing.T, server *httptest.Server, method, path, token, generation string, body []byte) appDeferredReply {
	t.Helper()
	reply, err := appDeferredControl(server.Client(), server.URL, method, path, token, generation, body)
	if err != nil {
		t.Fatal(err)
	}
	for name, expected := range map[string]string{
		"Content-Type": deferred.MediaType, "X-Aiproxy-Deferred-Version": "1",
		"Cache-Control": "private, no-store", "Pragma": "no-cache", "Vary": "Authorization",
	} {
		if reply.header.Get(name) != expected {
			t.Fatalf("%s: missing private marker/cache header %s; status=%d body=%s", path, name, reply.status, reply.body)
		}
	}
	return reply
}

func appDeferredCapability(t *testing.T, server *httptest.Server, token string) deferred.Capability {
	t.Helper()
	reply := callAppDeferred(t, server, "GET", deferred.Prefix+"/capabilities?model=local/m", token, "", nil)
	var capability deferred.Capability
	if reply.status != 200 || deferred.DecodeControl(reply.body, &capability) != nil {
		t.Fatalf("capability: %d %s", reply.status, reply.body)
	}
	return capability
}

func appDeferredSubmit(t *testing.T, capability deferred.Capability) (deferred.SubmitRequest, []byte) {
	t.Helper()
	key, err := deferred.NewAttemptKey()
	if err != nil {
		t.Fatal(err)
	}
	request := deferred.SubmitRequest{
		Envelope: deferred.NewEnvelope(), Generation: capability.Generation, SubmitToken: capability.SubmitToken,
		AttemptKey: key, Headers: http.Header{"Content-Type": {"application/json"}, "User-Agent": {"peer-inference"}},
		Body: []byte(`{"model":"local/m","messages":[]}`),
	}
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return request, wire
}

func TestDeferredAppHTTPJobsKeepExecutionLeaseAcrossFailedReloadRemovalAndReenable(t *testing.T) {
	gate, started := make(chan struct{}), make(chan string, 2)
	release := sync.OnceFunc(func() { close(gate) })
	var oldCalls, newCalls atomic.Int32
	body := []byte(" {\n\"choices\":[],\"source\":\"old\",\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n")
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldCalls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if !bytes.Contains(input, []byte(`"model":"m"`)) || r.Header.Get("X-Aiproxy-Deferred-Version") != "" {
			w.WriteHeader(400)
			return
		}
		started <- r.Header.Get("Authorization")
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(oldServer.Close)
	t.Cleanup(release)
	nextServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newCalls.Add(1)
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(nextServer.Close)
	oldDisk, nextDisk := t.TempDir(), t.TempDir()
	initial := deferredLifecycleConfig(oldServer.URL, "peer-token", "old-key", oldDisk, true, 4)
	path := writeConfigFile(t, initial)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	_ = a.health.Close()
	backend := &countingHealthBackend{}
	a.health = providerhealth.NewWithBackend(a.metrics, a.Config.ProviderHealth, backend)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.handler)
	t.Cleanup(server.Close)
	capability := appDeferredCapability(t, server, "peer-token")
	var receipts []deferred.Receipt
	var requests []deferred.SubmitRequest
	var wires [][]byte
	for range 2 {
		request, wire := appDeferredSubmit(t, capability)
		reply := callAppDeferred(t, server, "POST", deferred.Prefix+"/submit", "peer-token", "", wire)
		var receipt deferred.Receipt
		if reply.status != 202 || deferred.DecodeControl(reply.body, &receipt) != nil {
			t.Fatalf("submit: %d %s", reply.status, reply.body)
		}
		receipts, requests, wires = append(receipts, receipt), append(requests, request), append(wires, wire)
		select {
		case authorization := <-started:
			if authorization != "Bearer old-key" {
				t.Fatal("HTTP admission lost the selected upstream credential")
			}
		case <-time.After(time.Second):
			t.Fatal("accepted HTTP job was not scheduled independently of the acknowledgment")
		}
	}
	s, generation, oldOptions, oldConfig, oldResources := a.DeferredStore(), capability.Generation, a.DeferredStore().Options(), a.Config, a.runtimeResources
	a.resourceManager.mu.Lock()
	healthOwner, diskOwner := a.resourceManager.owners[a.health], a.resourceManager.owners[a.payloadLog]
	a.resourceManager.mu.Unlock()
	exceptions := filepath.Join(t.TempDir(), "exceptions.json")
	rewriteConfigFile(t, exceptions, "{broken json")
	disabled := deferredLifecycleConfig(nextServer.URL, "rotated-token", "new-key", nextDisk, false, 4) + "\nprovider_health { cooldown = \"60s\" }\n"
	invalid := disabled + fmt.Sprintf("\ningress_guardrails {\n enabled = true\n mode = \"audit\"\n exceptions_file = %q\n}\n", exceptions)
	rewriteConfigFile(t, path, invalid)
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "ingress guardrails exceptions") {
		t.Fatalf("late activation failure was not exercised: %v", err)
	}
	if a.Config != oldConfig || a.runtimeResources != oldResources || s.Options() != oldOptions || backend.closes.Load() != 0 {
		t.Fatal("failed reload changed accepted HTTP jobs, policy, or runtime resources")
	}
	rewriteConfigFile(t, path, disabled)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	draining := oldOptions
	draining.Enabled = false
	if s != a.DeferredStore() || s.Generation() != generation || s.Options() != draining || backend.closes.Load() != 0 {
		t.Fatal("root removal lost the app-owned store or closed accepted HTTP execution resources")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = healthOwner.Wait(ctx)
	cancel()
	if !errors.Is(err, httpapi.ErrResourcesIncomplete) {
		t.Fatalf("HTTP acknowledgment did not transfer its runtime lease: %v", err)
	}
	reply := callAppDeferred(t, server, "GET", deferred.Prefix+"/capabilities?model=local/m", "rotated-token", "", nil)
	if reply.status != 404 {
		t.Fatal("root removal admitted new discovery")
	}
	for index, receipt := range receipts {
		base := deferred.Prefix + "/jobs/" + receipt.ID
		reply := callAppDeferred(t, server, "GET", base, "peer-token", generation, nil)
		if reply.status != 401 || bytes.Contains(reply.body, []byte(receipt.ID)) {
			t.Fatal("draining controls continued authenticating the old token or disclosed job metadata")
		}
		reply = callAppDeferred(t, server, "GET", base, "rotated-token", generation, nil)
		var info deferred.JobInfo
		if reply.status != 200 || deferred.DecodeControl(reply.body, &info) != nil || info.State != deferred.StateRunning ||
			info.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS {
			t.Fatal("draining controls re-resolved execution or changed its frozen deadline")
		}
		reply = callAppDeferred(t, server, "POST", deferred.Prefix+"/submit", "rotated-token", "", wires[index])
		var replay deferred.Receipt
		if reply.status != 202 || deferred.DecodeControl(reply.body, &replay) != nil || replay.ID != receipt.ID || !replay.Replayed {
			t.Fatal("root removal reran admission instead of replaying accepted HTTP work")
		}
	}
	denied := strings.Replace(disabled, `allowed_models = ["local/m"]`, `allowed_models = ["other/m"]`, 1)
	rewriteConfigFile(t, path, denied)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	base := deferred.Prefix + "/jobs/" + receipts[0].ID
	for _, request := range []struct{ method, path string }{
		{"GET", base}, {"GET", base + "/read?cursor=0"}, {"GET", base + "/result"}, {"DELETE", base},
	} {
		reply := callAppDeferred(t, server, request.method, request.path, "rotated-token", generation, nil)
		if reply.status != 403 || bytes.Contains(reply.body, []byte(receipts[0].ID)) {
			t.Fatal("current model denial disclosed or deleted draining HTTP work")
		}
	}
	reply = callAppDeferred(t, server, "POST", deferred.Prefix+"/submit", "rotated-token", "", wires[0])
	if reply.status != 403 {
		t.Fatal("protected HTTP replay bypassed current model authorization")
	}
	rewriteConfigFile(t, path, disabled)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	beforeConfig, beforeResources := a.Config, a.runtimeResources
	narrow := deferredLifecycleConfig(nextServer.URL, "rotated-token", "new-key", nextDisk, true, 1) + "\nprovider_health { cooldown = \"60s\" }\n"
	rewriteConfigFile(t, path, narrow)
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "live reservations") {
		t.Fatalf("HTTP jobs did not participate in final reenable capacity validation: %v", err)
	}
	if a.Config != beforeConfig || a.runtimeResources != beforeResources || s.Options() != draining {
		t.Fatal("failed reenable partially published policy/resources")
	}
	enabled := deferredLifecycleConfig(nextServer.URL, "rotated-token", "new-key", nextDisk, true, 4) + "\nprovider_health { cooldown = \"60s\" }\n"
	enabled = strings.Replace(enabled, `execution_timeout = "10s"`, `execution_timeout = "1s"`, 1)
	rewriteConfigFile(t, path, enabled)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.DeferredStore() != s || s.Generation() != generation || !s.Options().Enabled || s.Options().ExecutionTimeout != time.Second {
		t.Fatal("safe reenable did not reuse live job ownership")
	}
	release()
	awaitResourceCondition(t, "HTTP job finalization after reload", func() bool { return s.Stats().FinalizationCredits == 0 })
	for _, receipt := range receipts {
		base := deferred.Prefix + "/jobs/" + receipt.ID
		reply := callAppDeferred(t, server, "GET", base+"/result", "rotated-token", generation, nil)
		var manifest deferred.ResultManifest
		if reply.status != 200 || deferred.DecodeControl(reply.body, &manifest) != nil || manifest.State != deferred.StateCompleted ||
			manifest.Outcome.Usage.TotalTokens != 5 || manifest.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS {
			t.Fatal("HTTP execution crossed reload resources or lost exact known usage")
		}
		reply = callAppDeferred(t, server, "GET", base+"/read?cursor=0", "rotated-token", generation, nil)
		var frame deferred.ReadFrame
		if reply.status != 200 || deferred.DecodeControl(reply.body, &frame) != nil || !frame.Done || !bytes.Equal(frame.Data, body) {
			t.Fatal("reloaded HTTP result retrieval changed retained unary bytes")
		}
		reply = callAppDeferred(t, server, "DELETE", base, "rotated-token", generation, nil)
		if reply.status != 204 || len(reply.body) != 0 {
			t.Fatal("reloaded delete did not release retained HTTP work")
		}
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := healthOwner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := diskOwner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	events, logs := a.usage.Recent(10), readPayloadLines(t, oldDisk)
	if oldCalls.Load() != 2 || newCalls.Load() != 0 || backend.closes.Load() != 1 || len(events) != 2 || len(logs) != 2 ||
		s.Stats().Jobs != 0 || s.Stats().WorkBytes != 0 || s.Stats().InputBytes != 0 {
		t.Fatal("HTTP leases, controls, or finalization duplicated logical inference or leaked resources")
	}
	for _, event := range events {
		if event.StatusCode != 200 || event.TotalTokens != 5 || event.Provider != "local" || event.Operation != "chat_completions" {
			t.Fatal("control acknowledgments were accounted as successful inference")
		}
	}
	for _, entry := range logs {
		encoded, _ := json.Marshal(entry)
		for _, secret := range []string{"old-key", "peer-token", "rotated-token", deferred.Protocol, requests[0].SubmitToken, requests[1].SubmitToken} {
			if bytes.Contains(encoded, []byte(secret)) {
				t.Fatal("HTTP completion log retained a private envelope or credential")
			}
		}
	}
}

type appDeferredAdmissionLimiter struct {
	entered chan struct{}
	gate    <-chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (l *appDeferredAdmissionLimiter) Allow(string) (bool, time.Duration) {
	l.calls.Add(1)
	l.once.Do(func() { close(l.entered) })
	<-l.gate
	return true, 0
}

func TestDeferredAppHTTPPreparationSerializesCommitWithPolicyActivation(t *testing.T) {
	for _, mode := range []string{"remove", "changed", "failed"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			t.Cleanup(upstream.Close)
			base := deferredLifecycleConfig(upstream.URL, "peer-token", "old-key", "", true, 4)
			base = strings.Replace(base, `control_timeout = "100ms"`, `control_timeout = "1s"`, 1)
			path := writeConfigFile(t, base)
			a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.Close() })
			server := httptest.NewServer(a.handler)
			t.Cleanup(server.Close)
			capability := appDeferredCapability(t, server, "peer-token")
			_, wire := appDeferredSubmit(t, capability)
			gate := make(chan struct{})
			release := sync.OnceFunc(func() { close(gate) })
			t.Cleanup(release)
			limiter := &appDeferredAdmissionLimiter{entered: make(chan struct{}), gate: gate}
			deps := a.handler.SnapshotDependencies()
			deps.RateLimiter = limiter
			a.handler.UpdateDependencies(deps)
			a.rateLimiter = limiter
			oldResources, oldConfig, oldOptions := a.runtimeResources, a.Config, a.DeferredStore().Options()
			type result struct {
				reply appDeferredReply
				err   error
			}
			submission := make(chan result, 1)
			go func() {
				reply, err := appDeferredControl(server.Client(), server.URL, "POST", deferred.Prefix+"/submit", "peer-token", "", wire)
				submission <- result{reply, err}
			}()
			select {
			case <-limiter.entered:
			case <-time.After(time.Second):
				t.Fatal("HTTP request did not enter bounded shared preparation")
			}
			candidate := strings.Replace(base, `api_key = "old-key"`, `api_key = "new-key"`, 1)
			switch mode {
			case "remove":
				candidate = deferredLifecycleConfig(upstream.URL, "peer-token", "new-key", "", false, 4)
			case "failed":
				candidate += "\ninvalid {"
			}
			rewriteConfigFile(t, path, candidate)
			err = a.Reload()
			if mode == "failed" {
				if err == nil || a.Config != oldConfig || a.runtimeResources != oldResources || a.DeferredStore().Options() != oldOptions {
					t.Fatal("failed activation changed the in-progress HTTP preparation generation")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			release()
			var submitted result
			select {
			case submitted = <-submission:
			case <-time.After(2 * time.Second):
				t.Fatal("HTTP preparation did not finish after activation")
			}
			if submitted.err != nil {
				t.Fatal(submitted.err)
			}
			replay := callAppDeferred(t, server, "POST", deferred.Prefix+"/submit", "peer-token", "", wire)
			if mode == "failed" {
				var receipt, repeated deferred.Receipt
				if submitted.reply.status != 202 || replay.status != 202 ||
					deferred.DecodeControl(submitted.reply.body, &receipt) != nil || deferred.DecodeControl(replay.body, &repeated) != nil ||
					receipt.ID != repeated.ID || !repeated.Replayed {
					t.Fatal("failed reload lost the old HTTP preparation or reran accepted admission")
				}
				awaitResourceCondition(t, "accepted old-generation HTTP completion", func() bool { return a.DeferredStore().Stats().FinalizationCredits == 0 })
				if calls.Load() != 1 || len(a.usage.Recent(10)) != 1 {
					t.Fatal("failed reload/replay duplicated old-generation execution")
				}
			} else {
				expectedStatus, expectedType := 503, "deferred_saturated"
				if mode == "remove" {
					expectedStatus, expectedType = 404, "deferred_disabled"
				}
				var failure deferred.ControlError
				if submitted.reply.status != expectedStatus || deferred.DecodeControl(submitted.reply.body, &failure) != nil ||
					failure.Error.Type != expectedType || failure.Acceptance != deferred.AcceptanceNotAccepted ||
					replay.status != submitted.reply.status || !bytes.Equal(replay.body, submitted.reply.body) ||
					calls.Load() != 0 || len(a.usage.Recent(10)) != 0 || a.DeferredStore().Stats().Jobs != 0 {
					t.Fatalf("HTTP preparation committed across activation or lost cached rejection: %d %s", submitted.reply.status, submitted.reply.body)
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				if err := oldResources.Wait(ctx); err != nil {
					cancel()
					t.Fatal("rejected HTTP preparation retained its old runtime lease")
				}
				cancel()
			}
			if limiter.calls.Load() != 1 || a.DeferredStore().Stats().Admissions != 0 {
				t.Fatal("HTTP replay reran rate admission or retained ingress ownership")
			}
		})
	}
}
