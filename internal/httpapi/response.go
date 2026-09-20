package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		BlockID string `json:"block_id,omitempty"`
	} `json:"error"`
}

type modelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelCard `json:"data"`
}

type billingUsageResponse struct {
	Object string              `json:"object"`
	Data   []billingUsageEntry `json:"data"`
}

type billingUsageEntry struct {
	accounting.Summary
	EstimatedCostUSD *float64 `json:"estimated_cost_usd,omitempty"`
}

func billingCost(s accounting.Summary, prices map[string]*config.ModelPricing, aliases []config.Alias, upstream []accounting.UpstreamSummary) (float64, bool) {
	if len(prices) == 0 {
		return 0, false
	}
	if p, ok := prices[s.Model]; ok && p != nil {
		return p.Cost(s.PromptTokens, s.CompletionTokens, s.CachedTokens, s.CacheCreationTokens, s.CacheReadTokens)
	}
	return billingAliasCost(s, prices, aliases, upstream)
}

func billingAliasCost(s accounting.Summary, prices map[string]*config.ModelPricing, aliases []config.Alias, upstream []accounting.UpstreamSummary) (float64, bool) {
	entries, ok := aliasCostEntries(s, prices, aliases, upstream)
	if !ok {
		return 0, false
	}
	if upstreamTokenTotal(entries) > 0 {
		var total float64
		priced := false
		for _, e := range entries {
			if !e.matched {
				continue
			}
			cost, ok := e.price.Cost(e.prompt, e.completion, e.cached, e.write, e.read)
			if !ok {
				continue
			}
			total += cost
			priced = true
		}
		if !priced {
			return 0, false
		}
		return total, true
	}
	return splitAliasCost(s, entries)
}

type aliasCostEntry struct {
	price                                          *config.ModelPricing
	prompt, completion, cached, write, read, count int64
	matched                                        bool
}

func aliasCostEntries(s accounting.Summary, prices map[string]*config.ModelPricing, aliases []config.Alias, upstream []accounting.UpstreamSummary) ([]aliasCostEntry, bool) {
	aliasName, ok := strings.CutPrefix(s.Model, "alias/")
	if !ok {
		return nil, false
	}
	var targets []config.AliasTarget
	for _, a := range aliases {
		if a.Name == aliasName {
			targets = a.Targets
			break
		}
	}
	if len(targets) == 0 {
		return nil, false
	}
	entries := make([]aliasCostEntry, 0, len(targets))
	for _, t := range targets {
		p, ok := prices[t.Provider+"/"+t.Model]
		if !ok || p == nil {
			continue
		}
		e := aliasCostEntry{price: p}
		for _, u := range upstream {
			if u.Provider != t.Provider || u.Model != t.Model {
				continue
			}
			if u.Operation != s.Operation || u.StatusCode != s.StatusCode {
				continue
			}
			if s.Tenant != "" && u.Tenant != s.Tenant {
				continue
			}
			if s.Client != "" && u.Client != s.Client {
				continue
			}
			e.prompt += u.PromptTokens
			e.completion += u.CompletionTokens
			e.cached += u.CachedTokens
			e.write += u.CacheCreationTokens
			e.read += u.CacheReadTokens
			e.count += u.Count
			e.matched = true
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return nil, false
	}
	return entries, true
}

func upstreamTokenTotal(entries []aliasCostEntry) int64 {
	var total int64
	for _, e := range entries {
		if !e.matched {
			continue
		}
		total += e.prompt + e.completion + e.cached + e.write + e.read
	}
	return total
}

func splitAliasCost(s accounting.Summary, entries []aliasCostEntry) (float64, bool) {
	var totalWeight int64
	for _, e := range entries {
		totalWeight += e.count
	}
	var total float64
	priced := false
	for _, e := range entries {
		frac := 1.0 / float64(len(entries))
		if totalWeight > 0 {
			if e.count <= 0 {
				continue
			}
			frac = float64(e.count) / float64(totalWeight)
		}
		cost, ok := e.price.Cost(
			int64(float64(s.PromptTokens)*frac),
			int64(float64(s.CompletionTokens)*frac),
			int64(float64(s.CachedTokens)*frac),
			int64(float64(s.CacheCreationTokens)*frac),
			int64(float64(s.CacheReadTokens)*frac),
		)
		if !ok {
			continue
		}
		total += cost
		priced = true
	}
	if !priced {
		return 0, false
	}
	return total, true
}

func (h *Handler) writeResult(w http.ResponseWriter, req *http.Request, r *provider.Result) provider.StreamOutcome {
	defer closeResult(r)
	if r.StatusCode >= 400 && !isJSONContentType(r.Header.Get("Content-Type")) {
		h.writeUpstreamError(w, r)
		if r.Stream != nil {
			r.Stream.Complete(nil, false)
		}
		return provider.StreamOutcome{}
	}
	copyResponseHeaders(w.Header(), r.Header)
	w.WriteHeader(r.StatusCode)
	if r.Streaming && r.StreamBody != nil {
		var err error
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
			_, err = copyAndFlush(w, r.StreamBody, flusher)
		} else {
			_, err = io.Copy(w, r.StreamBody)
		}
		downstreamCanceled := req != nil && req.Context().Err() != nil
		if r.Stream != nil {
			r.Stream.Complete(err, downstreamCanceled)
			outcome := r.Stream.Wait()
			r.Usage = outcome.Usage
			return outcome
		}
		return provider.StreamOutcome{Err: err, DownstreamCanceled: downstreamCanceled}
	}
	_, _ = w.Write(r.Body)
	return provider.StreamOutcome{}
}

func (h *Handler) writeUpstreamError(w http.ResponseWriter, r *provider.Result) {
	body := r.Body
	if r.Streaming && r.StreamBody != nil {
		read, err := io.ReadAll(io.LimitReader(r.StreamBody, maxUpstreamErrorBodyBytes))
		if err == nil {
			body = read
		}
	}
	e := apiError{}
	e.Error.Type = upstreamErrorType(r.StatusCode)
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		e.Error.Message = fmt.Sprintf("upstream returned status %d", r.StatusCode)
	} else {
		e.Error.Message = trimmed
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(r.StatusCode)
	_ = json.NewEncoder(w).Encode(e)
}

const maxUpstreamErrorBodyBytes int64 = 4 << 10

func isJSONContentType(contentType string) bool {
	contentType = strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	return contentType == "application/json" || contentType == "application/vnd.api+json"
}

func upstreamErrorType(status int) string {
	switch {
	case status >= 500:
		return "upstream_error"
	case status == http.StatusUnauthorized:
		return "upstream_auth_failed"
	case status == http.StatusForbidden:
		return "upstream_forbidden"
	case status == http.StatusNotFound:
		return "upstream_not_found"
	case status == http.StatusTooManyRequests:
		return "upstream_rate_limited"
	default:
		return "upstream_error"
	}
}

func (h *Handler) writeModels(w http.ResponseWriter, catalog []ModelCard) {
	resp := modelsResponse{
		Object: "list",
		Data:   make([]ModelCard, 0, len(catalog)),
	}
	resp.Data = append(resp.Data, catalog...)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) writeBillingUsage(w http.ResponseWriter, summaries []accounting.Summary, prices map[string]*config.ModelPricing, aliases []config.Alias, upstream []accounting.UpstreamSummary) {
	entries := make([]billingUsageEntry, 0, len(summaries))
	for _, s := range summaries {
		entry := billingUsageEntry{Summary: s}
		if cost, priced := billingCost(s, prices, aliases, upstream); priced {
			c := cost
			entry.EstimatedCostUSD = &c
		}
		entries = append(entries, entry)
	}
	resp := billingUsageResponse{Object: "list", Data: entries}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, errType, message string) {
	h.writeErrorWithBlock(w, status, errType, message, "")
}

func (h *Handler) writeErrorWithBlock(w http.ResponseWriter, status int, errType, message, blockID string) {
	e := apiError{}
	e.Error.Type = errType
	e.Error.Message = message
	e.Error.BlockID = blockID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

func (h *Handler) writeRequestError(metrics *observability.Metrics, w http.ResponseWriter, r *http.Request, status int, errType, message string) {
	h.writeRequestErrorWithBlock(metrics, w, r, status, errType, message, "")
}

func (h *Handler) writeRequestErrorWithBlock(metrics *observability.Metrics, w http.ResponseWriter, r *http.Request, status int, errType, message, blockID string) {
	if metrics != nil {
		metrics.RecordHTTPError(r.Method, metricsPathLabel(r), status, errType)
	}
	h.writeErrorWithBlock(w, status, errType, message, blockID)
}

func closeResult(r *provider.Result) {
	if r == nil {
		return
	}
	if r.Streaming && r.Stream != nil {
		r.Stream.Complete(io.ErrClosedPipe, true)
	}
	if r.OnClose != nil {
		r.OnClose()
		r.OnClose = nil
	}
	if r.StreamBody != nil {
		_ = r.StreamBody.Close()
		r.StreamBody = nil
	}
}

func copyAndFlush(dst io.Writer, src io.Reader, flusher http.Flusher) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
			flusher.Flush()
		}
		if er != nil {
			if er == io.EOF {
				return written, nil
			}
			return written, er
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	blocked := map[string]struct{}{
		"Connection":          {},
		"Keep-Alive":          {},
		"Proxy-Authenticate":  {},
		"Proxy-Authorization": {},
		"Te":                  {},
		"Trailer":             {},
		"Transfer-Encoding":   {},
		"Upgrade":             {},
		"Content-Length":      {},
	}
	for _, value := range src.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			token = textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(token))
			if token != "" {
				blocked[token] = struct{}{}
			}
		}
	}
	for key, vals := range src {
		if _, skip := blocked[textproto.CanonicalMIMEHeaderKey(key)]; skip {
			continue
		}
		for _, v := range vals {
			dst.Add(key, v)
		}
	}
}
