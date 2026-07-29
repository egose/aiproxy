package accounting

import (
	"sort"
	"sync"
	"time"
)

type Event struct {
	Timestamp  time.Time
	Tenant     string
	Client     string
	Model      string
	Operation  string
	StatusCode int
}

type Recorder interface {
	Record(Event)
}

type Reader interface {
	Summaries() []Summary
}

type Summary struct {
	Tenant     string
	Client     string
	Model      string
	Operation  string
	StatusCode int
	Count      int64
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

type Aggregator struct {
	mu     sync.Mutex
	counts map[summaryKey]int64
}

type summaryKey struct {
	tenant     string
	client     string
	model      string
	operation  string
	statusCode int
}

func NewAggregator() *Aggregator {
	return &Aggregator{counts: make(map[summaryKey]int64)}
}

func (a *Aggregator) Record(event Event) {
	a.mu.Lock()
	if a.counts == nil {
		a.counts = make(map[summaryKey]int64)
	}
	a.counts[summaryKey{
		tenant:     event.Tenant,
		client:     event.Client,
		model:      event.Model,
		operation:  event.Operation,
		statusCode: event.StatusCode,
	}]++
	a.mu.Unlock()
}

func (a *Aggregator) Summaries() []Summary {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Summary, 0, len(a.counts))
	for key, count := range a.counts {
		out = append(out, Summary{
			Tenant:     key.tenant,
			Client:     key.client,
			Model:      key.model,
			Operation:  key.operation,
			StatusCode: key.statusCode,
			Count:      count,
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
