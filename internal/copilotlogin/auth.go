package copilotlogin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
)

const (
	DefaultBaseURL      = "https://api.githubcopilot.com"
	ChatCompletionsPath = "/chat/completions"
	APIVersion          = "2026-06-01"
	OpenAIIntent        = "conversation-edits"
	InitiatorUser       = "user"
	VisionHeader        = "Copilot-Vision-Request"
	InitiatorHeader     = "x-initiator"
	IntentHeader        = "Openai-Intent"
	APIVersionHeader    = "X-GitHub-Api-Version"
)

func UserAgent(version string) string {
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	return "aiproxy/" + version
}

func HasImageContent(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return false
	}
	return containsImagePart(v)
}

func containsImagePart(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		if typ, ok := t["type"].(string); ok && (typ == "image_url" || typ == "input_image") {
			return true
		}
		if _, ok := t["image_url"]; ok {
			return true
		}
		if _, ok := t["input_image"]; ok {
			return true
		}
		for _, child := range t {
			if containsImagePart(child) {
				return true
			}
		}
		return false
	case []any:
		for _, child := range t {
			if containsImagePart(child) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func IsStreamingBody(body []byte) bool {
	var probe struct {
		Stream bool `json:"stream"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Stream
}

func BuildHeaders(token, version string, body []byte) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", UserAgent(version))
	h.Set(APIVersionHeader, APIVersion)
	h.Set(IntentHeader, OpenAIIntent)
	h.Set(InitiatorHeader, InitiatorUser)
	if HasImageContent(body) {
		h.Set(VisionHeader, "true")
	}
	if IsStreamingBody(body) {
		h.Set("Accept", "text/event-stream")
	} else {
		h.Set("Accept", "application/json")
	}
	return h
}

func ApplyHeaders(req *http.Request, token, version string, body []byte) {
	req.Header.Del(VisionHeader)
	for key, vals := range BuildHeaders(token, version, body) {
		copied := make([]string, len(vals))
		copy(copied, vals)
		req.Header[key] = copied
	}
	for _, key := range []string{"Cookie", "Cookie2", "X-Api-Key", "Anthropic-Beta", "X-Interaction-Id", "X-Interaction-Type"} {
		req.Header.Del(key)
	}
}

func BuildModelsHeaders(token, version string) http.Header {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+token)
	h.Set("User-Agent", UserAgent(version))
	h.Set(APIVersionHeader, APIVersion)
	h.Set("Accept", "application/json")
	return h
}

func ApplyModelsHeaders(req *http.Request, token, version string) {
	for key, vals := range BuildModelsHeaders(token, version) {
		copied := make([]string, len(vals))
		copy(copied, vals)
		req.Header[key] = copied
	}
	for _, key := range []string{"Cookie", "Cookie2", "X-Api-Key", "Anthropic-Beta", "X-Interaction-Id", "X-Interaction-Type"} {
		req.Header.Del(key)
	}
}
