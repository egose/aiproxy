package observability

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type LogEntry struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Attrs   string
}

type LogBuffer struct {
	mu      sync.Mutex
	entries []LogEntry
	head    int
	filled  bool
	cap     int
	notify  chan struct{}
}

func NewLogBuffer(capacity int) *LogBuffer {
	if capacity <= 0 {
		capacity = 500
	}
	return &LogBuffer{
		entries: make([]LogEntry, capacity),
		cap:     capacity,
		notify:  make(chan struct{}, 1),
	}
}

func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.cap
	if !b.filled && b.head == 0 {
		b.filled = true
	}
	b.mu.Unlock()
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func (b *LogBuffer) Since(n int) []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	available := b.head
	if b.filled {
		available = b.cap
	}
	if n > available {
		n = available
	}
	if n <= 0 {
		return nil
	}
	out := make([]LogEntry, n)
	start := b.head - n
	if start < 0 {
		start += b.cap
	}
	for i := 0; i < n; i++ {
		idx := (start + i) % b.cap
		if idx < 0 {
			idx += b.cap
		}
		out[i] = b.entries[idx]
	}
	return out
}

func (b *LogBuffer) Notify() <-chan struct{} {
	return b.notify
}

type bufferingHandler struct {
	slog.Handler
	buffer *LogBuffer
	attrs  []slog.Attr
	groups []string
}

func WrapWithBuffer(h slog.Handler, buffer *LogBuffer) slog.Handler {
	if h == nil || buffer == nil {
		return h
	}
	return &bufferingHandler{Handler: h, buffer: buffer}
}

func (h *bufferingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.Handler = h.Handler.WithAttrs(attrs)
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &next
}

func (h *bufferingHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.Handler = h.Handler.WithGroup(name)
	next.groups = append(append([]string(nil), h.groups...), name)
	return &next
}

func (h *bufferingHandler) Handle(ctx context.Context, r slog.Record) error {
	h.buffer.Add(LogEntry{
		Time:    r.Time,
		Level:   r.Level,
		Message: r.Message,
		Attrs:   formatAttrs(r, h.attrs, h.groups),
	})
	return h.Handler.Handle(ctx, r)
}

func formatAttrs(r slog.Record, pinned []slog.Attr, groups []string) string {
	var b strings.Builder
	for _, a := range pinned {
		writeAttr(&b, a, groups)
	}
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, a, groups)
		return true
	})
	return strings.TrimRight(b.String(), " ")
}

func writeAttr(b *strings.Builder, a slog.Attr, groups []string) {
	if len(groups) > 0 {
		b.WriteString(strings.Join(groups, "."))
		b.WriteByte('.')
	}
	b.WriteString(a.Key)
	b.WriteByte('=')
	if v, ok := a.Value.Any().(string); ok {
		b.WriteString(v)
	} else {
		b.WriteString(a.Value.String())
	}
	b.WriteByte(' ')
}

type writerSink struct {
	w io.Writer
}

func (s writerSink) Write(p []byte) (int, error) { return s.w.Write(p) }
