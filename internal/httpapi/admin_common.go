package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type ctxLike interface {
	ctx() context.Context
}

type ctxWrap struct {
	c context.Context
}

func (w ctxWrap) ctx() context.Context { return w.c }

func contextOf(c context.Context) ctxLike {
	if c == nil {
		return ctxWrap{c: context.Background()}
	}
	return ctxWrap{c: c}
}

func errBad(msg string) error {
	return errors.New(msg)
}

func (h *Handler) activateChange(deps Dependencies, w http.ResponseWriter) bool {
	if deps.RequestReload == nil {
		return true
	}
	if err := deps.RequestReload(); err != nil {
		http.Error(w, "saved but activation failed: "+err.Error(), http.StatusInternalServerError)
		return false
	}
	return true
}

func jsonBlockPresent(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

func unmarshalJSONBlock(raw []byte, target interface{}) bool {
	if !jsonBlockPresent(raw) {
		return false
	}
	return json.Unmarshal(raw, target) == nil
}

func validResourceName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return cIsLowerStart(name)
}

func cIsLowerStart(name string) bool {
	c := name[0]
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}
