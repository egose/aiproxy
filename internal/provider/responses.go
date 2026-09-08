package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func translateOpenAIResponsesInput(body []byte) (openAIChatRequest, error) {
	if err := rejectUnsupportedTopLevelFields(body, openAIResponsesRequestFields); err != nil {
		return openAIChatRequest{}, err
	}
	var req openAIResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return openAIChatRequest{}, err
	}
	chatReq := openAIChatRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	if req.Instructions != "" {
		chatReq.Messages = append(chatReq.Messages, openAIMessage{Role: "system", Content: mustMarshalJSON(req.Instructions)})
	}
	msgs, err := parseOpenAIResponsesMessages(req.Input)
	if err != nil {
		return openAIChatRequest{}, err
	}
	chatReq.Messages = append(chatReq.Messages, msgs...)
	return chatReq, nil
}

func parseOpenAIResponsesMessages(raw json.RawMessage) ([]openAIMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("input is required")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []openAIMessage{{Role: "user", Content: mustMarshalJSON(text)}}, nil
	}
	var items []struct {
		Type    string          `json:"type,omitempty"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("input must be a string or array of message objects")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("input array must not be empty")
	}
	out := make([]openAIMessage, 0, len(items))
	for _, item := range items {
		if item.Type != "" && item.Type != "message" {
			return nil, fmt.Errorf("unsupported input item type %q", item.Type)
		}
		normalized, err := normalizeResponsesContent(item.Content)
		if err != nil {
			return nil, err
		}
		out = append(out, openAIMessage{Role: item.Role, Content: normalized})
	}
	return out, nil
}

func normalizeResponsesContent(raw json.RawMessage) (json.RawMessage, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return mustMarshalJSON(text), nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("unsupported content shape")
	}
	normalized := make([]map[string]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text", "input_text", "output_text":
			normalized = append(normalized, map[string]string{"type": "text", "text": part.Text})
		default:
			return nil, fmt.Errorf("unsupported content part type %q", part.Type)
		}
	}
	return mustMarshalJSON(normalized), nil
}

type responsesStreamState struct {
	ResponseID   string
	ItemID       string
	PublicModel  string
	Text         strings.Builder
	Usage        openAIResponsesUsage
	Created      bool
	OutputOpened bool
	Completed    bool
}

// Retained-text budget and overflow contract (STREAM-03 release note).
//
// Translated Responses streams accumulate generated text in
// responsesStreamState so the terminal events can replay it:
// response.output_text.done carries the full text once and
// response.completed embeds it again inside response.output. Per-event SSE
// framing limits (maxSSELineBytes/maxSSEEventBytes) reset between events, so
// without an aggregate bound many individually valid deltas could grow
// retained state indefinitely.
//
// Budget: maxResponsesRetainedTextBytes (1 MiB) of retained translated text
// per stream. 1 MiB matches the existing per-line SSE bound, so the aggregate
// is on the order of one maximum line; it covers large realistic model
// outputs (~250k tokens of text) while peak completion serialization stays
// small: retained 1x plus the two terminal serializations (~3x transient,
// ~3 MiB), an order of magnitude below the 32 MiB upstream body cap. The cap
// is a fixed internal value, not operator configuration: no concrete
// operator need justifies tuning it. It is a package var (not const) only so
// tests can substitute a small deterministic budget; production code never
// changes it.
//
// Overflow contract: the delta that would exceed the budget is neither
// retained nor emitted downstream; translation fails with
// ErrResponsesOutputOverflow through the existing pipe error boundary
// (reusing the STREAM-02 propagation path), upstream is closed, and no
// response.completed or data: [DONE] terminal is emitted. Partial text
// already streamed as deltas stays downstream; only the successful terminal
// is withheld. Opaque pass-through streams (openai/openai-compatible) are
// intentionally not capped: they retain no aggregate text, only a bounded
// per-event usage observer.
//
// Non-streaming translated Responses JSON (translateAnthropicResponsesResponse,
// translateGeminiResponsesResponse) does not use this accumulator; its input
// remains bounded by the existing maxUpstreamBodyBytes body cap.
var maxResponsesRetainedTextBytes = 1 << 20

type ErrResponsesOutputOverflow struct {
	Limit int
}

func (e ErrResponsesOutputOverflow) Error() string {
	return fmt.Sprintf("translated responses output exceeds %d bytes", e.Limit)
}

func newResponsesStreamState(publicModel, fallbackID string) *responsesStreamState {
	state := &responsesStreamState{PublicModel: publicModel}
	state.ensureIDs(fallbackID)
	return state
}

func (s *responsesStreamState) ensureIDs(fallbackID string) {
	if s.ResponseID == "" {
		if fallbackID == "" {
			fallbackID = "resp_translated"
		}
		s.ResponseID = fallbackID
	}
	if s.ItemID == "" {
		s.ItemID = s.ResponseID + "_msg"
	}
}

func (s *responsesStreamState) setUsage(usage openAIResponsesUsage) {
	if usage.InputTokens > 0 {
		s.Usage.InputTokens = usage.InputTokens
	}
	if usage.OutputTokens > 0 {
		s.Usage.OutputTokens = usage.OutputTokens
	}
	if usage.TotalTokens > 0 {
		s.Usage.TotalTokens = usage.TotalTokens
	}
	if total := s.Usage.InputTokens + s.Usage.OutputTokens; total > s.Usage.TotalTokens {
		s.Usage.TotalTokens = total
	}
}

func reconcileResponsesUsage(input, output, total int) openAIResponsesUsage {
	out := openAIResponsesUsage{InputTokens: input, OutputTokens: output, TotalTokens: total}
	if out.TotalTokens == 0 || input+output > out.TotalTokens {
		out.TotalTokens = input + output
	}
	return out
}

func (s *responsesStreamState) appendText(delta string) error {
	if s.Text.Len()+len(delta) > maxResponsesRetainedTextBytes {
		return ErrResponsesOutputOverflow{Limit: maxResponsesRetainedTextBytes}
	}
	s.Text.WriteString(delta)
	return nil
}

func (s *responsesStreamState) text() string {
	return s.Text.String()
}

func buildResponsesObject(id, publicModel, text, status string, usage openAIResponsesUsage) openAIResponsesResponse {
	out := openAIResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  publicModel,
		Status: status,
	}
	if text != "" || status == "completed" {
		out.Output = []openAIResponsesOutputItem{{
			ID:   id + "_msg",
			Type: "message",
			Role: "assistant",
			Content: []openAIResponsesContentItem{{
				Type:        "output_text",
				Text:        text,
				Annotations: []any{},
			}},
		}}
	}
	if usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.TotalTokens > 0 {
		out.Usage = &usage
	}
	return out
}

func buildResponsesOutput(id, publicModel, text string, usage openAIResponsesUsage) ([]byte, error) {
	out := buildResponsesObject(id, publicModel, text, "completed", usage)
	return json.Marshal(out)
}

func writeResponsesEvent(w io.Writer, event any) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: "); err != nil {
		return err
	}
	if _, err := w.Write(encoded); err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n\n")
	return err
}

func writeResponsesCreated(w io.Writer, state *responsesStreamState) error {
	if state.Created {
		return nil
	}
	state.ensureIDs("")
	if err := writeResponsesEvent(w, struct {
		Type     string                  `json:"type"`
		Response openAIResponsesResponse `json:"response"`
	}{
		Type:     "response.created",
		Response: buildResponsesObject(state.ResponseID, state.PublicModel, "", "in_progress", openAIResponsesUsage{}),
	}); err != nil {
		return err
	}
	state.Created = true
	return nil
}

func writeResponsesOutputItemAdded(w io.Writer, state *responsesStreamState) error {
	if state.OutputOpened {
		return nil
	}
	if err := writeResponsesCreated(w, state); err != nil {
		return err
	}
	if err := writeResponsesEvent(w, struct {
		Type        string                    `json:"type"`
		OutputIndex int                       `json:"output_index"`
		Item        openAIResponsesOutputItem `json:"item"`
	}{
		Type:        "response.output_item.added",
		OutputIndex: 0,
		Item: openAIResponsesOutputItem{
			ID:      state.ItemID,
			Type:    "message",
			Role:    "assistant",
			Content: []openAIResponsesContentItem{},
		},
	}); err != nil {
		return err
	}
	state.OutputOpened = true
	return nil
}

func writeResponsesDelta(w io.Writer, state *responsesStreamState, delta string) error {
	if delta == "" {
		return nil
	}
	if state.Text.Len()+len(delta) > maxResponsesRetainedTextBytes {
		return ErrResponsesOutputOverflow{Limit: maxResponsesRetainedTextBytes}
	}
	if err := writeResponsesOutputItemAdded(w, state); err != nil {
		return err
	}
	if err := state.appendText(delta); err != nil {
		return err
	}
	return writeResponsesEvent(w, struct {
		Type         string `json:"type"`
		ItemID       string `json:"item_id"`
		OutputIndex  int    `json:"output_index"`
		ContentIndex int    `json:"content_index"`
		Delta        string `json:"delta"`
	}{
		Type:         "response.output_text.delta",
		ItemID:       state.ItemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Delta:        delta,
	})
}

func writeResponsesCompleted(w io.Writer, state *responsesStreamState) error {
	if state.Completed {
		return nil
	}
	if err := writeResponsesOutputItemAdded(w, state); err != nil {
		return err
	}
	if err := writeResponsesEvent(w, struct {
		Type         string `json:"type"`
		ItemID       string `json:"item_id"`
		OutputIndex  int    `json:"output_index"`
		ContentIndex int    `json:"content_index"`
		Text         string `json:"text"`
	}{
		Type:         "response.output_text.done",
		ItemID:       state.ItemID,
		OutputIndex:  0,
		ContentIndex: 0,
		Text:         state.text(),
	}); err != nil {
		return err
	}
	if err := writeResponsesEvent(w, struct {
		Type     string                  `json:"type"`
		Response openAIResponsesResponse `json:"response"`
	}{
		Type:     "response.completed",
		Response: buildResponsesObject(state.ResponseID, state.PublicModel, state.text(), "completed", state.Usage),
	}); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
		return err
	}
	state.Completed = true
	return nil
}

func mustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
