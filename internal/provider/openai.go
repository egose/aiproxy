package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
)

func (a *adapter) doOpenAI(ctx context.Context, r Request) (*Result, error) {
	if r.Operation == OpAudioTranscriptions {
		return a.doOpenAIAudioTranscriptions(ctx, r)
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	rewritten, err := rewriteModel(body, r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("rewrite model: %v", err)}
	}

	path, err := openAIPathForOperation(r.Operation)
	if err != nil {
		return nil, err
	}
	target := joinBaseURLAndPath(r.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(rewritten))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if accept := r.Inbound.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}

	streaming := (r.Operation == OpChatCompletions || r.Operation == OpResponses) && isStream(body)
	return executeUpstream(r, req, openAIPassthroughHandlers(r, streaming))
}

func openAIPassthroughHandlers(r Request, streaming bool) upstreamResponseHandlers {
	return upstreamResponseHandlers{
		PreferStreaming: true,
		StreamSuccess:   streamsOpaqueSuccess(r.Operation),
		IsStreaming: func(resp *http.Response) bool {
			return streaming || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream")
		},
		OnStream: func(resp *http.Response) (*Result, error) {
			stream := NewStreamCompletion()
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, StreamBody: newOpenAIStreamUsageReadCloser(resp.Body, stream), Streaming: true, Stream: stream}, nil
		},
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body, Usage: usageFromBody(body)}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
		},
	}
}

type openAIStreamUsageReadCloser struct {
	io.ReadCloser
	observer *sseObserver
}

func newOpenAIStreamUsageReadCloser(src io.ReadCloser, stream *StreamCompletion) io.ReadCloser {
	return &openAIStreamUsageReadCloser{ReadCloser: src, observer: newSSEObserver(func(event sseEvent) {
		observeOpenAIStreamEvent(stream, event)
	})}
}

func (r *openAIStreamUsageReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		_ = r.observer.Observe(p[:n])
	}
	if err == io.EOF {
		_ = r.observer.ObserveEOF()
	}
	return n, err
}

func observeOpenAIStreamEvent(stream *StreamCompletion, event sseEvent) {
	if event.Data == "" {
		return
	}
	if strings.TrimSpace(event.Data) == "[DONE]" {
		return
	}
	if err := openAIStreamError([]byte(event.Data)); err != nil {
		stream.Complete(err, false)
		return
	}
	if usage := usageFromBody([]byte(event.Data)); usage.Has() {
		stream.SetUsage(usage)
	}
}

func openAIStreamError(data []byte) error {
	var event struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &event) != nil || len(event.Error) == 0 || string(event.Error) == "null" {
		return nil
	}
	var detail struct {
		Message string          `json:"message"`
		Type    string          `json:"type"`
		Code    json.RawMessage `json:"code"`
	}
	if json.Unmarshal(event.Error, &detail) == nil {
		parts := make([]string, 0, 3)
		if detail.Type != "" {
			parts = append(parts, detail.Type)
		}
		if len(detail.Code) > 0 && string(detail.Code) != "null" {
			parts = append(parts, strings.Trim(string(detail.Code), `"`))
		}
		if detail.Message != "" {
			parts = append(parts, detail.Message)
		}
		if len(parts) > 0 {
			return fmt.Errorf("upstream stream error: %s", strings.Join(parts, ": "))
		}
	}
	return fmt.Errorf("upstream stream error: %s", strings.TrimSpace(string(event.Error)))
}

func (a *adapter) doOpenAIAudioTranscriptions(ctx context.Context, r Request) (*Result, error) {
	contentType := r.Inbound.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "multipart/form-data;") {
		return nil, ErrInvalidRequest{Message: "audio transcription requires multipart/form-data"}
	}
	boundary := multipartBoundary(contentType)
	if boundary == "" {
		return nil, ErrInvalidRequest{Message: "multipart boundary is required"}
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	rewritten, rewrittenType, err := rewriteMultipartModel(body, boundary, r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("rewrite model: %v", err)}
	}
	path, err := openAIPathForOperation(r.Operation)
	if err != nil {
		return nil, err
	}
	target := joinBaseURLAndPath(r.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(rewritten))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", rewrittenType)
	if accept := r.Inbound.Header.Get("Accept"); accept != "" {
		req.Header.Set("Accept", accept)
	}
	return executeUpstream(r, req, upstreamResponseHandlers{
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
		},
	})
}

func streamsOpaqueSuccess(op Operation) bool {
	switch op {
	case OpImagesGenerations, OpAudioSpeech:
		return true
	default:
		return false
	}
}

func multipartBoundary(contentType string) string {
	parts := strings.Split(contentType, ";")
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "boundary=") {
			return strings.Trim(strings.TrimPrefix(part, "boundary="), `"`)
		}
	}
	return ""
}

func rewriteMultipartModel(body []byte, boundary, upstreamModel string) ([]byte, string, error) {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	writer := multipart.NewWriter(&out)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writer.Close()
			return nil, "", err
		}
		headers := make(textproto.MIMEHeader)
		for k, vals := range part.Header {
			copied := make([]string, len(vals))
			copy(copied, vals)
			headers[k] = copied
		}
		w, err := writer.CreatePart(headers)
		if err != nil {
			writer.Close()
			return nil, "", err
		}
		if part.FormName() == "model" {
			if _, err := io.WriteString(w, upstreamModel); err != nil {
				writer.Close()
				return nil, "", err
			}
		} else {
			if _, err := io.Copy(w, part); err != nil {
				writer.Close()
				return nil, "", err
			}
		}
		part.Close()
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), writer.FormDataContentType(), nil
}

func joinBaseURLAndPath(baseURL, path string) string {
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/v1") && strings.HasPrefix(path, "/v1/") {
		return baseURL + strings.TrimPrefix(path, "/v1")
	}
	return baseURL + path
}

func rewriteModel(body []byte, upstreamModel string) (json.RawMessage, error) {
	return rewriteTopLevelModel(body, upstreamModel)
}

func isStream(body []byte) bool {
	var probe struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Stream
}
