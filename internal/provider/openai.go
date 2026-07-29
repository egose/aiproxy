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
	if r.BaseURL == "" {
		r.BaseURL = defaultOpenAIBaseURL
	}
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
	return executeUpstream(r, req, upstreamResponseHandlers{
		PreferStreaming: true,
		IsStreaming: func(resp *http.Response) bool {
			return streaming || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream")
		},
		OnStream: func(resp *http.Response) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, StreamBody: resp.Body, Streaming: true}, nil
		},
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
		},
	})
}

func openAIPathForOperation(op Operation) (string, error) {
	switch op {
	case OpChatCompletions:
		return "/v1/chat/completions", nil
	case OpEmbeddings:
		return "/v1/embeddings", nil
	case OpResponses:
		return "/v1/responses", nil
	case OpImagesGenerations:
		return "/v1/images/generations", nil
	case OpAudioTranscriptions:
		return "/v1/audio/transcriptions", nil
	case OpAudioSpeech:
		return "/v1/audio/speech", nil
	default:
		return "", ErrUnsupportedOperation{ProviderType: "openai-compatible", Operation: op}
	}
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
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	m["model"] = upstreamModel
	return json.Marshal(m)
}

func isStream(body []byte) bool {
	var probe struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Stream
}
