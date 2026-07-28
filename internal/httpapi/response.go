package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

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

func (h *handler) writeResult(w http.ResponseWriter, r *provider.Result) {
	defer closeResult(r)
	for key, vals := range r.Header {
		if key == "Content-Length" || key == "Transfer-Encoding" {
			continue
		}
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
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

func (h *handler) writeModels(w http.ResponseWriter) {
	resp := modelsResponse{
		Object: "list",
		Data:   make([]ModelCard, 0, len(h.deps.Catalog)),
	}
	resp.Data = append(resp.Data, h.deps.Catalog...)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *handler) writeError(w http.ResponseWriter, status int, errType, message string) {
	e := apiError{}
	e.Error.Type = errType
	e.Error.Message = message
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
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
