package config

import (
	"fmt"
	"slices"
	"strings"
)

func DefaultEncryptedReasoning() EncryptedReasoning {
	return EncryptedReasoning{
		Passthrough:      true,
		OnCallerMismatch: EncryptedReasoningFail,
		MatchMessages:    slices.Clone(DefaultEncryptedReasoningMatchMessages),
	}
}

func CloneEncryptedReasoningDeclaration(in *EncryptedReasoningDeclaration) *EncryptedReasoningDeclaration {
	if in == nil {
		return nil
	}
	out := *in
	if in.Passthrough != nil {
		value := *in.Passthrough
		out.Passthrough = &value
	}
	if in.OnCallerMismatch != nil {
		value := *in.OnCallerMismatch
		out.OnCallerMismatch = &value
	}
	if in.MatchMessages != nil {
		value := slices.Clone(*in.MatchMessages)
		out.MatchMessages = &value
	}
	return &out
}

func CloneEncryptedReasoning(in *EncryptedReasoning) *EncryptedReasoning {
	if in == nil {
		return nil
	}
	out := *in
	out.MatchMessages = slices.Clone(in.MatchMessages)
	out.Declaration = CloneEncryptedReasoningDeclaration(in.Declaration)
	return &out
}

func NewEncryptedReasoning(declaration *EncryptedReasoningDeclaration) *EncryptedReasoning {
	if declaration == nil {
		return nil
	}
	return NormalizeEncryptedReasoning(&EncryptedReasoning{Declaration: declaration})
}

func DeclaredEncryptedReasoning(policy *EncryptedReasoning) *EncryptedReasoningDeclaration {
	if policy == nil {
		return nil
	}
	if policy.Declaration != nil {
		return CloneEncryptedReasoningDeclaration(policy.Declaration)
	}
	passthrough := policy.Passthrough
	out := &EncryptedReasoningDeclaration{Passthrough: &passthrough}
	if policy.OnCallerMismatch != "" {
		value := policy.OnCallerMismatch
		out.OnCallerMismatch = &value
	}
	if policy.MatchMessages != nil {
		value := slices.Clone(policy.MatchMessages)
		out.MatchMessages = &value
	}
	return out
}

func NormalizeEncryptedReasoning(policy *EncryptedReasoning) *EncryptedReasoning {
	if policy == nil {
		return nil
	}
	out := CloneEncryptedReasoning(policy)
	if declaration := out.Declaration; declaration != nil {
		out.Passthrough = true
		out.OnCallerMismatch = EncryptedReasoningFail
		out.MatchMessages = nil
		if declaration.Passthrough != nil {
			out.Passthrough = *declaration.Passthrough
		}
		if declaration.OnCallerMismatch != nil {
			out.OnCallerMismatch = *declaration.OnCallerMismatch
		}
		if declaration.MatchMessages != nil {
			out.MatchMessages = slices.Clone(*declaration.MatchMessages)
		}
	}
	if out.OnCallerMismatch == "" {
		out.OnCallerMismatch = EncryptedReasoningFail
	}
	for i, message := range out.MatchMessages {
		out.MatchMessages[i] = strings.ToLower(strings.TrimSpace(message))
	}
	if len(out.MatchMessages) == 0 {
		out.MatchMessages = DefaultEncryptedReasoning().MatchMessages
	}
	return out
}

func ValidateEncryptedReasoning(policy *EncryptedReasoning, owner string) error {
	er := NormalizeEncryptedReasoning(policy)
	if er == nil {
		return nil
	}
	switch er.OnCallerMismatch {
	case EncryptedReasoningFail, EncryptedReasoningStripAndRetry:
	default:
		return fmt.Errorf("%s: encrypted_reasoning.on_caller_mismatch must be %q or %q", owner, EncryptedReasoningFail, EncryptedReasoningStripAndRetry)
	}
	if len(er.MatchMessages) > 16 {
		return fmt.Errorf("%s: encrypted_reasoning.match_messages must list at most 16 entries", owner)
	}
	seen := make(map[string]bool, len(er.MatchMessages))
	for _, message := range er.MatchMessages {
		if message == "" {
			return fmt.Errorf("%s: encrypted_reasoning.match_messages must not contain empty values", owner)
		}
		if len(message) < 4 || len(message) > 256 {
			return fmt.Errorf("%s: encrypted_reasoning.match_messages entries must be between 4 and 256 characters", owner)
		}
		if seen[message] {
			return fmt.Errorf("%s: encrypted_reasoning.match_messages has duplicate entry %q", owner, message)
		}
		seen[message] = true
	}
	return nil
}

func ResolveEncryptedReasoning(root *EncryptedReasoning, provider Provider, model Model, alias *Alias) EncryptedReasoningResolution {
	if alias != nil && alias.EncryptedReasoning != nil {
		return resolvedEncryptedReasoning(alias.EncryptedReasoning, EncryptedReasoningSourceAlias, false)
	}
	if model.EncryptedReasoning != nil {
		return resolvedEncryptedReasoning(model.EncryptedReasoning, EncryptedReasoningSourceModel, model.EncryptedReasoningInherited)
	}
	if provider.EncryptedReasoning != nil {
		return resolvedEncryptedReasoning(provider.EncryptedReasoning, EncryptedReasoningSourceProvider, provider.EncryptedReasoningInherited)
	}
	if root != nil {
		return resolvedEncryptedReasoning(root, EncryptedReasoningSourceRoot, false)
	}
	return EncryptedReasoningResolution{Policy: DefaultEncryptedReasoning(), Source: EncryptedReasoningSourceBuiltin}
}

func resolvedEncryptedReasoning(policy *EncryptedReasoning, source EncryptedReasoningSource, inherited bool) EncryptedReasoningResolution {
	er := NormalizeEncryptedReasoning(policy)
	er.Declaration = nil
	return EncryptedReasoningResolution{Policy: *er, Source: source, Inherited: inherited}
}

func buildEncryptedReasoning(raw *rawEncryptedReasoning) *EncryptedReasoning {
	if raw == nil {
		return nil
	}
	return NewEncryptedReasoning(&EncryptedReasoningDeclaration{
		Passthrough: raw.Passthrough, OnCallerMismatch: raw.OnCallerMismatch, MatchMessages: raw.MatchMessages,
	})
}

func buildProviderEncryptedReasoning(provider *Provider, raw rawProvider) error {
	if raw.EncryptedReasoning != nil {
		provider.EncryptedReasoning = buildEncryptedReasoning(raw.EncryptedReasoning)
		provider.EncryptedReasoningInherited = false
	}
	for _, local := range raw.Models {
		if local.EncryptedReasoning == nil {
			continue
		}
		policy := buildEncryptedReasoning(local.EncryptedReasoning)
		found := false
		for i := range provider.Models {
			if provider.Models[i].Name == local.Name {
				provider.Models[i].EncryptedReasoning = CloneEncryptedReasoning(policy)
				provider.Models[i].EncryptedReasoningInherited = false
				found = true
			}
		}
		if model, ok := provider.ModelByName[local.Name]; ok {
			model.EncryptedReasoning = CloneEncryptedReasoning(policy)
			model.EncryptedReasoningInherited = false
			provider.ModelByName[local.Name] = model
			found = true
		}
		if !found {
			return fmt.Errorf("model %q: encrypted_reasoning requires an existing model", local.Name)
		}
	}
	return validateProviderEncryptedReasoning(*provider)
}

func validateProviderEncryptedReasoning(provider Provider) error {
	owner := fmt.Sprintf("provider %q", provider.Name)
	if err := ValidateEncryptedReasoning(provider.EncryptedReasoning, owner); err != nil {
		return err
	}
	for _, model := range provider.Models {
		if err := ValidateEncryptedReasoning(model.EncryptedReasoning, fmt.Sprintf("%s: model %q", owner, model.Name)); err != nil {
			return err
		}
	}
	for _, model := range provider.ModelByName {
		if err := ValidateEncryptedReasoning(model.EncryptedReasoning, fmt.Sprintf("%s: model %q", owner, model.Name)); err != nil {
			return err
		}
	}
	return nil
}
