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

		field, value, ok := strings.Cut(line, ":")
		if ok && strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		if !ok {
			value = ""
		}

		switch field {
		case "event":
			d.eventType = strings.TrimSpace(value)
		case "data":
			if err := d.appendData(strings.TrimSpace(value)); err != nil {
				return sseEvent{}, err
			}
		}
	}
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
