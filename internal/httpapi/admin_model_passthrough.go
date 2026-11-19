package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var errManagedModelPassthrough = errors.New("model_passthrough and allowed_model_passthroughs are static-config-only; edit the static config file")

func managedModelPassthroughJSONError(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return inspectManagedModelPassthroughValue(decoder)
}

func inspectManagedModelPassthroughValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name := key.(string)
			if strings.EqualFold(name, "model_passthrough") || strings.EqualFold(name, "model_passthroughs") ||
				strings.EqualFold(name, "allowed_model_passthroughs") {
				return errManagedModelPassthrough
			}
			if err := inspectManagedModelPassthroughValue(decoder); err != nil {
				return err
			}
		}
	case json.Delim('['):
		for decoder.More() {
			if err := inspectManagedModelPassthroughValue(decoder); err != nil {
				return err
			}
		}
	default:
		return nil
	}
	_, err = decoder.Token()
	return err
}

func decodeManagedKeyBody(r *http.Request, target any) error {
	var raw json.RawMessage
	if err := json.NewDecoder(ioLimitReader(r)).Decode(&raw); err != nil {
		return errBad("invalid body")
	}
	if err := managedModelPassthroughJSONError(raw); err != nil {
		return err
	}
	if json.Unmarshal(raw, target) != nil {
		return errBad("invalid body")
	}
	return nil
}
