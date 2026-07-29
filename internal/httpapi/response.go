package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

type apiError struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type modelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelCard `json:"data"`
}

type billingUsageResponse struct {
	Object string               `json:"object"`
	Data   []accounting.Summary `json:"data"`
}

func (h *Handler) writeResult(w http.ResponseWriter, r *provider.Result) {
	defer closeResult(r)
	if r.StatusCode >= 400 && !isJSONContentType(r.Header.Get("Content-Type")) {
		h.writeUpstreamError(w, r)
		return
	}
	copyResponseHeaders(w.Header(), r.Header)
	w.WriteHeader(r.StatusCode)
	if r.Streaming && r.StreamBody != nil {
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
			_, _ = copyAndFlush(w, r.StreamBody, flusher)
			return
		}
		_, _ = io.Copy(w, r.StreamBody)
		return
	}
	_, _ = w.Write(r.Body)
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

func (h *Handler) writeBillingUsage(w http.ResponseWriter, summaries []accounting.Summary) {
	resp := billingUsageResponse{Object: "list", Data: summaries}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, errType, message string) {
	e := apiError{}
	e.Error.Type = errType
	e.Error.Message = message
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

func (h *Handler) writeRequestError(metrics *observability.Metrics, w http.ResponseWriter, r *http.Request, status int, errType, message string) {
	if metrics != nil {
		metrics.RecordHTTPError(r.Method, metricsPathLabel(r), status, errType)
	}
	h.writeError(w, status, errType, message)
}

func closeResult(r *provider.Result) {
	if r == nil {
		return
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
