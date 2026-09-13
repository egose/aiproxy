package payloadlog

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/egose/aiproxy/internal/config"
)

type Body struct {
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
	Encoding  string `json:"encoding,omitempty"`
	Data      string `json:"data"`
}

type Entry struct {
	Timestamp     string    `json:"ts"`
	RequestID     string    `json:"request_id,omitempty"`
	Method        string    `json:"method,omitempty"`
	Path          string    `json:"path,omitempty"`
	Operation     string    `json:"operation,omitempty"`
	PublicModel   string    `json:"public_model,omitempty"`
	Client        string    `json:"client,omitempty"`
	Tenant        string    `json:"tenant,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	UpstreamModel string    `json:"upstream_model,omitempty"`
	Status        int       `json:"status"`
	DurationMs    int64     `json:"duration_ms"`
	Streaming     bool      `json:"streaming,omitempty"`
	Error         string    `json:"error,omitempty"`
	Request       EntrySide `json:"request"`
	Response      EntrySide `json:"response"`
}

type EntrySide struct {
	Headers map[string][]string `json:"headers"`
	Body    Body                `json:"body"`
}

var redactedHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"proxy-authenticate":  {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
}

func RedactHeaders(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		if _, ok := redactedHeaders[strings.ToLower(k)]; ok {
			out[k] = []string{"[REDACTED]"}
			continue
		}
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func EncodeBody(b []byte, max int) Body {
	out := Body{Bytes: len(b)}
	data := b
	if max > 0 && len(data) > max {
		data = data[:max]
		out.Truncated = true
	}
	if !utf8.Valid(data) {
		out.Encoding = "base64"
		out.Data = base64.StdEncoding.EncodeToString(data)
		return out
	}
	out.Data = string(data)
	return out
}

type Logger struct {
	mu        sync.Mutex
	dir       string
	rotation  config.PayloadLogRotation
	retention time.Duration
	maxBody   int
	bucket    string
	file      *os.File
	stopCh    chan struct{}
	stoppedCh chan struct{}
	lastSweep time.Time
}

func New(cfg config.PayloadLog) (*Logger, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.Dir == "" {
		return nil, fmt.Errorf("payload log dir is required")
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create payload log dir %s: %w", cfg.Dir, err)
	}
	l := &Logger{
		dir:       cfg.Dir,
		rotation:  cfg.Rotation,
		retention: cfg.Retention,
		maxBody:   cfg.MaxBodyBytes,
		stopCh:    make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
	l.sweep()
	go l.retentionLoop()
	return l, nil
}

func (l *Logger) MaxBodyBytes() int {
	return l.maxBody
}

func (l *Logger) Config() config.PayloadLog {
	return config.PayloadLog{Enabled: true, Dir: l.dir, Rotation: l.rotation, Retention: l.retention, MaxBodyBytes: l.maxBody}
}

func BucketFor(t time.Time, rotation config.PayloadLogRotation) string {
	t = t.UTC()
	if rotation == config.PayloadLogRotationHourly {
		return t.Format("20060102-15")
	}
	return t.Format("20060102")
}

func (l *Logger) filename(bucket string) string {
	return filepath.Join(l.dir, config.PayloadLogFilePrefix+bucket+config.PayloadLogFileExt)
}

func (l *Logger) Record(e Entry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := BucketFor(time.Now(), l.rotation)
	if l.file == nil || bucket != l.bucket {
		if err := l.rotateLocked(bucket); err != nil {
			return err
		}
	}
	if _, err := l.file.Write(line); err != nil {
		return err
	}
	if time.Since(l.lastSweep) > time.Hour {
		l.sweepLocked(time.Now())
	}
	return nil
}

func (l *Logger) rotateLocked(bucket string) error {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	f, err := os.OpenFile(l.filename(bucket), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open payload log file: %w", err)
	}
	l.file = f
	l.bucket = bucket
	l.sweepLocked(time.Now())
	return nil
}

func (l *Logger) retentionLoop() {
	defer close(l.stoppedCh)
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case now := <-t.C:
			l.mu.Lock()
			l.sweepLocked(now)
			l.mu.Unlock()
		}
	}
}

func (l *Logger) sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweepLocked(time.Now())
}

func (l *Logger) sweepLocked(now time.Time) {
	l.lastSweep = now
	if l.retention <= 0 {
		return
	}
	cutoff := now.Add(-l.retention)
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, config.PayloadLogFilePrefix) || !strings.HasSuffix(name, config.PayloadLogFileExt) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(l.dir, name))
	}
}

func (l *Logger) Close() error {
	select {
	case <-l.stopCh:
		<-l.stoppedCh
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.file != nil {
			err := l.file.Close()
			l.file = nil
			return err
		}
		return nil
	default:
	}
	close(l.stopCh)
	<-l.stoppedCh
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}

type Capture struct {
	rc        io.ReadCloser
	buf       []byte
	max       int
	truncated bool
	total     int
}

func NewCapture(rc io.ReadCloser, max int) *Capture {
	return &Capture{rc: rc, max: max}
}

func (c *Capture) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.total += n
		chunk := p[:n]
		if c.max > 0 && len(c.buf) >= c.max {
			c.truncated = true
		} else {
			if c.max > 0 && len(c.buf)+len(chunk) > c.max {
				chunk = chunk[:c.max-len(c.buf)]
				c.truncated = true
			}
			c.buf = append(c.buf, chunk...)
		}
	}
	return n, err
}

func (c *Capture) Close() error {
	return c.rc.Close()
}

func (c *Capture) Body() Body {
	out := EncodeBody(c.buf, 0)
	out.Bytes = c.total
	out.Truncated = c.truncated || (c.max > 0 && c.total > c.max)
	return out
}
