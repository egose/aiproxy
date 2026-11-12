package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/provider"
)

const (
	guardrailBlockType         = "secret_blocked"
	guardrailBlockMessage      = "request blocked: suspected secret in request content"
	guardrailGapType           = "scan_incomplete"
	guardrailGapMessage        = "request blocked: secret scan could not complete"
	guardrailReasonUnparsed    = "unparsable"
	guardrailReasonMultipart   = "multipart"
	maxGuardrailRedactionDepth = 128
)

func guardrailCovered(op provider.Operation) bool {
	return op == provider.OpChatCompletions || op == provider.OpResponses || op == provider.OpMessages
}

type guardrailLogState struct {
	incomplete bool
	redactions map[string]string
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
	case provider.OpMessages:
		collectMessagesTexts(c, root)
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

func collectMessagesTexts(c *guardrailTextCollector, root map[string]any) {
	if system, ok := root["system"]; ok {
		collectContentValue(c, system)
		if c.reason != "" {
			return
		}
	}
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
		collectMessagesToolBlocks(c, msg["content"])
		if c.reason != "" {
			return
		}
	}
}

func collectMessagesToolBlocks(c *guardrailTextCollector, content any) {
	parts, ok := content.([]any)
	if !ok {
		return
	}
	for _, part := range parts {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch partMap["type"] {
		case "tool_use":
			if input, ok := partMap["input"]; ok {
				if raw, err := json.Marshal(input); err == nil {
					c.add(string(raw))
					collectJSONStringLeaves(c, string(raw))
				}
			}
		case "tool_result":
			collectContentValue(c, partMap["content"])
		}
		if c.reason != "" {
			return
		}
	}
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

func (h *Handler) checkGuardrails(deps Dependencies, w http.ResponseWriter, r *http.Request, op provider.Operation, body []byte, scanner *guardrails.Scanner, publicModel string, logger *slog.Logger, logRedactions bool) (bool, []byte, *guardrailLogState) {
	policy := scanner.Policy()
	mode := string(policy.Mode)
	texts, reason := extractGuardrailTexts(r.Header.Get("Content-Type"), op, body, policy.MaxStrings, policy.MaxTextBytes)
	var res guardrails.Result
	var captured []guardrails.CapturedFinding
	var secrets []string
	captureComplete := true
	if reason != "" {
		res = guardrails.Result{Outcome: guardrails.OutcomeIncomplete, Reason: reason}
	} else if deps.Quarantine != nil || deps.Exceptions != nil || logRedactions {
		res, captured, secrets, captureComplete = scanner.ScanCapture(r.Context(), texts)
	} else {
		res = scanner.Scan(r.Context(), texts)
	}
	var logState *guardrailLogState
	if logRedactions {
		logState = &guardrailLogState{}
		switch {
		case reason != "" || res.Outcome == guardrails.OutcomeIncomplete:
			logState.incomplete = true
		case res.Outcome == guardrails.OutcomeFlagged && (len(captured) == 0 || len(secrets) == 0 || !captureComplete):
			logState.incomplete = true
		case res.Outcome == guardrails.OutcomeFlagged:
			logState.redactions = guardrailSecretRedactions(secrets, deps.Exceptions)
		}
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
		return false, body, logState
	case guardrails.OutcomeFlagged, guardrails.OutcomeIncomplete:
		if res.Outcome == guardrails.OutcomeFlagged && len(captured) > 0 && captureComplete {
			if blocking, redactions := partitionGuardrailFindings(deps.Exceptions, captured, secrets); len(blocking) == 0 {
				out := body
				extra := []any{}
				if len(redactions) > 0 {
					out = rewriteGuardrailBody(body, redactions)
					if out == nil {
						if logState != nil {
							logState.incomplete = true
						}
						if policy.Mode == guardrails.ModeAudit {
							logger.Info("guardrail redaction could not complete; audit request unchanged", safeAttrs()...)
							return false, body, logState
						}
						logger.Warn("guardrail redaction could not complete", safeAttrs()...)
						h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, guardrailGapType, guardrailGapMessage)
						return true, body, logState
					}
					extra = append(extra, "redacted", len(redactions))
				} else {
					extra = append(extra, "allowed", len(captured))
				}
				logger.Info("guardrail scan cleared by exceptions", append(safeAttrs(), extra...)...)
				return false, out, logState
			} else if policy.Mode == guardrails.ModeAudit {
				logger.Info("guardrail scan did not block request", safeAttrs()...)
				return false, body, logState
			} else {
				captured = blocking
				res.RuleIDs = guardrailRuleIDs(blocking)
				res.FindingCount = len(blocking)
			}
		} else if policy.Mode == guardrails.ModeAudit {
			logger.Info("guardrail scan did not block request", safeAttrs()...)
			return false, body, logState
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
		return true, body, logState
	default:
		return false, body, logState
	}
}

func guardrailRedactPlaceholder(exceptions *guardrails.Exceptions) string {
	placeholder := ""
	if exceptions != nil {
		placeholder = exceptions.Placeholder()
	}
	if placeholder == "" {
		placeholder = guardrails.DefaultRedactPlaceholder
	}
	return placeholder
}

func guardrailSecretRedactions(secrets []string, exceptions *guardrails.Exceptions) map[string]string {
	placeholder := guardrailRedactPlaceholder(exceptions)
	out := make(map[string]string, len(secrets))
	for _, value := range secrets {
		if value == "" || value == placeholder {
			continue
		}
		out[value] = placeholder
	}
	return out
}

func partitionGuardrailFindings(exceptions *guardrails.Exceptions, captured []guardrails.CapturedFinding, secrets []string) ([]guardrails.CapturedFinding, map[string]string) {
	blocking := make([]guardrails.CapturedFinding, 0, len(captured))
	redactions := map[string]string{}
	if exceptions == nil {
		return append(blocking, captured...), redactions
	}
	placeholder := guardrailRedactPlaceholder(exceptions)
	fullSecrets := make(map[string]string, len(secrets))
	for _, value := range secrets {
		fullSecrets[guardrails.Fingerprint(value)] = value
	}
	for _, f := range captured {
		sha := f.SecretSHA
		if sha == "" && f.Secret != "" {
			sha = guardrails.Fingerprint(f.Secret)
		}
		entry, ok := exceptions.Lookup(sha)
		if !ok || sha == "" {
			blocking = append(blocking, f)
			continue
		}
		switch entry.Action {
		case guardrails.ExceptionActionAllow:
			continue
		case guardrails.ExceptionActionRedact:
			value, ok := fullSecrets[sha]
			if !ok || value == "" {
				blocking = append(blocking, f)
			} else if value != placeholder {
				redactions[value] = placeholder
			}
		default:
			blocking = append(blocking, f)
		}
	}
	return blocking, redactions
}

func rewriteGuardrailBody(body []byte, redactions map[string]string) []byte {
	keys := make([]string, 0, len(redactions))
	for value := range redactions {
		if value != "" {
			keys = append(keys, value)
		}
	}
	if len(keys) == 0 {
		return body
	}
	value, ok := decodeGuardrailJSON(body)
	if !ok {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	pairs := make([]string, 0, len(keys)*2)
	remaining := make(map[string]struct{}, len(keys))
	for _, value := range keys {
		pairs = append(pairs, value, redactions[value])
		remaining[value] = struct{}{}
	}
	redactor := guardrailBodyRedactor{replacer: strings.NewReplacer(pairs...), remaining: remaining}
	out, changed := redactor.rewrite(value, 0)
	if redactor.incomplete || len(redactor.remaining) > 0 {
		return nil
	}
	if !changed {
		return body
	}
	data, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return data
}

func decodeGuardrailJSON(body []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || len(bytes.TrimSpace(body[decoder.InputOffset():])) != 0 {
		return nil, false
	}
	return value, true
}

type guardrailBodyRedactor struct {
	replacer   *strings.Replacer
	remaining  map[string]struct{}
	incomplete bool
}

func (r *guardrailBodyRedactor) replace(text string) string {
	for value := range r.remaining {
		if strings.Contains(text, value) {
			delete(r.remaining, value)
		}
	}
	return r.replacer.Replace(text)
}

func (r *guardrailBodyRedactor) rewrite(value any, depth int) (any, bool) {
	if depth > maxGuardrailRedactionDepth {
		r.incomplete = true
		return value, false
	}
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if len(trimmed) >= 2 && (trimmed[0] == '{' || trimmed[0] == '[') {
			if nested, ok := decodeGuardrailJSON([]byte(trimmed)); ok {
				if out, changed := r.rewrite(nested, depth+1); changed {
					data, err := json.Marshal(out)
					if err != nil {
						r.incomplete = true
						return value, false
					}
					return string(data), true
				}
			}
		}
		out := r.replace(v)
		return out, out != v
	case []any:
		changed := false
		for i, item := range v {
			out, replaced := r.rewrite(item, depth+1)
			v[i] = out
			changed = changed || replaced
		}
		return v, changed
	case map[string]any:
		out := make(map[string]any, len(v))
		changed := false
		for key, item := range v {
			nextKey := r.replace(key)
			next, replaced := r.rewrite(item, depth+1)
			if _, exists := out[nextKey]; exists {
				r.incomplete = true
			}
			out[nextKey] = next
			changed = changed || replaced || nextKey != key
		}
		return out, changed
	default:
		return value, false
	}
}

func guardrailRuleIDs(blocking []guardrails.CapturedFinding) []string {
	seen := make(map[string]struct{}, len(blocking))
	for _, f := range blocking {
		if f.RuleID == "" {
			continue
		}
		seen[f.RuleID] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	if len(out) > guardrails.MaxReportedRuleIDs {
		out = out[:guardrails.MaxReportedRuleIDs]
	}
	return out
}
