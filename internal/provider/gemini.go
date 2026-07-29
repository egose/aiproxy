package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
}

type geminiEmbedRequest struct {
	Content              geminiContent `json:"content"`
	OutputDimensionality int           `json:"outputDimensionality,omitempty"`
}

type geminiBatchEmbedRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiGenConfig struct {
	MaxOutputTokens int      `json:"maxOutputTokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiEmbedding struct {
	Values []float64 `json:"values"`
}

type geminiEmbedResponse struct {
	Embedding     geminiEmbedding     `json:"embedding"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type geminiBatchEmbedResponse struct {
	Embeddings    []geminiEmbedding   `json:"embeddings"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type openAIEmbeddingRequest struct {
	Input      json.RawMessage `json:"input"`
	Dimensions int             `json:"dimensions,omitempty"`
}

type openAIEmbeddingResponse struct {
	Object string                 `json:"object"`
	Data   []openAIEmbeddingDatum `json:"data"`
	Model  string                 `json:"model"`
	Usage  openAIEmbeddingUsage   `json:"usage"`
}

type openAIEmbeddingDatum struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type openAIEmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

func (a *adapter) doGemini(ctx context.Context, r Request) (*Result, error) {
	switch r.Operation {
	case OpChatCompletions:
		return a.doGeminiChat(ctx, r)
	case OpEmbeddings:
		return a.doGeminiEmbeddings(ctx, r)
	case OpResponses:
		return a.doGeminiResponses(ctx, r)
	default:
		return nil, ErrUnsupportedOperation{ProviderType: config.ProviderTypeGemini, Operation: r.Operation}
	}
}

func (a *adapter) doGeminiChat(ctx context.Context, r Request) (*Result, error) {
	if r.BaseURL == "" {
		r.BaseURL = defaultGeminiBaseURL
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	translated, streaming, err := translateOpenAIToGemini(body)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	path := "/v1beta/models/" + r.UpstreamModel + ":generateContent"
	if streaming {
		path = "/v1beta/models/" + r.UpstreamModel + ":streamGenerateContent?alt=sse"
	}
	target := strings.TrimRight(r.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body: %w", err)
		}
		return &Result{
			StatusCode: resp.StatusCode,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       translateGeminiError(errBody),
		}, nil
	}
	if streaming {
		return &Result{
			StatusCode: resp.StatusCode,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			StreamBody: translateGeminiStream(resp.Body, r.PublicModel),
			Streaming:  true,
		}, nil
	}
	defer resp.Body.Close()

	outBody, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	translatedBody, err := translateGeminiResponse(outBody, r.PublicModel)
	if err != nil {
		return nil, fmt.Errorf("translate response: %w", err)
	}
	return &Result{
		StatusCode: resp.StatusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       translatedBody,
	}, nil
}

func (a *adapter) doGeminiEmbeddings(ctx context.Context, r Request) (*Result, error) {
	if r.BaseURL == "" {
		r.BaseURL = defaultGeminiBaseURL
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	translated, batch, err := translateOpenAIEmbeddingsToGemini(body)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	path := "/v1beta/models/" + r.UpstreamModel + ":embedContent"
	if batch {
		path = "/v1beta/models/" + r.UpstreamModel + ":batchEmbedContents"
	}
	target := strings.TrimRight(r.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", r.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body: %w", err)
		}
		return &Result{
			StatusCode: resp.StatusCode,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       translateGeminiError(errBody),
		}, nil
	}
	defer resp.Body.Close()
	outBody, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	translatedBody, err := translateGeminiEmbeddingResponse(outBody, r.PublicModel, batch)
	if err != nil {
		return nil, fmt.Errorf("translate response: %w", err)
	}
	return &Result{
		StatusCode: resp.StatusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       translatedBody,
	}, nil
}

func (a *adapter) doGeminiResponses(ctx context.Context, r Request) (*Result, error) {
	if r.BaseURL == "" {
		r.BaseURL = defaultGeminiBaseURL
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	chatReq, err := translateOpenAIResponsesInput(body)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	translated, streaming, err := translateOpenAIToGemini(mustMarshalJSON(chatReq))
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	path := "/v1beta/models/" + r.UpstreamModel + ":generateContent"
	if streaming {
		path = "/v1beta/models/" + r.UpstreamModel + ":streamGenerateContent?alt=sse"
	}
	target := strings.TrimRight(r.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-goog-api-key", r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body: %w", err)
		}
		return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translateGeminiError(errBody)}, nil
	}
	if streaming {
		return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, StreamBody: translateGeminiResponsesStream(resp.Body, r.PublicModel), Streaming: true}, nil
	}
	defer resp.Body.Close()
	outBody, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	translatedBody, err := translateGeminiResponsesResponse(outBody, r.PublicModel)
	if err != nil {
		return nil, fmt.Errorf("translate response: %w", err)
	}
	return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translatedBody}, nil
}

func translateOpenAIToGemini(body []byte) ([]byte, bool, error) {
	var req openAIChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false, err
	}
	out := geminiRequest{}
	if req.MaxTokens > 0 || req.Temperature != nil || req.TopP != nil {
		out.GenerationConfig = &geminiGenConfig{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
			TopP:            req.TopP,
		}
	}
	if out.GenerationConfig == nil && req.MaxTokens <= 0 {
		out.GenerationConfig = &geminiGenConfig{MaxOutputTokens: defaultMaxTokens}
	} else if out.GenerationConfig != nil && out.GenerationConfig.MaxOutputTokens <= 0 {
		out.GenerationConfig.MaxOutputTokens = defaultMaxTokens
	}

	var systemParts []geminiPart
	for _, msg := range req.Messages {
		parts, joined, err := parseOpenAITextParts(msg.Content)
		if err != nil {
			return nil, false, fmt.Errorf("message role %q: %w", msg.Role, err)
		}
		switch msg.Role {
		case "system":
			if joined != "" {
				systemParts = append(systemParts, parts...)
			}
		case "user":
			out.Contents = append(out.Contents, geminiContent{Role: "user", Parts: parts})
		case "assistant":
			out.Contents = append(out.Contents, geminiContent{Role: "model", Parts: parts})
		default:
			return nil, false, fmt.Errorf("unsupported message role %q", msg.Role)
		}
	}
	if len(systemParts) > 0 {
		out.SystemInstruction = &geminiContent{Parts: systemParts}
	}
	translated, err := json.Marshal(out)
	if err != nil {
		return nil, false, err
	}
	return translated, req.Stream, nil
}

func translateOpenAIEmbeddingsToGemini(body []byte) ([]byte, bool, error) {
	var req openAIEmbeddingRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false, err
	}
	inputs, err := parseOpenAIEmbeddingInputs(req.Input)
	if err != nil {
		return nil, false, err
	}
	if len(inputs) == 1 {
		payload := geminiEmbedRequest{
			Content: geminiContent{Parts: []geminiPart{{Text: inputs[0]}}},
		}
		if req.Dimensions > 0 {
			payload.OutputDimensionality = req.Dimensions
		}
		encoded, err := json.Marshal(payload)
		return encoded, false, err
	}
	payload := geminiBatchEmbedRequest{Requests: make([]geminiEmbedRequest, 0, len(inputs))}
	for _, input := range inputs {
		reqItem := geminiEmbedRequest{
			Content: geminiContent{Parts: []geminiPart{{Text: input}}},
		}
		if req.Dimensions > 0 {
			reqItem.OutputDimensionality = req.Dimensions
		}
		payload.Requests = append(payload.Requests, reqItem)
	}
	encoded, err := json.Marshal(payload)
	return encoded, true, err
}

func parseOpenAIEmbeddingInputs(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("input is required")
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var multi []string
	if err := json.Unmarshal(raw, &multi); err == nil {
		if len(multi) == 0 {
			return nil, fmt.Errorf("input array must not be empty")
		}
		return multi, nil
	}
	return nil, fmt.Errorf("input must be a string or array of strings")
}

func parseOpenAITextParts(raw json.RawMessage) ([]geminiPart, string, error) {
	if len(raw) == 0 {
		return nil, "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []geminiPart{{Text: text}}, text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, "", fmt.Errorf("unsupported content shape")
	}
	out := make([]geminiPart, 0, len(parts))
	joined := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" {
			return nil, "", fmt.Errorf("unsupported content part type %q", part.Type)
		}
		out = append(out, geminiPart{Text: part.Text})
		joined = append(joined, part.Text)
	}
	return out, strings.Join(joined, ""), nil
}

func translateGeminiResponse(body []byte, publicModel string) ([]byte, error) {
	var resp geminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	content := ""
	finish := ""
	if len(resp.Candidates) > 0 {
		content = joinGeminiText(resp.Candidates[0].Content.Parts)
		finish = mapGeminiFinishReason(resp.Candidates[0].FinishReason)
	}
	out := openAIResponse{
		ID:      "chatcmpl-gemini",
		Object:  "chat.completion",
		Created: 0,
		Model:   publicModel,
		Choices: []openAIChoice{{
			Index: 0,
			Message: openAIMessageOut{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: finish,
		}},
		Usage: openAIUsage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		},
	}
	return json.Marshal(out)
}

func translateGeminiResponsesResponse(body []byte, publicModel string) ([]byte, error) {
	var resp geminiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	text := ""
	if len(resp.Candidates) > 0 {
		text = joinGeminiText(resp.Candidates[0].Content.Parts)
	}
	usage := openAIUsage{
		PromptTokens:     resp.UsageMetadata.PromptTokenCount,
		CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      resp.UsageMetadata.TotalTokenCount,
	}
	return buildResponsesOutput("resp_gemini", publicModel, text, usage)
}

func translateGeminiEmbeddingResponse(body []byte, publicModel string, batch bool) ([]byte, error) {
	out := openAIEmbeddingResponse{
		Object: "list",
		Model:  publicModel,
	}
	if batch {
		var resp geminiBatchEmbedResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		out.Data = make([]openAIEmbeddingDatum, 0, len(resp.Embeddings))
		for i, emb := range resp.Embeddings {
			out.Data = append(out.Data, openAIEmbeddingDatum{
				Object:    "embedding",
				Embedding: emb.Values,
				Index:     i,
			})
		}
		out.Usage = openAIEmbeddingUsage{
			PromptTokens: resp.UsageMetadata.PromptTokenCount,
			TotalTokens:  totalEmbeddingTokens(resp.UsageMetadata),
		}
		return json.Marshal(out)
	}
	var resp geminiEmbedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	out.Data = []openAIEmbeddingDatum{{
		Object:    "embedding",
		Embedding: resp.Embedding.Values,
		Index:     0,
	}}
	out.Usage = openAIEmbeddingUsage{
		PromptTokens: resp.UsageMetadata.PromptTokenCount,
		TotalTokens:  totalEmbeddingTokens(resp.UsageMetadata),
	}
	return json.Marshal(out)
}

func totalEmbeddingTokens(usage geminiUsageMetadata) int {
	if usage.TotalTokenCount > 0 {
		return usage.TotalTokenCount
	}
	return usage.PromptTokenCount
}

func translateGeminiError(body []byte) []byte {
	var resp struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Error.Message == "" {
		return []byte(`{"error":{"type":"upstream_error","message":"gemini upstream error"}}`)
	}
	out := struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	out.Error.Type = strings.ToLower(resp.Error.Status)
	if out.Error.Type == "" {
		out.Error.Type = "upstream_error"
	}
	out.Error.Message = resp.Error.Message
	encoded, err := json.Marshal(out)
	if err != nil {
		return []byte(`{"error":{"type":"upstream_error","message":"gemini upstream error"}}`)
	}
	return encoded
}

func translateGeminiStream(src io.ReadCloser, publicModel string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer src.Close()
		defer pw.Close()

		reader := bufio.NewReader(src)
		sentRole := false
		sentDone := false
		for {
			payload, err := readSSEPayload(reader)
			if err != nil && err != io.EOF {
				_ = pw.CloseWithError(err)
				return
			}
			if payload != "" {
				wrote, done, processErr := processGeminiPayload(pw, payload, publicModel, &sentRole)
				if processErr != nil {
					_ = pw.CloseWithError(processErr)
					return
				}
				if wrote && done {
					sentDone = true
				}
			}
			if err == io.EOF {
				if !sentDone {
					_, _ = io.WriteString(pw, "data: [DONE]\n\n")
				}
				return
			}
		}
	}()
	return pr
}

func translateGeminiResponsesStream(src io.ReadCloser, publicModel string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer src.Close()
		defer pw.Close()

		reader := bufio.NewReader(src)
		state := newResponsesStreamState(publicModel, "resp_gemini")
		for {
			payload, err := readSSEPayload(reader)
			if err != nil && err != io.EOF {
				_ = pw.CloseWithError(err)
				return
			}
			if payload != "" {
				done, processErr := processGeminiResponsesPayload(pw, payload, state)
				if processErr != nil {
					_ = pw.CloseWithError(processErr)
					return
				}
				if done {
					return
				}
			}
			if err == io.EOF {
				if !state.Completed {
					if err := writeResponsesCompleted(pw, state); err != nil {
						_ = pw.CloseWithError(err)
					}
				}
				return
			}
		}
	}()
	return pr
}

func readSSEPayload(reader *bufio.Reader) (string, error) {
	var dataLines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if len(dataLines) > 0 {
				return strings.Join(dataLines, "\n"), io.EOF
			}
			return "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if len(dataLines) > 0 {
				return strings.Join(dataLines, "\n"), nil
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
		if err == io.EOF {
			if len(dataLines) > 0 {
				return strings.Join(dataLines, "\n"), io.EOF
			}
			return "", io.EOF
		}
	}
}

func processGeminiPayload(w io.Writer, payload, publicModel string, sentRole *bool) (bool, bool, error) {
	var resp geminiResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		return false, false, err
	}
	if len(resp.Candidates) == 0 {
		return false, false, nil
	}
	candidate := resp.Candidates[0]
	wrote := false
	if !*sentRole {
		if err := writeOpenAIChunk(w, openAIChunk{
			ID:      "chatcmpl-gemini",
			Object:  "chat.completion.chunk",
			Created: 0,
			Model:   publicModel,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{Role: "assistant"},
			}},
		}); err != nil {
			return false, false, err
		}
		*sentRole = true
		wrote = true
	}
	text := joinGeminiText(candidate.Content.Parts)
	if text != "" {
		if err := writeOpenAIChunk(w, openAIChunk{
			ID:      "chatcmpl-gemini",
			Object:  "chat.completion.chunk",
			Created: 0,
			Model:   publicModel,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{Content: text},
			}},
		}); err != nil {
			return wrote, false, err
		}
		wrote = true
	}
	finish := mapGeminiFinishReason(candidate.FinishReason)
	if finish != "" {
		if err := writeOpenAIChunk(w, openAIChunk{
			ID:      "chatcmpl-gemini",
			Object:  "chat.completion.chunk",
			Created: 0,
			Model:   publicModel,
			Choices: []openAIChunkChoice{{
				Index:        0,
				Delta:        openAIChunkDelta{},
				FinishReason: stringPtr(finish),
			}},
		}); err != nil {
			return wrote, false, err
		}
		if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
			return true, false, err
		}
		return true, true, nil
	}
	return wrote, false, nil
}

func processGeminiResponsesPayload(w io.Writer, payload string, state *responsesStreamState) (bool, error) {
	var resp geminiResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		return false, err
	}
	state.setUsage(openAIUsage{
		PromptTokens:     resp.UsageMetadata.PromptTokenCount,
		CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      resp.UsageMetadata.TotalTokenCount,
	})
	if err := writeResponsesCreated(w, state); err != nil {
		return false, err
	}
	if len(resp.Candidates) == 0 {
		return false, nil
	}
	candidate := resp.Candidates[0]
	text := joinGeminiText(candidate.Content.Parts)
	if text != "" {
		if err := writeResponsesDelta(w, state, text); err != nil {
			return false, err
		}
	}
	if mapGeminiFinishReason(candidate.FinishReason) == "" {
		return false, nil
	}
	return true, writeResponsesCompleted(w, state)
}

func joinGeminiText(parts []geminiPart) string {
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(part.Text)
	}
	return b.String()
}

func mapGeminiFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	default:
		return strings.ToLower(reason)
	}
}
