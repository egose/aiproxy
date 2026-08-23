package provider

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

const (
	maxSSELineBytes  = 1 << 20
	maxSSEEventBytes = maxUpstreamBodyBytes
)

type ErrSSEOverflow struct {
	Kind  string
	Limit int
}

func (e ErrSSEOverflow) Error() string {
	return fmt.Sprintf("sse %s exceeds %d bytes", e.Kind, e.Limit)
}

type sseEvent struct {
	Type string
	Data string
}

type sseDecoder struct {
	r         *bufio.Reader
	maxLine   int
	maxEvent  int
	eventType string
	dataLines []string
	dataBytes int
}

func newSSEDecoder(r io.Reader) *sseDecoder {
	return &sseDecoder{r: bufio.NewReader(r), maxLine: maxSSELineBytes, maxEvent: maxSSEEventBytes}
}

func (d *sseDecoder) Next() (sseEvent, error) {
	for {
		line, err := d.readLine()
		if err != nil {
			if errors.Is(err, io.EOF) && d.hasEvent() {
				return d.flushEvent(), nil
			}
			return sseEvent{}, err
		}

		if line == "" {
			if d.hasEvent() {
				return d.flushEvent(), nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}

		field, value := parseSSEField(line)

		switch field {
		case "event":
			d.eventType = value
		case "data":
			if err := d.appendData(value); err != nil {
				return sseEvent{}, err
			}
		}
	}
}

func parseSSEField(line string) (string, string) {
	line = strings.TrimRight(line, "\r")
	if line == "" || strings.HasPrefix(line, ":") {
		return "", ""
	}
	field, value, ok := strings.Cut(line, ":")
	if !ok {
		return field, ""
	}
	if strings.HasPrefix(value, " ") {
		value = value[1:]
	}
	return field, value
}

func (d *sseDecoder) readLine() (string, error) {
	var parts [][]byte
	total := 0
	for {
		part, err := d.r.ReadSlice('\n')
		total += len(part)
		if total > d.maxLine {
			return "", ErrSSEOverflow{Kind: "line", Limit: d.maxLine}
		}
		if err == bufio.ErrBufferFull {
			parts = append(parts, append([]byte(nil), part...))
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) && total > 0 {
				parts = append(parts, part)
				return trimSSELine(parts), nil
			}
			return "", err
		}
		parts = append(parts, part)
		return trimSSELine(parts), nil
	}
}

func trimSSELine(parts [][]byte) string {
	line := string(bytesJoin(parts))
	return strings.TrimRight(line, "\r\n")
}

func bytesJoin(parts [][]byte) []byte {
	if len(parts) == 1 {
		return parts[0]
	}
	size := 0
	for _, part := range parts {
		size += len(part)
	}
	joined := make([]byte, 0, size)
	for _, part := range parts {
		joined = append(joined, part...)
	}
	return joined
}

func (d *sseDecoder) appendData(value string) error {
	added := len(value)
	if len(d.dataLines) > 0 {
		added++
	}
	if d.dataBytes+added > d.maxEvent {
		return ErrSSEOverflow{Kind: "event", Limit: d.maxEvent}
	}
	d.dataLines = append(d.dataLines, value)
	d.dataBytes += added
	return nil
}

func (d *sseDecoder) hasEvent() bool {
	return d.eventType != "" || len(d.dataLines) > 0
}

func (d *sseDecoder) flushEvent() sseEvent {
	event := sseEvent{Type: d.eventType, Data: strings.Join(d.dataLines, "\n")}
	d.eventType = ""
	d.dataLines = nil
	d.dataBytes = 0
	return event
}

type sseObserver struct {
	maxLine   int
	maxEvent  int
	line      []byte
	eventType string
	dataLines []string
	dataBytes int
	disabled  bool
	onEvent   func(sseEvent)
}

func newSSEObserver(onEvent func(sseEvent)) *sseObserver {
	return &sseObserver{maxLine: maxSSELineBytes, maxEvent: maxSSEEventBytes, onEvent: onEvent}
}

func (o *sseObserver) Observe(chunk []byte) error {
	if o == nil || o.disabled {
		return nil
	}
	for _, b := range chunk {
		if b == '\n' {
			if len(o.line)+1 > o.maxLine {
				return o.disable(ErrSSEOverflow{Kind: "line", Limit: o.maxLine})
			}
			if err := o.observeLine(); err != nil {
				return o.disable(err)
			}
			continue
		}
		if len(o.line)+1 > o.maxLine {
			return o.disable(ErrSSEOverflow{Kind: "line", Limit: o.maxLine})
		}
		o.line = append(o.line, b)
	}
	return nil
}

func (o *sseObserver) ObserveEOF() error {
	if o == nil || o.disabled {
		return nil
	}
	if len(o.line) > 0 {
		if err := o.observeLine(); err != nil {
			return o.disable(err)
		}
	}
	o.flushEvent()
	return nil
}

func (o *sseObserver) observeLine() error {
	line := string(o.line)
	o.line = o.line[:0]
	if strings.TrimRight(line, "\r") == "" {
		o.flushEvent()
		return nil
	}
	field, value := parseSSEField(line)
	switch field {
	case "event":
		o.eventType = value
	case "data":
		return o.appendData(value)
	}
	return nil
}

func (o *sseObserver) appendData(value string) error {
	added := len(value)
	if len(o.dataLines) > 0 {
		added++
	}
	if o.dataBytes+added > o.maxEvent {
		return ErrSSEOverflow{Kind: "event", Limit: o.maxEvent}
	}
	o.dataLines = append(o.dataLines, value)
	o.dataBytes += added
	return nil
}

func (o *sseObserver) flushEvent() {
	if o.eventType == "" && len(o.dataLines) == 0 {
		return
	}
	event := sseEvent{Type: o.eventType, Data: strings.Join(o.dataLines, "\n")}
	o.eventType = ""
	o.dataLines = nil
	o.dataBytes = 0
	if o.onEvent != nil {
		o.onEvent(event)
	}
}

func (o *sseObserver) disable(err error) error {
	o.disabled = true
	o.line = nil
	o.eventType = ""
	o.dataLines = nil
	o.dataBytes = 0
	return err
}

type pipeReadCloser struct {
	*io.PipeReader
	upstream io.Closer
	once     sync.Once
}

func (r *pipeReadCloser) Close() error {
	var err error
	r.once.Do(func() {
		if r.upstream != nil {
			err = r.upstream.Close()
		}
		if pipeErr := r.PipeReader.Close(); err == nil {
			err = pipeErr
		}
	})
	return err
}

func newTranslatedStream(src io.ReadCloser, translate func(*io.PipeWriter)) io.ReadCloser {
	pr, pw := io.Pipe()
	go translate(pw)
	return &pipeReadCloser{PipeReader: pr, upstream: src}
}
