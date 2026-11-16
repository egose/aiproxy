package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

type deferredAllocationTransport func(*http.Request) (*http.Response, error)

func (f deferredAllocationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type deferredAllocationBody struct {
	io.Reader
	reads  atomic.Int32
	closes atomic.Int32
}

func (b *deferredAllocationBody) Read(data []byte) (int, error) {
	b.reads.Add(1)
	return b.Reader.Read(data)
}

func (b *deferredAllocationBody) Close() error {
	b.closes.Add(1)
	return nil
}

func deferredAllocationRequest(deps httpapi.Dependencies, writer *deferred.Writer, client *http.Client) provider.Request {
	input := writer.Input()
	target, _ := deps.Catalog.Provider("local")
	model := target.ModelByName["m"]
	return provider.Request{
		Operation: provider.OpChatCompletions, ProviderType: target.Type, ModelProtocol: model.Protocol,
		PublicModel: input.PublicModel, BaseURL: target.BaseURL, APIKey: target.APIKey,
		CopilotToken: target.CopilotToken, UpstreamModel: model.UpstreamName, Version: "test",
		UserAgent: target.UserAgent, ForwardHeaders: target.ForwardHeaders, Body: input.Body,
		Inbound: &http.Request{Header: http.Header(input.Headers)}, Client: client,
		ExecutionLimits: &provider.ExecutionLimits{
			RequestBytes: input.Limits.RequestBytes, ResponseBytes: input.Limits.ResultBytes,
			Work: writer.WorkBudget(),
		},
	}
}

func TestDeferredAdapterAllocationFailureIsSealedOnceAcrossReloadAndFinalization(t *testing.T) {
	path := writeConfigFile(t, deferredLifecycleConfig("http://127.0.0.1:1", "peer-token", "old-key", "", true, 4))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	deps, release, ok := a.handler.SnapshotDependenciesWithLease()
	if !ok {
		t.Fatal("runtime unavailable")
	}
	a.resourceManager.mu.Lock()
	oldOwner := a.resourceManager.owners[deps.Health]
	a.resourceManager.mu.Unlock()
	ingress, preparation, _ := lifecyclePrepare(t, deps)
	workEntered, workGate := make(chan struct{}), make(chan struct{})
	finalEntered, finalGate := make(chan struct{}), make(chan struct{})
	unblockWork := sync.OnceFunc(func() { close(workGate) })
	unblockFinal := sync.OnceFunc(func() { close(finalGate) })
	defer unblockWork()
	defer unblockFinal()
	var calls, finalizations atomic.Int32
	source := &deferredAllocationBody{Reader: strings.NewReader(strings.Repeat(" ", 65537))}
	client := &http.Client{Transport: deferredAllocationTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		_ = req.Body.Close()
		if req.Header.Get("Authorization") != "Bearer old-key" || req.URL.Host != "127.0.0.1:1" {
			t.Error("accepted allocation work crossed endpoint/credential generations")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: source, ContentLength: 65537}, nil
	})}
	receipt, err := preparation.Commit(func(ctx context.Context, writer *deferred.Writer) deferred.Outcome {
		_ = writer.SetFinalizer(func(context.Context) error {
			finalizations.Add(1)
			close(finalEntered)
			<-finalGate
			return nil
		})
		close(workEntered)
		<-workGate
		_, err := provider.New().Do(ctx, deferredAllocationRequest(deps, writer, client))
		if !errors.Is(err, deferred.ErrBudgetExceeded) {
			t.Error("provider did not enforce the admitted result allowance", err)
			return deferred.Outcome{Kind: deferred.OutcomeUpstreamError}
		}
		return deferred.Outcome{Kind: deferred.OutcomeLimitExceeded, Error: &deferred.ErrorDetail{Type: "deferred_limit_exceeded", Message: "provider allocation bound"}}
	}, release)
	ingress.Release()
	if err != nil {
		release()
		t.Fatal(err)
	}
	<-workEntered
	rewriteConfigFile(t, path, deferredLifecycleConfig("http://127.0.0.1:2", "rotated-token", "new-key", "", true, 4)+"\nprovider_health { cooldown = \"60s\" }\n")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	unblockWork()
	<-finalEntered
	s := a.DeferredStore()
	control, err := s.AcquireControl(deferred.Owner{Kind: "static", ID: "peer"}, false)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Status(control, receipt.Generation, receipt.ID)
	control.Release()
	if err != nil || info.State != deferred.StateFailed || info.Outcome.Kind != deferred.OutcomeLimitExceeded ||
		source.reads.Load() != 0 || source.closes.Load() != 1 || calls.Load() != 1 || finalizations.Load() != 1 {
		t.Fatalf("allocation failure was not bounded/sealed/closed once: %+v %v", info, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := oldOwner.Wait(ctx); !errors.Is(err, httpapi.ErrResourcesIncomplete) {
		t.Fatal("blocked finalization lost the admitted runtime lease", err)
	}
	if stats := s.Stats(); stats.ReservedBytes > s.Options().MaxMemoryBytes || stats.FinalizationCredits != 1 || stats.WorkBytes == 0 {
		t.Fatalf("finalization ownership escaped admission reservations: %+v", stats)
	}
	unblockFinal()
	awaitResourceCondition(t, "allocation completion ownership", func() bool { return s.Stats().FinalizationCredits == 0 })
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := oldOwner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	lifecycleDelete(t, s, receipt)
	if s.Stats().Jobs != 0 || s.Stats().WorkBytes != 0 || finalizations.Load() != 1 {
		t.Fatal("releasing failed output leaked or duplicated finalization")
	}
}

func TestDeferredLateHTTPBodyOwnershipPinsShutdownRuntimeAndFinalization(t *testing.T) {
	path := writeConfigFile(t, deferredLifecycleConfig("http://127.0.0.1:1", "peer-token", "key", "", true, 4))
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
		t.Fatal("runtime unavailable")
	}
	ingress, preparation, _ := lifecyclePrepare(t, deps)
	sentBody := make(chan io.ReadCloser, 1)
	finalEntered := make(chan struct{})
	client := &http.Client{Transport: deferredAllocationTransport(func(req *http.Request) (*http.Response, error) {
		sentBody <- req.Body
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("{}")), ContentLength: 2}, nil
	})}
	receipt, err := preparation.Commit(func(ctx context.Context, writer *deferred.Writer) deferred.Outcome {
		result, err := provider.New().Do(ctx, deferredAllocationRequest(deps, writer, client))
		if err != nil {
			t.Error(err)
			return deferred.Outcome{Kind: deferred.OutcomeUpstreamError}
		}
		_ = writer.SetFinalizer(func(context.Context) error {
			result.OnClose()
			close(finalEntered)
			return nil
		})
		_ = writer.SetResponse(deferred.ResponseHead{StatusCode: result.StatusCode, Headers: result.Header})
		_, _ = writer.Write(result.Body)
		return deferred.Outcome{Kind: deferred.OutcomeResponse}
	}, release)
	ingress.Release()
	if err != nil {
		release()
		t.Fatal(err)
	}
	body := <-sentBody
	defer body.Close()
	<-finalEntered
	s := a.DeferredStore()
	awaitResourceCondition(t, "closed but undrained provider budget", func() bool {
		release, err := preparation.WorkBudget().Reserve(1)
		if err == nil {
			release()
		}
		return errors.Is(err, deferred.ErrBudgetClosed)
	})
	lifecycleDelete(t, s, receipt)
	if stats := s.Stats(); stats.Jobs != 1 || stats.WorkBytes == 0 || stats.FinalizationCredits != 1 || stats.OutputBytes != 0 {
		t.Fatalf("late body cleanup falsely freed ownership: %+v", stats)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = a.CloseContext(ctx)
	cancel()
	if !errors.Is(err, deferred.ErrCleanupIncomplete) || !errors.Is(err, httpapi.ErrResourcesIncomplete) || backend.closes.Load() != 0 {
		t.Fatal("shutdown released resources still owned by an HTTP body", err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	awaitResourceCondition(t, "late HTTP body cleanup", func() bool { return s.Stats().Jobs == 0 })
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := a.resourceManager.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if stats := s.Stats(); stats.ReservedBytes != 0 || stats.FinalizationCredits != 0 {
		t.Fatalf("shutdown retained completed provider ownership: %+v", stats)
	}
	assertHealthClosedOnce(t, backend)
}
