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
	Truncated      RecentTruncation `json:",omitzero"`
	RecentSequence uint64           `json:",string,omitempty"`
	ProviderID     uint64           `json:",string,omitempty"`
	RequestID      string           `json:",omitempty"`
	PublicModel    string           `json:",omitempty"`
	Timestamp      time.Time
	Tenant         string
	Client         string
	Model          string
	Operation      string
	StatusCode     int

	Provider      string
	UpstreamModel string

	ReasoningEffort string `json:",omitempty"`

	PromptTokens        int64
	CompletionTokens    int64
	TotalTokens         int64
	CachedTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64

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
	return e.PromptTokens > 0 || e.CompletionTokens > 0 || e.TotalTokens > 0 || e.CachedTokens > 0 || e.CacheCreationTokens > 0 || e.CacheReadTokens > 0
}

type Recorder interface {
	Record(Event)
}

type Reader interface {
	Summaries() []Summary
	UpstreamSummaries() []UpstreamSummary
	BillingSummaries() ([]Summary, []UpstreamSummary)
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

	PromptTokens        int64
	CompletionTokens    int64
	TotalTokens         int64
	CachedTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

type ProviderSummary struct {
	ProviderID          uint64 `json:",string,omitempty"`
	Provider            string
	Requests            int64
	Errors              int64
	Throttled           int64
	PromptTokens        int64
	CompletionTokens    int64
	TotalTokens         int64
	CachedTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

type UpstreamSummary struct {
	PublicModel string `json:",omitempty"`
	Tenant      string
	Client      string
	Provider    string
	Model       string
	Operation   string
	StatusCode  int
	Count       int64

	PromptTokens        int64
	CompletionTokens    int64
	TotalTokens         int64
	CachedTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
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

func (r *MemoryRecorder) Summaries() []Summary {
	r.mu.Lock()
	defer r.mu.Unlock()
	byKey := map[summaryKey]*Summary{}
	var order []summaryKey
	for _, e := range r.events {
		key := summaryKey{tenant: e.Tenant, client: e.Client, model: e.Model, operation: e.Operation, statusCode: e.StatusCode}
		s, ok := byKey[key]
		if !ok {
			s = &Summary{Tenant: key.tenant, Client: key.client, Model: key.model, Operation: key.operation, StatusCode: key.statusCode}
			byKey[key] = s
			order = append(order, key)
		}
		s.Count++
		s.PromptTokens += e.PromptTokens
		s.CompletionTokens += e.CompletionTokens
		s.TotalTokens += e.TotalTokens
		s.CachedTokens += e.CachedTokens
		s.CacheCreationTokens += e.CacheCreationTokens
		s.CacheReadTokens += e.CacheReadTokens
	}
	out := make([]Summary, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	return out
}

func (r *MemoryRecorder) UpstreamSummaries() []UpstreamSummary {
	return nil
}

func (r *MemoryRecorder) BillingSummaries() ([]Summary, []UpstreamSummary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	counts := make(map[billingKey]aggregateEntry)
	for _, event := range r.events {
		key := eventBillingKey(event)
		counts[key] = counts[key].add(eventAggregate(event))
	}
	return summarizeBilling(counts)
}

type providerEntry struct {
	id                  uint64
	requests            int64
	errors              int64
	throttled           int64
	promptTokens        int64
	completionTokens    int64
	totalTokens         int64
	cachedTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
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
	buckets        map[time.Time]map[billingKey]aggregateEntry
	bucketOrder    []time.Time
	bucketDuration time.Duration
	recent         *ringBuffer
	retention      time.Duration
	now            func() time.Time
	providers      map[string]*providerEntry
	upstream       map[upstreamKey]*providerEntry
	rates          [rateSeconds + 1]rateBucket
}

type aggregateEntry struct {
	count               int64
	promptTokens        int64
	completionTokens    int64
	totalTokens         int64
	cachedTokens        int64
	cacheCreationTokens int64
	cacheReadTokens     int64
}

type summaryKey struct {
	tenant     string
	client     string
	model      string
	operation  string
	statusCode int
}

type billingKey struct {
	summaryKey
	provider      string
	upstreamModel string
}

func eventBillingKey(e Event) billingKey {
	return billingKey{
		summaryKey: summaryKey{tenant: e.Tenant, client: e.Client, model: e.Model, operation: e.Operation, statusCode: e.StatusCode},
		provider:   e.Provider, upstreamModel: e.UpstreamModel,
	}
}

func eventAggregate(e Event) aggregateEntry {
	return aggregateEntry{count: 1, promptTokens: e.PromptTokens, completionTokens: e.CompletionTokens,
		totalTokens: e.TotalTokens, cachedTokens: e.CachedTokens, cacheCreationTokens: e.CacheCreationTokens, cacheReadTokens: e.CacheReadTokens}
}

func (e aggregateEntry) add(other aggregateEntry) aggregateEntry {
	e.count += other.count
	e.promptTokens += other.promptTokens
	e.completionTokens += other.completionTokens
	e.totalTokens += other.totalTokens
	e.cachedTokens += other.cachedTokens
	e.cacheCreationTokens += other.cacheCreationTokens
	e.cacheReadTokens += other.cacheReadTokens
	return e
}

func NewAggregator() *Aggregator {
	return NewAggregatorWithClock(time.Now)
}

func NewAggregatorWithClock(now func() time.Time) *Aggregator {
	return &Aggregator{
		buckets:        make(map[time.Time]map[billingKey]aggregateEntry),
		bucketDuration: time.Minute,
		recent:         newRingBuffer(defaultRecentN),
		retention:      defaultRetention,
		now:            now,
	}
}

func (a *Aggregator) Record(event Event) {
	a.mu.Lock()
	if a.buckets == nil {
		a.buckets = make(map[time.Time]map[billingKey]aggregateEntry)
	}
	if a.recent == nil {
		a.recent = newRingBuffer(defaultRecentN)
	}
	clockNow := a.nowTime(time.Time{})
	now := event.Timestamp
	if now.IsZero() || now.After(clockNow) {
		now = clockNow
	}
	a.recordRateLocked(event, now, clockNow)
	key := eventBillingKey(event)
	bucketStart := a.bucketStart(now)
	bucket := a.buckets[bucketStart]
	if bucket == nil {
		bucket = make(map[billingKey]aggregateEntry)
		a.buckets[bucketStart] = bucket
		a.bucketOrder = append(a.bucketOrder, bucketStart)
	}
	bucket[key] = bucket[key].add(eventAggregate(event))
	a.pruneLocked(clockNow)
	if a.providers == nil {
		a.providers = make(map[string]*providerEntry)
	}
	provName := EventProvider(event)
	prov := a.providers[provName]
	if prov == nil {
		prov = &providerEntry{id: uint64(len(a.providers)) + 1}
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
	prov.cachedTokens += event.CachedTokens
	prov.cacheCreationTokens += event.CacheCreationTokens
	prov.cacheReadTokens += event.CacheReadTokens
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
		uent.cachedTokens += event.CachedTokens
		uent.cacheCreationTokens += event.CacheCreationTokens
		uent.cacheReadTokens += event.CacheReadTokens
	}
	ringEntry := event
	ringEntry.ProviderID = prov.id
	if ringEntry.Timestamp.IsZero() {
		ringEntry.Timestamp = now
	}
	a.recent.push(ringEntry)
	a.mu.Unlock()
}

func (a *Aggregator) Summaries() []Summary {
	summaries, _ := a.BillingSummaries()
	return summaries
}

func (a *Aggregator) BillingSummaries() ([]Summary, []UpstreamSummary) {
	snapshot := a.BillingSnapshot()
	return snapshot.Usage, snapshot.Upstream
}

func summarizeBilling(retained map[billingKey]aggregateEntry) ([]Summary, []UpstreamSummary) {
	counts := make(map[summaryKey]aggregateEntry)
	upstream := make([]UpstreamSummary, 0, len(retained))
	for key, entry := range retained {
		counts[key.summaryKey] = counts[key.summaryKey].add(entry)
		if key.provider == "" || key.upstreamModel == "" {
			continue
		}
		upstream = append(upstream, UpstreamSummary{
			PublicModel: key.model, Tenant: key.tenant, Client: key.client,
			Provider: key.provider, Model: key.upstreamModel, Operation: key.operation, StatusCode: key.statusCode,
			Count: entry.count, PromptTokens: entry.promptTokens, CompletionTokens: entry.completionTokens,
			TotalTokens: entry.totalTokens, CachedTokens: entry.cachedTokens,
			CacheCreationTokens: entry.cacheCreationTokens, CacheReadTokens: entry.cacheReadTokens,
		})
	}
	out := make([]Summary, 0, len(counts))
	for key, entry := range counts {
		out = append(out, Summary{
			Tenant:              key.tenant,
			Client:              key.client,
			Model:               key.model,
			Operation:           key.operation,
			StatusCode:          key.statusCode,
			Count:               entry.count,
			PromptTokens:        entry.promptTokens,
			CompletionTokens:    entry.completionTokens,
			TotalTokens:         entry.totalTokens,
			CachedTokens:        entry.cachedTokens,
			CacheCreationTokens: entry.cacheCreationTokens,
			CacheReadTokens:     entry.cacheReadTokens,
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
	sort.Slice(upstream, func(i, j int) bool {
		a, b := upstream[i], upstream[j]
		if a.Tenant != b.Tenant {
			return a.Tenant < b.Tenant
		}
		if a.Client != b.Client {
			return a.Client < b.Client
		}
		if a.PublicModel != b.PublicModel {
			return a.PublicModel < b.PublicModel
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		if a.StatusCode != b.StatusCode {
			return a.StatusCode < b.StatusCode
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Model < b.Model
	})
	return out, upstream
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
		entry.CachedTokens += s.CachedTokens
		entry.CacheCreationTokens += s.CacheCreationTokens
		entry.CacheReadTokens += s.CacheReadTokens
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
			ProviderID:          entry.id,
			Provider:            name,
			Requests:            entry.requests,
			Errors:              entry.errors,
			Throttled:           entry.throttled,
			PromptTokens:        entry.promptTokens,
			CompletionTokens:    entry.completionTokens,
			TotalTokens:         entry.totalTokens,
			CachedTokens:        entry.cachedTokens,
			CacheCreationTokens: entry.cacheCreationTokens,
			CacheReadTokens:     entry.cacheReadTokens,
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
			Tenant:              key.tenant,
			Client:              key.client,
			Provider:            key.provider,
			Model:               key.model,
			Operation:           key.operation,
			StatusCode:          key.statusCode,
			Count:               entry.requests,
			PromptTokens:        entry.promptTokens,
			CompletionTokens:    entry.completionTokens,
			TotalTokens:         entry.totalTokens,
			CachedTokens:        entry.cachedTokens,
			CacheCreationTokens: entry.cacheCreationTokens,
			CacheReadTokens:     entry.cacheReadTokens,
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
		if summary.Tenant == "" && summary.Client == client {
			out = append(out, summary)
		}
	}
	return out
}

type ringBuffer struct {
	sequence uint64
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
	r.sequence++
	event.RecentSequence = r.sequence
	r.items[r.head] = boundedRecent(event)
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
