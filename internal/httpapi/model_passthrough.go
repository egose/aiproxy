package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"strings"
	"unicode"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func resolveInferenceModelPassthrough(deps Dependencies, principal *auth.Principal, op provider.Operation, publicModel string, logger *slog.Logger) (modelresolver.ResolveResult, bool) {
	if deps.Resolver == nil {
		return modelresolver.ResolveResult{}, false
	}
	if principal == nil {
		if deps.MultiTenancy.Enabled {
			return modelresolver.ResolveResult{}, false
		}
	} else if _, static := principal.CredentialID().StaticClientName(); !static {
		return modelresolver.ResolveResult{}, false
	}
	var workspace *identity.WorkspaceScope
	if deps.MultiTenancy.Enabled {
		scope := principal.WorkspaceScope()
		workspace = &scope
	}
	resolved, match := deps.Resolver.ResolveModelPassthrough(publicModel, workspace)
	if match == config.ModelPassthroughAmbiguous {
		logger.Warn("ambiguous model passthrough match")
	}
	if match != config.ModelPassthroughMatched ||
		!deps.Authorizer.AllowModelPassthrough(principal, resolved.Provider.Name, resolved.PassthroughRule.Name()) {
		return modelresolver.ResolveResult{}, false
	}
	required, ok := requiredCapability(op)
	if !ok || !config.HasCapability(resolved.PassthroughRule.Capabilities(), required) {
		return modelresolver.ResolveResult{}, false
	}
	return resolved, true
}

func modelPassthroughTargetIdentity(resolved modelresolver.ResolveResult) string {
	return "@passthrough/" + resolved.PassthroughRule.Name()
}

func validModelPassthroughInput(op provider.Operation, contentType string, body []byte, model string) bool {
	isMultipart := strings.HasPrefix(contentType, "multipart/form-data;")
	if isMultipart != (op == provider.OpAudioTranscriptions) {
		return false
	}
	if isMultipart {
		boundary := multipartContentBoundary(contentType)
		mediaType, params, err := mime.ParseMediaType(contentType)
		var form multipart.Writer
		if err != nil || mediaType != "multipart/form-data" || params["boundary"] != boundary ||
			form.SetBoundary(boundary) != nil {
			return false
		}
		return validModelPassthroughMultipart(body, boundary, model)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if token, err := dec.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	var found bool
	var other json.RawMessage
	for dec.More() {
		token, err := dec.Token()
		key, ok := token.(string)
		if err != nil || !ok || alternateModelSelector(key) {
			return false
		}
		if strings.EqualFold(key, "model") {
			var value string
			if found || key != "model" || dec.Decode(&value) != nil || value != model {
				return false
			}
			found = true
		} else if err := dec.Decode(&other); err != nil {
			return false
		}
	}
	if token, err := dec.Token(); err != nil || token != json.Delim('}') || !found {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}

func hasMultipartClosingBoundary(body []byte, boundary string) bool {
	closing := []byte("\n--" + boundary + "--")
	for {
		_, tail, ok := bytes.Cut(body, closing)
		if !ok {
			return false
		}
		tail = bytes.TrimLeft(tail, " \t")
		if len(tail) == 0 || tail[0] == '\n' || bytes.HasPrefix(tail, []byte("\r\n")) {
			return true
		}
		body = tail
	}
}

func validModelPassthroughMultipart(body []byte, boundary, model string) bool {
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var found bool
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			return found && hasMultipartClosingBoundary(body, boundary)
		}
		if err != nil {
			return false
		}
		disposition := part.Header.Values("Content-Disposition")
		if len(disposition) != 1 || hasExtendedFormName(disposition[0]) {
			return false
		}
		mediaType, params, err := mime.ParseMediaType(disposition[0])
		name := params["name"]
		if err != nil || mediaType != "form-data" || name == "" || alternateModelSelector(name) {
			return false
		}
		if !strings.EqualFold(name, "model") {
			if _, err := io.Copy(io.Discard, part); err != nil {
				return false
			}
			continue
		}
		if _, file := params["filename"]; found || name != "model" || file || len(part.Header.Values("Content-Transfer-Encoding")) != 0 {
			return false
		}
		data, err := io.ReadAll(io.LimitReader(part, int64(len(model)+1)))
		if err != nil || string(data) != model {
			return false
		}
		found = true
	}
}

func alternateModelSelector(name string) bool {
	return strings.EqualFold(name, "models") || strings.EqualFold(name, "preset")
}

func hasExtendedFormName(disposition string) bool {
	quoted := false
	for i := 0; i < len(disposition); i++ {
		switch disposition[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if quoted {
				i++
			}
		case ';':
			if !quoted {
				param := strings.TrimLeftFunc(disposition[i+1:], unicode.IsSpace)
				if len(param) >= len("name*") && strings.EqualFold(param[:len("name*")], "name*") {
					return true
				}
			}
		}
	}
	return false
}
