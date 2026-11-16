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

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func deferredLifecycleConfig(baseURL, token, key, disk string, enabled bool, jobs int) string {
	receiver, root := "", ""
	if enabled {
		receiver = "deferred_receiver {}"
		root = fmt.Sprintf(`
deferred {
  max_jobs = %d
  max_running = %d
  max_admissions = 2
  max_entries = 16
  max_memory_bytes = 33554432
  max_request_bytes = 2048
  max_result_bytes = 65536
  max_work_bytes = 4194304
  per_owner_jobs = %d
  per_owner_running = %d
  max_control_inflight = 8
  execution_timeout = "10s"
  retention = "10s"
  control_timeout = "100ms"
  finalization_timeout = "100ms"
}
`, jobs, min(jobs, 2), jobs, min(jobs, 2))
	}
	logging := "logging { access_log = false }"
	if disk != "" {
		logging = fmt.Sprintf("logging {\n access_log = false\n payload_log {\n enabled = true\n dir = %q\n }\n}", disk)
	}
	return fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" {
  mode = "bearer_static"
  client "peer" {
    token = %q
    allowed_models = ["local/m"]
  }
}
provider "openai-compatible" "local" {
  base_url = %q
  api_key = %q
  %s
  model "m" {}
}
%s
%s
`, token, baseURL, key, receiver, root, logging)
}

func lifecyclePrepare(t *testing.T, deps httpapi.Dependencies) (*deferred.Ingress, *deferred.Preparation, deferred.SubmitRequest) {
	t.Helper()
	s := deps.DeferredStore
	owner := deferred.Owner{Kind: "static", ID: "peer"}
	control, err := s.AcquireControl(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := s.Capability(control, "local/m", deferred.ReceiverLimits{})
	control.Release()
	if err != nil {
		t.Fatal(err)
	}
	key, err := deferred.NewAttemptKey()
	if err != nil {
		t.Fatal(err)
	}
	request := deferred.SubmitRequest{
		Envelope: deferred.NewEnvelope(), Generation: capability.Generation, SubmitToken: capability.SubmitToken,
		AttemptKey: key, Headers: http.Header{"Content-Type": {"application/json"}},
		Body: []byte(`{"model":"local/m","messages":[]}`),
	}
	wire, _ := json.Marshal(request)
	ingress, err := s.AcquireIngress(owner, int64(len(wire)))
	if err != nil {
		t.Fatal(err)
	}
	preparation, replay, err := s.Begin(ingress, "local/m", request)
	if err != nil || preparation == nil || replay != nil {
		ingress.Release()
		t.Fatalf("prepare job: %v", err)
	}
	return ingress, preparation, request
}

func lifecycleReplay(t *testing.T, s *deferred.Store, request deferred.SubmitRequest) (*deferred.Receipt, error) {
	t.Helper()
	wire, _ := json.Marshal(request)
	ingress, err := s.AcquireIngress(deferred.Owner{Kind: "static", ID: "peer"}, int64(len(wire)))
	if err != nil {
		return nil, err
	}
	defer ingress.Release()
	preparation, receipt, err := s.Begin(ingress, "local/m", request)
	if preparation != nil {
		preparation.Abort()
		t.Fatal("protected replay entered the admission pipeline")
	}
	return receipt, err
}

func lifecycleDelete(t *testing.T, s *deferred.Store, receipt deferred.Receipt) {
	t.Helper()
	control, err := s.AcquireControl(deferred.Owner{Kind: "static", ID: "peer"}, true)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Release()
	if err := s.Delete(control, receipt.Generation, receipt.ID); err != nil {
		t.Fatal(err)
	}
}

func lifecycleResponse(_ context.Context, w *deferred.Writer) deferred.Outcome {
	_ = w.SetResponse(deferred.ResponseHead{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}})
	_, _ = w.Write([]byte("{}"))
	return deferred.Outcome{Kind: deferred.OutcomeResponse}
}

func TestDeferredLazyStoreGenerationSurvivesReloadRemovalAndReenableValidation(t *testing.T) {
	path := writeConfigFile(t, deferredLifecycleConfig("http://127.0.0.1:1", "first-token", "first-key", "", false, 4))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if a.DeferredStore() != nil {
		t.Fatal("absent initial root allocated an enabled store")
	}
	rewriteConfigFile(t, path, deferredLifecycleConfig("http://127.0.0.1:1", "first-token", "first-key", "", true, 4))
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	s, generation := a.DeferredStore(), a.DeferredStore().Generation()
	before := s.Options()
	var receipts []deferred.Receipt
	var requests []deferred.SubmitRequest
	gate := make(chan struct{})
	releaseGate := sync.OnceFunc(func() { close(gate) })
	defer releaseGate()
	for range 2 {
		deps, release, ok := a.handler.SnapshotDependenciesWithLease()
		if !ok {
			t.Fatal("runtime snapshot was unavailable")
		}
		i, p, request := lifecyclePrepare(t, deps)
		receipt, err := p.Commit(func(ctx context.Context, w *deferred.Writer) deferred.Outcome {
			select {
			case <-gate:
				return lifecycleResponse(ctx, w)
			case <-ctx.Done():
				return deferred.Outcome{Kind: deferred.OutcomeCancelled}
			}
		}, release)
		i.Release()
		if err != nil {
			release()
			t.Fatal(err)
		}
		receipts, requests = append(receipts, receipt), append(requests, request)
	}
	rewriteConfigFile(t, path, deferredLifecycleConfig("http://127.0.0.1:2", "rotated-token", "rotated-key", "", false, 4))
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.DeferredStore() != s || s.Generation() != generation {
		t.Fatal("root removal replaced process store/generation")
	}
	drain := before
	drain.Enabled = false
	if s.Options() != drain {
		t.Fatal("root removal discarded last control/resource policy")
	}
	current := a.handler.SnapshotDependencies()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer first-token")
	if _, err := current.Auth.Authenticate(req); err == nil {
		t.Fatal("draining store continued authenticating a rotated token")
	}
	req.Header.Set("Authorization", "Bearer rotated-token")
	principal, err := current.Auth.Authenticate(req)
	if err != nil || !current.Authorizer.Allow(principal, "local/m") {
		t.Fatal("draining controls cannot use current admitted-model authorization")
	}
	for index, request := range requests {
		replay, err := lifecycleReplay(t, s, request)
		if err != nil || replay.ID != receipts[index].ID || !replay.Replayed {
			t.Fatalf("root removal rejected accepted replay: %v", err)
		}
	}
	control, _ := s.AcquireControl(deferred.Owner{Kind: "static", ID: "peer"}, false)
	_, err = s.Capability(control, "local/m", deferred.ReceiverLimits{})
	control.Release()
	var failure *deferred.ProtocolError
	if !errors.As(err, &failure) || failure.Wire.Error.Type != "deferred_disabled" {
		t.Fatal("disabled store issued new capabilities")
	}
	oldConfig, oldResources := a.Config, current.RuntimeResources
	rewriteConfigFile(t, path, deferredLifecycleConfig("http://127.0.0.1:2", "rotated-token", "rotated-key", "", true, 1))
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "live reservations") {
		t.Fatalf("reenable capacity reduction was not rejected: %v", err)
	}
	if a.Config != oldConfig || a.handler.SnapshotDependencies().RuntimeResources != oldResources || s.Options() != drain {
		t.Fatal("failed reenable partially published dependencies or policy")
	}
	releaseGate()
	awaitResourceCondition(t, "deferred source completion", func() bool { return s.Stats().FinalizationCredits == 0 })
	for _, receipt := range receipts {
		lifecycleDelete(t, s, receipt)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.DeferredStore() != s || s.Generation() != generation || !s.Options().Enabled {
		t.Fatal("reenable did not reuse the draining generation")
	}
}

func TestDeferredFailedLateReloadLeavesPolicyJobsAndRuntimeUntouched(t *testing.T) {
	exceptions := filepath.Join(t.TempDir(), "exceptions.json")
	base := deferredLifecycleConfig("http://127.0.0.1:1", "peer-token", "old-key", "", true, 4)
	withGuardrails := base + fmt.Sprintf("\ningress_guardrails {\n enabled = true\n mode = \"audit\"\n exceptions_file = %q\n}\n", exceptions)
	path := writeConfigFile(t, withGuardrails)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	deps, release, ok := a.handler.SnapshotDependenciesWithLease()
	if !ok {
		t.Fatal("runtime unavailable")
	}
	i, p, request := lifecyclePrepare(t, deps)
	gate := make(chan struct{})
	continueJob := sync.OnceFunc(func() { close(gate) })
	defer continueJob()
	receipt, err := p.Commit(func(ctx context.Context, w *deferred.Writer) deferred.Outcome {
		select {
		case <-gate:
			return lifecycleResponse(ctx, w)
		case <-ctx.Done():
			return deferred.Outcome{Kind: deferred.OutcomeCancelled}
		}
	}, release)
	i.Release()
	if err != nil {
		release()
		t.Fatal(err)
	}
	s, oldConfig, oldOptions, oldResources := a.DeferredStore(), a.Config, a.DeferredStore().Options(), deps.RuntimeResources
	rewriteConfigFile(t, exceptions, "{broken json")
	candidate := strings.Replace(withGuardrails, `execution_timeout = "10s"`, `execution_timeout = "1s"`, 1)
	candidate += "\nprovider_health { cooldown = \"60s\" }\n"
	rewriteConfigFile(t, path, candidate)
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "ingress guardrails exceptions") {
		t.Fatalf("late reload error = %v", err)
	}
	if a.Config != oldConfig || a.DeferredStore() != s || s.Options() != oldOptions || a.handler.SnapshotDependencies().RuntimeResources != oldResources {
		t.Fatal("failed late candidate changed job policy/resources")
	}
	replay, err := lifecycleReplay(t, s, request)
	if err != nil || replay.ID != receipt.ID {
		t.Fatal("failed reload erased accepted job evidence")
	}
	continueJob()
	awaitResourceCondition(t, "old job finalization", func() bool { return s.Stats().FinalizationCredits == 0 })
}

func TestDeferredCommitRechecksRootRemovalAfterPreparation(t *testing.T) {
	path := writeConfigFile(t, deferredLifecycleConfig("http://127.0.0.1:1", "peer-token", "key", "", true, 4))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	deps, release, _ := a.handler.SnapshotDependenciesWithLease()
	defer release()
	i, p, request := lifecyclePrepare(t, deps)
	defer i.Release()
	rewriteConfigFile(t, path, deferredLifecycleConfig("http://127.0.0.1:1", "peer-token", "key", "", false, 4))
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	_, err = p.Commit(func(ctx context.Context, w *deferred.Writer) deferred.Outcome {
		calls.Add(1)
		return lifecycleResponse(ctx, w)
	}, nil)
	var failure *deferred.ProtocolError
	if !errors.As(err, &failure) || failure.Wire.Error.Type != "deferred_disabled" || calls.Load() != 0 {
		t.Fatal("preparation committed through disabled new-admission policy")
	}
	i.Release()
	_, err = lifecycleReplay(t, a.DeferredStore(), request)
	if !errors.As(err, &failure) || failure.Wire.Error.Type != "deferred_disabled" {
		t.Fatal("commit rejection was not protected for replay")
	}
}

type lifecycleSource struct {
	io.ReadCloser
	closed atomic.Bool
}

func (s *lifecycleSource) Close() error {
	s.closed.Store(true)
	return s.ReadCloser.Close()
}

func TestDeferredOldCatalogTransportSharedHealthAndPayloadLeaseThroughFinalization(t *testing.T) {
	var oldCalls, newCalls atomic.Int32
	var oldKey atomic.Value
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldCalls.Add(1)
		oldKey.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[],"source":"old"}`)
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[],"source":"new"}`)
	}))
	defer newServer.Close()
	oldDisk, nextDisk := t.TempDir(), t.TempDir()
	path := writeConfigFile(t, deferredLifecycleConfig(oldServer.URL, "peer-token", "old-key", oldDisk, true, 4))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	_ = a.health.Close()
	backend := &countingHealthBackend{}
	a.health = providerhealth.NewWithBackend(a.metrics, a.Config.ProviderHealth, backend)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	deps, release, ok := a.handler.SnapshotDependenciesWithLease()
	if !ok {
		t.Fatal("old runtime unavailable")
	}
	a.resourceManager.mu.Lock()
	oldHealthOwner, oldDiskOwner := a.resourceManager.owners[a.health], a.resourceManager.owners[a.payloadLog]
	a.resourceManager.mu.Unlock()
	i, p, _ := lifecyclePrepare(t, deps)
	workEntered, workGate := make(chan struct{}), make(chan struct{})
	finalEntered, finalGate := make(chan struct{}), make(chan struct{})
	continueWork := sync.OnceFunc(func() { close(workGate) })
	continueFinal := sync.OnceFunc(func() { close(finalGate) })
	defer continueWork()
	defer continueFinal()
	var completions atomic.Int32
	receipt, err := p.Commit(func(ctx context.Context, w *deferred.Writer) deferred.Outcome {
		var source *lifecycleSource
		_ = w.SetFinalizer(func(context.Context) error {
			completions.Add(1)
			if source == nil || !source.closed.Load() || backend.closes.Load() != 0 {
				t.Error("completion lost source/resource ordering")
			}
			err := deps.PayloadLog.Record(payloadlog.Entry{RequestID: "deferred-once", Status: 200})
			close(finalEntered)
			<-finalGate
			return err
		})
		close(workEntered)
		select {
		case <-workGate:
		case <-ctx.Done():
			return deferred.Outcome{Kind: deferred.OutcomeCancelled}
		}
		target, _ := deps.Catalog.Provider("local")
		request, _ := http.NewRequestWithContext(ctx, http.MethodPost, target.BaseURL+"/chat/completions", bytes.NewReader(w.Input().Body))
		request.Header.Set("Authorization", "Bearer "+target.APIKey)
		response, err := deps.ClientForProvider(target).Do(request)
		if err != nil {
			t.Error(err)
			return deferred.Outcome{Kind: deferred.OutcomeUpstreamError}
		}
		source = &lifecycleSource{ReadCloser: response.Body}
		_ = w.SetSource(source)
		body, err := io.ReadAll(io.LimitReader(source, 65537))
		if err != nil {
			t.Error(err)
			return deferred.Outcome{Kind: deferred.OutcomeUpstreamError}
		}
		_ = w.SetResponse(deferred.ResponseHead{StatusCode: response.StatusCode, Headers: response.Header})
		_, _ = w.Write(body)
		return deferred.Outcome{Kind: deferred.OutcomeResponse}
	}, release)
	i.Release()
	if err != nil {
		release()
		t.Fatal(err)
	}
	<-workEntered
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.health != deps.Health || a.payloadRecorder == nil {
		t.Fatal("unchanged generation failed to share health/payload owners")
	}
	rewriteConfigFile(t, path, deferredLifecycleConfig(newServer.URL, "peer-token", "new-key", nextDisk, true, 4)+"\nprovider_health { cooldown = \"60s\" }\n")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	if a.health == deps.Health || backend.closes.Load() != 0 {
		t.Fatal("changed generation closed shared resources still held by old job")
	}
	continueWork()
	<-finalEntered
	if oldCalls.Load() != 1 || newCalls.Load() != 0 || oldKey.Load() != "Bearer old-key" {
		t.Fatal("admitted job crossed catalog, credential, endpoint, or transport generation")
	}
	s := a.DeferredStore()
	if s.Stats().WorkBytes == 0 || s.Stats().FinalizationCredits != 1 || backend.closes.Load() != 0 {
		t.Fatal("terminal publication prematurely freed work/runtime completion lease")
	}
	for range 3 {
		control, err := s.AcquireControl(deferred.Owner{Kind: "static", ID: "peer"}, false)
		if err != nil {
			t.Fatal(err)
		}
		info, err := s.Status(control, receipt.Generation, receipt.ID)
		control.Release()
		if err != nil || info.State != deferred.StateCompleted {
			t.Fatalf("terminal result is not visible before bounded finalization: %v", err)
		}
	}
	lifecycleDelete(t, s, receipt)
	if backend.closes.Load() != 0 || s.Stats().WorkBytes == 0 || completions.Load() != 1 {
		t.Fatal("polls/delete duplicated completion or released blocked callback ownership")
	}
	continueFinal()
	awaitResourceCondition(t, "job runtime lease release", func() bool { return s.Stats().Jobs == 0 && s.Stats().WorkBytes == 0 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := oldHealthOwner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := oldDiskOwner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	assertHealthClosedOnce(t, backend)
	if len(readPayloadLines(t, oldDisk)) != 1 || completions.Load() != 1 {
		t.Fatal("payload completion was not exactly once through the old sink")
	}
	if _, err := p.WorkBudget().Reserve(1); !errors.Is(err, deferred.ErrBudgetClosed) {
		t.Fatal("terminal store retained an open runtime work budget")
	}
}

func TestDeferredAppShutdownReportsLeasedResourcesInsteadOfPrematureClose(t *testing.T) {
	path := writeConfigFile(t, deferredLifecycleConfig("http://127.0.0.1:1", "peer-token", "key", "", true, 4))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	_ = a.health.Close()
	backend := &countingHealthBackend{}
	a.health = providerhealth.NewWithBackend(a.metrics, config.ProviderHealth{}, backend)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	_, release, ok := a.handler.SnapshotDependenciesWithLease()
	if !ok {
		t.Fatal("runtime snapshot unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = a.CloseContext(ctx)
	cancel()
	if !errors.Is(err, httpapi.ErrResourcesIncomplete) || backend.closes.Load() != 0 {
		t.Fatalf("shutdown pretended live resources were closed: %v", err)
	}
	if _, _, ok := a.handler.SnapshotDependenciesWithLease(); ok {
		t.Fatal("shutdown admitted another runtime owner")
	}
	if err := a.Reload(); err == nil {
		t.Fatal("shutdown activated a new generation")
	}
	release()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.resourceManager.wait(ctx); err != nil {
		t.Fatal(err)
	}
	assertHealthClosedOnce(t, backend)
	if a.DeferredStore().Stats().ReservedBytes != 0 {
		t.Fatal("shutdown leaked deferred fixed capacity")
	}
}

func TestRuntimeResourceRetirementUsesBoundedWorkerAndReportsBlockedClose(t *testing.T) {
	manager := newRuntimeResourceManager()
	entered, gate := make(chan struct{}), make(chan struct{})
	var closes atomic.Int32
	first, _, err := manager.prepare([]ownedRuntimeResource{{key: "first", close: func() error {
		closes.Add(1)
		close(entered)
		<-gate
		return nil
	}}})
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := manager.prepare([]ownedRuntimeResource{{key: "second", close: func() error { closes.Add(1); return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	first.Retire()
	<-entered
	second.Retire()
	manager.beginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = manager.wait(ctx)
	cancel()
	if !errors.Is(err, httpapi.ErrResourcesIncomplete) || closes.Load() != 1 {
		t.Fatal("blocked retirement spawned an unbounded replacement worker")
	}
	close(gate)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.wait(ctx); err != nil || closes.Load() != 2 {
		t.Fatalf("bounded retirement did not drain: %v", err)
	}
}
