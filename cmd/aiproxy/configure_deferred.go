package main

import (
	"fmt"
	"strings"

	"github.com/egose/aiproxy/internal/configedit"
)

func deferredConfigureUnsupported() error {
	return fmt.Errorf("peer deferred settings have no CLI creation flags in v1; edit deferred, deferred_transport, and deferred_receiver blocks in the config file directly")
}

func preserveProviderModelDeferredTransports(models, existing []providerModelInput) []providerModelInput {
	blocks := make(map[string]providerModelInput, len(existing))
	for _, model := range existing {
		blocks[model.Name] = model
	}
	models = append([]providerModelInput(nil), models...)
	for i := range models {
		if models[i].DeferredTransportBlock == "" {
			models[i].DeferredTransportBlock = blocks[models[i].Name].DeferredTransportBlock
		}
		if models[i].EncryptedReasoning == nil {
			models[i].EncryptedReasoning = configedit.CloneEncryptedReasoningInput(blocks[models[i].Name].EncryptedReasoning)
		}
	}
	return models
}

func preservedProviderDeferredModelOverlays(existing *providerInput, extends string) []providerModelInput {
	if existing == nil || strings.TrimSpace(extends) == "" {
		return nil
	}
	var models []providerModelInput
	for _, model := range existing.Models {
		if strings.TrimSpace(model.DeferredTransportBlock) != "" || model.EncryptedReasoning != nil {
			models = append(models, providerModelInput{
				Name:                   model.Name,
				DeferredTransportBlock: model.DeferredTransportBlock,
				EncryptedReasoning:     configedit.CloneEncryptedReasoningInput(model.EncryptedReasoning),
			})
		}
	}
	return models
}
