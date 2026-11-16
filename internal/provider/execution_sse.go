package provider

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/egose/aiproxy/internal/deferred"
)

type executionSSEBuffer struct {
	state     *executionState
	line      []byte
	data      []byte
	eventType string
	dataLines int
	release   func()
}

func newExecutionSSEBuffer(state *executionState) (*executionSSEBuffer, error) {
	l := state.limits
	release, err := state.reserve(int64(l.SSELineBytes)*2 + int64(l.SSEEventBytes)*2 + 16<<10)
	if err != nil {
		return nil, err
	}
	return &executionSSEBuffer{
		state: state, line: make([]byte, 0, l.SSELineBytes), data: make([]byte, 0, l.SSEEventBytes), release: release,
	}, nil
}

func (b *executionSSEBuffer) close() {
	if b.release == nil {
		return
	}
	b.line, b.data, b.eventType = nil, nil, ""
	b.release()
	b.release = nil
}

func (b *executionSSEBuffer) appendLine(part []byte) error {
	if len(part) > b.state.limits.SSELineBytes-len(b.line) {
		return ErrExecutionLimit{Kind: "SSE line bytes", Limit: int64(b.state.limits.SSELineBytes)}
	}
	b.line = append(b.line, part...)
	return nil
}

func (b *executionSSEBuffer) processLine() (bool, error) {
	line := bytes.TrimRight(b.line, "\r\n")
	defer func() { b.line = b.line[:0] }()
	if len(line) == 0 {
		return b.hasEvent(), nil
	}
	if line[0] == ':' {
		return false, nil
	}
	field, value, _ := bytes.Cut(line, []byte(":"))
	if len(value) > 0 && value[0] == ' ' {
		value = value[1:]
	}
	switch string(field) {
	case "event":
		b.eventType = string(value)
	case "data":
		if b.dataLines >= b.state.limits.SSEDataLines {
			return false, ErrExecutionLimit{Kind: "SSE data lines", Limit: int64(b.state.limits.SSEDataLines)}
		}
		added := len(value)
		if b.dataLines > 0 {
			added++
		}
		if added > b.state.limits.SSEEventBytes-len(b.data) {
			return false, ErrExecutionLimit{Kind: "SSE event bytes", Limit: int64(b.state.limits.SSEEventBytes)}
		}
		if b.dataLines > 0 {
			b.data = append(b.data, '\n')
		}
		b.data = append(b.data, value...)
		b.dataLines++
	}
	return false, nil
}

func (b *executionSSEBuffer) hasEvent() bool { return b.eventType != "" || b.dataLines != 0 }

func (b *executionSSEBuffer) event(required bool) (sseEvent, func(), error) {
	defer func() { b.data, b.dataLines, b.eventType = b.data[:0], 0, "" }()
	shape := deferred.JSONShape{}
	if len(b.data) > 0 && (required || !bytes.Equal(bytes.TrimSpace(b.data), []byte("[DONE]"))) {
		var err error
		shape, err = deferred.InspectJSON(b.data, b.state.limits.JSONDepth, b.state.limits.JSONTokens)
		if err != nil {
			var limit deferred.JSONLimitError
			if errors.As(err, &limit) {
				return sseEvent{}, nil, ErrExecutionLimit{Kind: "JSON " + limit.Kind, Limit: int64(limit.Limit)}
			}
			if required {
				return sseEvent{}, nil, fmt.Errorf("invalid translated SSE JSON: %w", err)
			}
			return sseEvent{}, func() {}, nil
		}
	}
	release, err := b.state.reserve(jsonDecodeWorkBytes(len(b.data), shape.Tokens))
	if err != nil {
		return sseEvent{}, nil, err
	}
	return sseEvent{Type: b.eventType, Data: string(b.data)}, release, nil
}

type executionSSEDecoder struct {
	reader  *bufio.Reader
	buffer  *executionSSEBuffer
	eof     bool
	release func()
}

func newExecutionSSEDecoder(src io.Reader, state *executionState) (*executionSSEDecoder, error) {
	buffer, err := newExecutionSSEBuffer(state)
	if err != nil {
		return nil, err
	}
	release, err := state.reserve(4 << 10)
	if err != nil {
		buffer.close()
		return nil, err
	}
	return &executionSSEDecoder{reader: bufio.NewReaderSize(src, 4<<10), buffer: buffer, release: release}, nil
}

func (d *executionSSEDecoder) close() {
	d.buffer.close()
	d.reader = nil
	d.release()
}

func (d *executionSSEDecoder) next() (sseEvent, func(), error) {
	for !d.eof {
		part, err := d.reader.ReadSlice('\n')
		if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
			return sseEvent{}, nil, err
		}
		if appendErr := d.buffer.appendLine(part); appendErr != nil {
			return sseEvent{}, nil, appendErr
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		d.eof = err == io.EOF
		ready, lineErr := d.buffer.processLine()
		if lineErr != nil {
			return sseEvent{}, nil, lineErr
		}
		if ready || d.eof && d.buffer.hasEvent() {
			return d.buffer.event(true)
		}
	}
	return sseEvent{}, nil, io.EOF
}

type executionOnceBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (b *executionOnceBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

type executionPassthroughBody struct {
	source *executionOnceBody
	state  *executionState
	stream *StreamCompletion
	readMu sync.Mutex
	mu     sync.Mutex
	buffer *executionSSEBuffer
	bytes  int64
	closed bool
}

func openAIStreamWithLimits(source io.ReadCloser, stream *StreamCompletion, state *executionState) (io.ReadCloser, error) {
	if state == nil {
		return newOpenAIStreamUsageReadCloser(source, stream), nil
	}
	buffer, err := newExecutionSSEBuffer(state)
	if err != nil {
		return nil, err
	}
	return &executionPassthroughBody{source: &executionOnceBody{ReadCloser: source}, state: state, stream: stream, buffer: buffer}, nil
}

func (b *executionPassthroughBody) Read(p []byte) (int, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()
	release, ok := b.state.retain()
	if !ok {
		return 0, io.ErrClosedPipe
	}
	defer release()
	if len(p) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	remaining, closed := b.state.limits.ResponseBytes-b.bytes, b.closed
	b.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	readSize := min(int64(len(p)), remaining+1)
	n, err := b.source.Read(p[:int(readSize)])
	b.mu.Lock()
	defer b.mu.Unlock()
	if int64(n) > remaining {
		n = int(remaining)
		err = ErrExecutionLimit{Kind: "response bytes", Limit: b.state.limits.ResponseBytes}
	}
	b.bytes += int64(n)
	if !b.closed && b.buffer != nil {
		observeErr := b.observe(p[:n])
		if observeErr == nil && err == io.EOF {
			if len(b.buffer.line) > 0 {
				_, observeErr = b.buffer.processLine()
			}
			if observeErr == nil && b.buffer.hasEvent() {
				observeErr = b.flushEvent()
			}
		}
		if observeErr != nil || err != nil {
			b.buffer.close()
			b.buffer = nil
		}
	}
	if err != nil && err != io.EOF {
		b.stream.Complete(boundedExecutionError(err), false)
	}
	return n, err
}

func (b *executionPassthroughBody) observe(chunk []byte) error {
	for len(chunk) > 0 {
		at := bytes.IndexByte(chunk, '\n')
		size := len(chunk)
		if at >= 0 {
			size = at + 1
		}
		if err := b.buffer.appendLine(chunk[:size]); err != nil {
			return err
		}
		chunk = chunk[size:]
		if at >= 0 {
			ready, err := b.buffer.processLine()
			if err != nil {
				return err
			}
			if ready {
				if err := b.flushEvent(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *executionPassthroughBody) flushEvent() error {
	event, release, err := b.buffer.event(false)
	if err != nil {
		return err
	}
	defer release()
	if event.Data == "" || strings.TrimSpace(event.Data) == "[DONE]" {
		return nil
	}
	if err := openAIStreamError([]byte(event.Data)); err != nil {
		b.stream.Complete(boundedExecutionError(err), false)
		return nil
	}
	if usage := usageFromSSEData([]byte(event.Data)); usage.Has() {
		b.stream.SetUsage(usage)
	}
	return nil
}

func (b *executionPassthroughBody) Close() error {
	err := b.source.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	if b.buffer != nil {
		b.buffer.close()
		b.buffer = nil
	}
	return err
}

type executionStreamWriter struct {
	io.Writer
	state *executionState
	bytes int64
}

func (w *executionStreamWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.state.limits.ResponseBytes-w.bytes {
		return 0, ErrExecutionLimit{Kind: "response bytes", Limit: w.state.limits.ResponseBytes}
	}
	n, err := w.Writer.Write(data)
	w.bytes += int64(n)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (w *executionStreamWriter) writeChunk(chunk openAIChunk) error {
	remaining := w.state.limits.ResponseBytes - w.bytes - int64(len("data: \n\n"))
	if remaining < 0 {
		return ErrExecutionLimit{Kind: "response bytes", Limit: w.state.limits.ResponseBytes}
	}
	data, release, err := w.state.marshal(chunk, remaining, false)
	if err != nil {
		return err
	}
	defer release()
	for _, part := range [][]byte{[]byte("data: "), data, []byte("\n\n")} {
		if _, err := w.Write(part); err != nil {
			return err
		}
	}
	return nil
}

type executionString struct {
	value   string
	release func()
}

func (s *executionString) set(value *string, state *executionState) error {
	if s.value == *value {
		*value = s.value
		return nil
	}
	release, err := state.reserve(int64(len(*value)) + 128)
	if err != nil {
		return err
	}
	owned := strings.Clone(*value)
	s.close()
	s.value, s.release, *value = owned, release, owned
	return nil
}

func (s *executionString) close() {
	if s.release != nil {
		s.release()
	}
	s.value, s.release = "", nil
}

func anthropicChatStreamWithLimits(source io.ReadCloser, publicModel string, stream *StreamCompletion, state *executionState) (io.ReadCloser, error) {
	if state == nil {
		return translateAnthropicStream(source, publicModel, stream), nil
	}
	return translatedChatStreamWithLimits(source, publicModel, stream, state, true)
}

func geminiChatStreamWithLimits(source io.ReadCloser, publicModel string, stream *StreamCompletion, state *executionState) (io.ReadCloser, error) {
	if state == nil {
		return translateGeminiStream(source, publicModel, stream), nil
	}
	return translatedChatStreamWithLimits(source, publicModel, stream, state, false)
}

func translatedChatStreamWithLimits(source io.ReadCloser, publicModel string, stream *StreamCompletion, state *executionState, anthropic bool) (io.ReadCloser, error) {
	src := &executionOnceBody{ReadCloser: source}
	decoder, err := newExecutionSSEDecoder(src, state)
	if err != nil {
		return nil, err
	}
	release, ok := state.retain()
	if !ok {
		decoder.close()
		return nil, io.ErrClosedPipe
	}
	return newTranslatedStream(src, func(pw *io.PipeWriter) {
		defer release()
		defer src.Close()
		defer pw.Close()
		defer decoder.close()
		writer := &executionStreamWriter{Writer: pw, state: state}
		var streamID, finishReason string
		var ownedID, ownedFinish executionString
		defer ownedID.close()
		defer ownedFinish.close()
		completed, sentRole := false, false
		fail := func(err error) {
			err = boundedExecutionError(err)
			stream.Complete(err, false)
			_ = pw.CloseWithError(err)
		}
		for {
			event, releaseEvent, err := decoder.next()
			if err != nil {
				if err != io.EOF {
					fail(err)
				} else if !completed {
					if anthropic {
						fail(fmt.Errorf("upstream stream truncated: anthropic chat stream ended without message_stop"))
					} else {
						fail(fmt.Errorf("upstream stream truncated: gemini chat stream ended without finishReason"))
					}
				}
				return
			}
			done := false
			if anthropic {
				done, err = processAnthropicEvent(writer, event.Type, event.Data, publicModel, &streamID, &finishReason, stream)
				if err == nil {
					err = ownedID.set(&streamID, state)
				}
				if err == nil {
					err = ownedFinish.set(&finishReason, state)
				}
			} else if event.Data != "" {
				var wrote bool
				wrote, done, err = processGeminiPayload(writer, event.Data, publicModel, &sentRole, stream)
				done = wrote && done
			}
			if err != nil {
				fail(err)
				releaseEvent()
				return
			}
			releaseEvent()
			completed = completed || done
			if anthropic && done {
				return
			}
		}
	}), nil
}
