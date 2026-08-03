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

	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64

	Duration time.Duration
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

type Aggregator struct {
	mu        sync.Mutex
	counts    map[summaryKey]aggregateEntry
	recent    *ringBuffer
	retention time.Duration
	now       func() time.Time
}

type aggregateEntry struct {
	count            int64
	lastSeen         time.Time
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
		counts:    make(map[summaryKey]aggregateEntry),
		recent:    newRingBuffer(defaultRecentN),
		retention: defaultRetention,
		now:       time.Now,
	}
}

func (a *Aggregator) Record(event Event) {
	a.mu.Lock()
	if a.counts == nil {
		a.counts = make(map[summaryKey]aggregateEntry)
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
	entry := a.counts[key]
	entry.count++
	entry.lastSeen = now
	entry.promptTokens += event.PromptTokens
	entry.completionTokens += event.CompletionTokens
	entry.totalTokens += event.TotalTokens
	a.counts[key] = entry
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
	out := make([]Summary, 0, len(a.counts))
	for key, entry := range a.counts {
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
	if a.retention <= 0 || a.counts == nil {
		return
	}
	cutoff := now.Add(-a.retention)
	for key, entry := range a.counts {
		if entry.lastSeen.Before(cutoff) {
			delete(a.counts, key)
		}
	}
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
		if s.StatusCode >= 500 || s.StatusCode == 0 {
			entry.Errors += s.Count
		} else if s.StatusCode >= 400 && s.StatusCode != 429 {
			entry.Errors += s.Count
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
