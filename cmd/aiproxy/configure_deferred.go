package main

import (
	"fmt"
	"strings"
)

func deferredConfigureUnsupported() error {
	return fmt.Errorf("peer deferred settings have no CLI creation flags in v1; edit deferred, deferred_transport, and deferred_receiver blocks in the config file directly")
}

func preserveProviderModelDeferredTransports(models, existing []providerModelInput) []providerModelInput {
	blocks := make(map[string]string, len(existing))
	for _, model := range existing {
		blocks[model.Name] = model.DeferredTransportBlock
	}
	models = append([]providerModelInput(nil), models...)
	for i := range models {
		if models[i].DeferredTransportBlock == "" {
			models[i].DeferredTransportBlock = blocks[models[i].Name]
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
		if strings.TrimSpace(model.DeferredTransportBlock) != "" {
			models = append(models, providerModelInput{Name: model.Name, DeferredTransportBlock: model.DeferredTransportBlock})
		}
	}
	return models
}
