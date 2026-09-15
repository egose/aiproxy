package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/provider"
)

const (
	guardrailBlockType       = "secret_blocked"
	guardrailBlockMessage    = "request blocked: suspected secret in request content"
	guardrailGapType         = "scan_incomplete"
	guardrailGapMessage      = "request blocked: secret scan could not complete"
	guardrailReasonUnparsed  = "unparsable"
	guardrailReasonMultipart = "multipart"
)

func guardrailCovered(op provider.Operation) bool {
	return op == provider.OpChatCompletions || op == provider.OpResponses
}

type guardrailTextCollector struct {
	texts      []string
	total      int
	maxStrings int
	maxBytes   int
	reason     string
}

func (c *guardrailTextCollector) add(s string) {
	if c.reason != "" {
		return
	}
	if len(c.texts) >= c.maxStrings {
		c.reason = guardrails.ReasonTooManyStrings
		return
	}
	if c.total+len(s) > c.maxBytes {
		c.reason = guardrails.ReasonOversize
		return
	}
	c.texts = append(c.texts, s)
	c.total += len(s)
}

func extractGuardrailTexts(contentType string, op provider.Operation, body []byte, maxStrings, maxBytes int) ([]string, string) {
	if strings.HasPrefix(contentType, "multipart/form-data;") {
		return nil, guardrailReasonMultipart
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, guardrailReasonUnparsed
	}
	root, ok := doc.(map[string]any)
	if !ok {
		return nil, guardrailReasonUnparsed
	}
	c := &guardrailTextCollector{maxStrings: maxStrings, maxBytes: maxBytes}
	switch op {
	case provider.OpChatCompletions:
		collectChatTexts(c, root)
	case provider.OpResponses:
		collectResponsesTexts(c, root)
	default:
		return nil, guardrailReasonUnparsed
	}
	if c.reason != "" {
		return nil, c.reason
	}
	return c.texts, ""
}

func collectChatTexts(c *guardrailTextCollector, root map[string]any) {
	messages, ok := root["messages"].([]any)
	if !ok {
		return
	}
	for _, item := range messages {
		msg, ok := item.(map[string]any)
		if !ok {
			continue
		}
		collectContentValue(c, msg["content"])
		if c.reason != "" {
			return
		}
		calls, ok := msg["tool_calls"].([]any)
		if !ok {
			continue
		}
		for _, call := range calls {
			callMap, ok := call.(map[string]any)
			if !ok {
				continue
			}
			fn, ok := callMap["function"].(map[string]any)
			if !ok {
				continue
			}
			args, ok := fn["arguments"].(string)
			if !ok {
				continue
			}
			c.add(args)
			collectJSONStringLeaves(c, args)
			if c.reason != "" {
				return
			}
		}
	}
}

func collectResponsesTexts(c *guardrailTextCollector, root map[string]any) {
	if instructions, ok := root["instructions"].(string); ok {
		c.add(instructions)
		if c.reason != "" {
			return
		}
	}
	collectInputValue(c, root["input"])
}

func collectInputValue(c *guardrailTextCollector, input any) {
	switch tv := input.(type) {
	case string:
		c.add(tv)
	case []any:
		for _, item := range tv {
			switch iv := item.(type) {
			case string:
				c.add(iv)
			case map[string]any:
				collectContentValue(c, iv["content"])
				if args, ok := iv["arguments"].(string); ok {
					c.add(args)
					collectJSONStringLeaves(c, args)
				}
			}
			if c.reason != "" {
				return
			}
		}
	}
}

func collectContentValue(c *guardrailTextCollector, content any) {
	switch tv := content.(type) {
	case string:
		c.add(tv)
	case []any:
		for _, part := range tv {
			partMap, ok := part.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := partMap["text"].(string); ok {
				c.add(text)
			} else if text, ok := partMap["input_text"].(string); ok {
				c.add(text)
			}
			if c.reason != "" {
				return
			}
		}
	}
}

func collectJSONStringLeaves(c *guardrailTextCollector, s string) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 2 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return
	}
	var doc any
	if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
		return
	}
	collectStringLeaves(c, doc)
}

func collectStringLeaves(c *guardrailTextCollector, v any) {
	switch tv := v.(type) {
	case string:
		c.add(tv)
	case []any:
		for _, item := range tv {
			collectStringLeaves(c, item)
			if c.reason != "" {
				return
			}
		}
	case map[string]any:
		for _, item := range tv {
			collectStringLeaves(c, item)
			if c.reason != "" {
				return
			}
		}
	}
}

func (h *Handler) checkGuardrails(deps Dependencies, w http.ResponseWriter, r *http.Request, op provider.Operation, body []byte, scanner *guardrails.Scanner, publicModel string, logger *slog.Logger) bool {
	policy := scanner.Policy()
	mode := string(policy.Mode)
	texts, reason := extractGuardrailTexts(r.Header.Get("Content-Type"), op, body, policy.MaxStrings, policy.MaxTextBytes)
	var res guardrails.Result
	var captured []guardrails.CapturedFinding
	if reason != "" {
		res = guardrails.Result{Outcome: guardrails.OutcomeIncomplete, Reason: reason}
	} else if deps.Quarantine != nil {
		res, captured = scanner.ScanCapture(r.Context(), texts)
	} else {
		res = scanner.Scan(r.Context(), texts)
	}
	outcome := string(res.Outcome)
	blockID := ""
	if deps.Metrics != nil {
		deps.Metrics.RecordGuardrailScan(op.String(), mode, outcome)
	}
	safeAttrs := func() []any {
		attrs := []any{"mode", mode, "outcome", outcome, "finding_count", res.FindingCount}
		if len(res.RuleIDs) > 0 {
			attrs = append(attrs, "rule_ids", strings.Join(res.RuleIDs, ","))
		}
		if res.Reason != "" {
			attrs = append(attrs, "reason", res.Reason)
		}
		if blockID != "" {
			attrs = append(attrs, "block_id", blockID)
		}
		return attrs
	}
	switch res.Outcome {
	case guardrails.OutcomeClean:
		return false
	case guardrails.OutcomeFlagged, guardrails.OutcomeIncomplete:
		if policy.Mode == guardrails.ModeAudit {
			logger.Info("guardrail scan did not block request", safeAttrs()...)
			return false
		}
		if res.Outcome == guardrails.OutcomeFlagged && deps.Quarantine != nil && len(captured) > 0 {
			if id, err := guardrails.MintBlockID(); err == nil {
				blockID = id
				deps.Quarantine.Store(blockID, guardrails.Capture{
					Operation:   op.String(),
					PublicModel: publicModel,
					RuleIDs:     append([]string(nil), res.RuleIDs...),
					Findings:    captured,
				})
			}
		}
		logger.Warn("guardrail blocked request", safeAttrs()...)
		if res.Outcome == guardrails.OutcomeFlagged {
			h.writeRequestErrorWithBlock(deps.Metrics, w, r, http.StatusBadRequest, guardrailBlockType, guardrailBlockMessage, blockID)
		} else {
			h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, guardrailGapType, guardrailGapMessage)
		}
		return true
	default:
		return false
	}
}
