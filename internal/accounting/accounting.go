package accounting

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultRetention = 24 * time.Hour
	defaultRecentN   = 200
)

type Event struct {
	Timestamp  time.Time
	Tenant     string
	Client     string
	Model      string
	Operation  string
	StatusCode int

	Provider      string
	UpstreamModel string

	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64

	Duration time.Duration
}

func EventProvider(e Event) string {
	if e.Provider != "" {
		return e.Provider
	}
	if strings.HasPrefix(e.Model, "_") {
		return "aiproxy"
	}
	for i := 0; i < len(e.Model); i++ {
		if e.Model[i] == '/' {
			return e.Model[:i]
		}
	}
	return e.Model
}

func isErrorStatus(code int) bool {
	if code >= 500 || code == 0 {
		return true
	}
	return code >= 400 && code != 429
}

func (e Event) HasTokens() bool {
	return e.PromptTokens > 0 || e.CompletionTokens > 0 || e.TotalTokens > 0
}

type Recorder interface {
	Record(Event)
}

type Reader interface {
	Summaries() []Summary
}

type Snapshotter interface {
	Recent(int) []Event
}

type Summary struct {
	Tenant     string
	Client     string
	Model      string
	Operation  string
	StatusCode int
	Count      int64

	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

type ProviderSummary struct {
	Provider         string
	Requests         int64
	Errors           int64
	Throttled        int64
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

type UpstreamSummary struct {
	Tenant     string
	Client     string
	Provider   string
	Model      string
	Operation  string
	StatusCode int
	Count      int64

	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

type RecorderFunc func(Event)

func (f RecorderFunc) Record(event Event) {
	f(event)
}

type noopRecorder struct{}

func NewNoop() Recorder {
	return noopRecorder{}
}

func (noopRecorder) Record(Event) {}

func NewMulti(recorders ...Recorder) Recorder {
	filtered := make([]Recorder, 0, len(recorders))
	for _, recorder := range recorders {
		if recorder != nil {
			filtered = append(filtered, recorder)
		}
	}
	if len(filtered) == 0 {
		return noopRecorder{}
	}
	if len(filtered) == 1 {
		return filtered[0]
	}
	return multiRecorder(filtered)
}

type multiRecorder []Recorder

func (r multiRecorder) Record(event Event) {
	for _, recorder := range r {
		recorder.Record(event)
	}
}

type MemoryRecorder struct {
	mu     sync.Mutex
	events []Event
}

func (r *MemoryRecorder) Record(event Event) {
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *MemoryRecorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Event, len(r.events))
	copy(out, r.events)
	return out
}

func (r *MemoryRecorder) Recent(n int) []Event {
	if n <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		return nil
	}
	if n > len(r.events) {
		n = len(r.events)
	}
	out := make([]Event, n)
	copy(out, r.events[len(r.events)-n:])
	return out
}

type providerEntry struct {
	requests         int64
	errors           int64
	throttled        int64
	promptTokens     int64
	completionTokens int64
	totalTokens      int64
}

type upstreamKey struct {
	tenant     string
	client     string
	provider   string
	model      string
	operation  string
	statusCode int
}

type Aggregator struct {
	mu             sync.Mutex
	buckets        map[time.Time]map[summaryKey]aggregateEntry
	bucketOrder    []time.Time
	bucketDuration time.Duration
	recent         *ringBuffer
	retention      time.Duration
	now            func() time.Time
	providers      map[string]*providerEntry
	upstream       map[upstreamKey]*providerEntry
}

type aggregateEntry struct {
	count            int64
	promptTokens     int64
	completionTokens int64
	totalTokens      int64
}

type summaryKey struct {
	tenant     string
	client     string
	model      string
	operation  string
	statusCode int
}

func NewAggregator() *Aggregator {
	return &Aggregator{
		buckets:        make(map[time.Time]map[summaryKey]aggregateEntry),
		bucketDuration: time.Minute,
		recent:         newRingBuffer(defaultRecentN),
		retention:      defaultRetention,
		now:            time.Now,
	}
}

func (a *Aggregator) Record(event Event) {
	a.mu.Lock()
	if a.buckets == nil {
		a.buckets = make(map[time.Time]map[summaryKey]aggregateEntry)
	}
	if a.recent == nil {
		a.recent = newRingBuffer(defaultRecentN)
	}
	now := a.nowTime(event.Timestamp)
	a.pruneLocked(a.nowTime(time.Time{}))
	key := summaryKey{
		tenant:     event.Tenant,
		client:     event.Client,
		model:      event.Model,
		operation:  event.Operation,
		statusCode: event.StatusCode,
	}
	bucketStart := a.bucketStart(now)
	bucket := a.buckets[bucketStart]
	if bucket == nil {
		bucket = make(map[summaryKey]aggregateEntry)
		a.buckets[bucketStart] = bucket
		a.bucketOrder = append(a.bucketOrder, bucketStart)
	}
	entry := bucket[key]
	entry.count++
	entry.promptTokens += event.PromptTokens
	entry.completionTokens += event.CompletionTokens
	entry.totalTokens += event.TotalTokens
	bucket[key] = entry
	if a.providers == nil {
		a.providers = make(map[string]*providerEntry)
	}
	provName := EventProvider(event)
	prov := a.providers[provName]
	if prov == nil {
		prov = &providerEntry{}
		a.providers[provName] = prov
	}
	prov.requests++
	if isErrorStatus(event.StatusCode) {
		prov.errors++
	}
	if event.StatusCode == 429 {
		prov.throttled++
	}
	prov.promptTokens += event.PromptTokens
	prov.completionTokens += event.CompletionTokens
	prov.totalTokens += event.TotalTokens
	if event.Provider != "" && event.UpstreamModel != "" {
		if a.upstream == nil {
			a.upstream = make(map[upstreamKey]*providerEntry)
		}
		ukey := upstreamKey{
			tenant:     event.Tenant,
			client:     event.Client,
			provider:   event.Provider,
			model:      event.UpstreamModel,
			operation:  event.Operation,
			statusCode: event.StatusCode,
		}
		uent := a.upstream[ukey]
		if uent == nil {
			uent = &providerEntry{}
			a.upstream[ukey] = uent
		}
		uent.requests++
		if isErrorStatus(event.StatusCode) {
			uent.errors++
		}
		uent.promptTokens += event.PromptTokens
		uent.completionTokens += event.CompletionTokens
		uent.totalTokens += event.TotalTokens
	}
	ringEntry := event
	if ringEntry.Timestamp.IsZero() {
		ringEntry.Timestamp = now
	}
	a.recent.push(ringEntry)
	a.mu.Unlock()
}

func (a *Aggregator) Summaries() []Summary {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneLocked(a.nowTime(time.Time{}))
	counts := make(map[summaryKey]aggregateEntry)
	for _, bucket := range a.buckets {
		for key, entry := range bucket {
			total := counts[key]
			total.count += entry.count
			total.promptTokens += entry.promptTokens
			total.completionTokens += entry.completionTokens
			total.totalTokens += entry.totalTokens
			counts[key] = total
		}
	}
	out := make([]Summary, 0, len(counts))
	for key, entry := range counts {
		out = append(out, Summary{
			Tenant:           key.tenant,
			Client:           key.client,
			Model:            key.model,
			Operation:        key.operation,
			StatusCode:       key.statusCode,
			Count:            entry.count,
			PromptTokens:     entry.promptTokens,
			CompletionTokens: entry.completionTokens,
			TotalTokens:      entry.totalTokens,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tenant != out[j].Tenant {
			return out[i].Tenant < out[j].Tenant
		}
		if out[i].Client != out[j].Client {
			return out[i].Client < out[j].Client
		}
		if out[i].Model != out[j].Model {
			return out[i].Model < out[j].Model
		}
		if out[i].Operation != out[j].Operation {
			return out[i].Operation < out[j].Operation
		}
		return out[i].StatusCode < out[j].StatusCode
	})
	return out
}

func (a *Aggregator) Recent(n int) []Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.recent == nil {
		return nil
	}
	return a.recent.snapshot(n)
}

func (a *Aggregator) nowTime(ts time.Time) time.Time {
	if !ts.IsZero() {
		return ts
	}
	if a.now != nil {
		return a.now()
	}
	return time.Now()
}

func (a *Aggregator) pruneLocked(now time.Time) {
	if a.retention <= 0 || len(a.bucketOrder) == 0 {
		return
	}
	cutoff := now.Add(-a.retention)
	keep := 0
	for _, start := range a.bucketOrder {
		if start.Before(cutoff) {
			delete(a.buckets, start)
			continue
		}
		a.bucketOrder[keep] = start
		keep++
	}
	clear(a.bucketOrder[keep:])
	a.bucketOrder = a.bucketOrder[:keep]
	sort.Slice(a.bucketOrder, func(i, j int) bool { return a.bucketOrder[i].Before(a.bucketOrder[j]) })
}

func (a *Aggregator) bucketStart(ts time.Time) time.Time {
	return ts.Truncate(a.effectiveBucketDuration())
}

func (a *Aggregator) effectiveBucketDuration() time.Duration {
	if a.bucketDuration > 0 {
		if a.retention > 0 && a.retention < a.bucketDuration {
			return a.retention
		}
		return a.bucketDuration
	}
	if a.retention > 0 && a.retention < time.Minute {
		return a.retention
	}
	return time.Minute
}

func ByProvider(summaries []Summary) []ProviderSummary {
	byProvider := make(map[string]*ProviderSummary)
	providerFor := func(model string) string {
		if strings.HasPrefix(model, "_") {
			return "aiproxy"
		}
		for i := 0; i < len(model); i++ {
			if model[i] == '/' {
				return model[:i]
			}
		}
		return model
	}
	for _, s := range summaries {
		name := providerFor(s.Model)
		entry, ok := byProvider[name]
		if !ok {
			entry = &ProviderSummary{Provider: name}
			byProvider[name] = entry
		}
		entry.Requests += s.Count
		if isErrorStatus(s.StatusCode) {
			entry.Errors += s.Count
		}
		if s.StatusCode == 429 {
			entry.Throttled += s.Count
		}
		entry.PromptTokens += s.PromptTokens
		entry.CompletionTokens += s.CompletionTokens
		entry.TotalTokens += s.TotalTokens
	}
	out := make([]ProviderSummary, 0, len(byProvider))
	for _, entry := range byProvider {
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func (a *Aggregator) ProviderSummaries() []ProviderSummary {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ProviderSummary, 0, len(a.providers))
	for name, entry := range a.providers {
		out = append(out, ProviderSummary{
			Provider:         name,
			Requests:         entry.requests,
			Errors:           entry.errors,
			Throttled:        entry.throttled,
			PromptTokens:     entry.promptTokens,
			CompletionTokens: entry.completionTokens,
			TotalTokens:      entry.totalTokens,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func (a *Aggregator) UpstreamSummaries() []UpstreamSummary {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]UpstreamSummary, 0, len(a.upstream))
	for key, entry := range a.upstream {
		out = append(out, UpstreamSummary{
			Tenant:           key.tenant,
			Client:           key.client,
			Provider:         key.provider,
			Model:            key.model,
			Operation:        key.operation,
			StatusCode:       key.statusCode,
			Count:            entry.requests,
			PromptTokens:     entry.promptTokens,
			CompletionTokens: entry.completionTokens,
			TotalTokens:      entry.totalTokens,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Model < out[j].Model
	})
	return out
}

func FilterSummaries(summaries []Summary, tenant, client string) []Summary {
	if tenant == "" && client == "" {
		out := make([]Summary, len(summaries))
		copy(out, summaries)
		return out
	}
	out := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if tenant != "" {
			if summary.Tenant == tenant {
				out = append(out, summary)
			}
			continue
		}
		if summary.Client == client {
			out = append(out, summary)
		}
	}
	return out
}

type ringBuffer struct {
	items    []Event
	head     int
	filled   bool
	capacity int
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity <= 0 {
		capacity = defaultRecentN
	}
	return &ringBuffer{items: make([]Event, capacity), capacity: capacity}
}

func (r *ringBuffer) push(event Event) {
	r.items[r.head] = event
	r.head = (r.head + 1) % r.capacity
	if !r.filled && r.head == 0 {
		r.filled = true
	}
}

func (r *ringBuffer) snapshot(n int) []Event {
	if n <= 0 {
		return nil
	}
	available := r.head
	if r.filled {
		available = r.capacity
	}
	if available == 0 {
		return nil
	}
	if n > available {
		n = available
	}
	out := make([]Event, n)
	start := r.head - n
	if start < 0 {
		start += r.capacity
	}
	for i := 0; i < n; i++ {
		idx := (start + i) % r.capacity
		if idx < 0 {
			idx += r.capacity
		}
		out[i] = r.items[idx]
	}
	return out
}
