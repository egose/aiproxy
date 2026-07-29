package httpapi

import (
	"net/http"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func operationFromRequest(r *http.Request) (provider.Operation, bool) {
	if r.Method != http.MethodPost {
		return 0, false
	}
	switch r.URL.Path {
	case "/v1/chat/completions":
		return provider.OpChatCompletions, true
	case "/v1/embeddings":
		return provider.OpEmbeddings, true
	case "/v1/responses":
		return provider.OpResponses, true
	case "/v1/images/generations":
		return provider.OpImagesGenerations, true
	case "/v1/audio/transcriptions":
		return provider.OpAudioTranscriptions, true
	case "/v1/audio/speech":
		return provider.OpAudioSpeech, true
	default:
		return 0, false
	}
}

func ensureOperationSupported(op provider.Operation, resolved modelresolver.ResolveResult, providers map[string]config.Provider) error {
	required, ok := requiredCapability(op)
	if !ok {
		return provider.ErrUnsupportedOperation{Operation: op}
	}
	if resolved.Kind == modelresolver.KindDirect {
		caps := config.EffectiveCapabilities(resolved.Provider.Type, resolved.Model)
		if config.HasCapability(caps, required) {
			return nil
		}
		return provider.ErrUnsupportedOperation{ProviderType: resolved.Provider.Type, Operation: op}
	}
	if resolved.Kind == modelresolver.KindAlias {
		caps := config.AliasEffectiveCapabilities(resolved.Alias, providers)
		if config.HasCapability(caps, required) {
			return nil
		}
		return provider.ErrUnsupportedOperation{Operation: op}
	}
	return provider.ErrUnsupportedOperation{Operation: op}
}

func requiredCapability(op provider.Operation) (config.Capability, bool) {
	switch op {
	case provider.OpChatCompletions:
		return config.CapabilityChat, true
	case provider.OpEmbeddings:
		return config.CapabilityEmbeddings, true
	case provider.OpResponses:
		return config.CapabilityResponses, true
	case provider.OpImagesGenerations:
		return config.CapabilityImages, true
	case provider.OpAudioTranscriptions:
		return config.CapabilityAudioTranscriptions, true
	case provider.OpAudioSpeech:
		return config.CapabilityAudioSpeech, true
	default:
		return "", false
	}
}
