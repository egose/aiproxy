package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

const adminProviderWritableFields = "name workspace_id type display_name base_url tls_client_hello tls_client_hello_spec_file tls_client_hello_spec upstream_header_timeout user_agent forward_user_agent forward_headers forward_authorization allow_insecure_base_url api_key api_key_ref credential_ref copilot_device_flow_id expected_updated_at enabled extends healthcheck models encrypted_reasoning"
const adminModelWritableFields = "name display_name upstream_name protocol capabilities pricing encrypted_reasoning"
const adminCredentialWritableFields = "api_key api_key_ref_path api_key_ref_key copilot_credential_path copilot_credential_name"
const adminAliasWritableFields = "name workspace_id algorithm retry_status_codes providers model session_affinity encrypted_reasoning targets"

func decodeAdminCatalogBody(r *http.Request, target any) error {
	decoder := json.NewDecoder(ioLimitReader(r))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("body must contain exactly one JSON object")
	}
	return nil
}

func adminJSONKeyError(raw []byte, writable string) error {
	return adminJSONKeyErrorWithCompatibility(raw, writable, nil, false)
}

func adminJSONKeyErrorWithCompatibility(raw []byte, writable string, caseFolded []string, allowUnknown bool) error {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("body must be a JSON object")
	}
	allowed := strings.Fields(writable)
	seen := make(map[string]bool, len(allowed))
	return visitAdminJSONFields(raw, func(name string, _ json.RawMessage) error {
		folded := strings.Map(func(value rune) rune {
			minimum := value
			for next := unicode.SimpleFold(value); next != value; next = unicode.SimpleFold(next) {
				if next < minimum {
					minimum = next
				}
			}
			return minimum
		}, name)
		if seen[folded] {
			return fmt.Errorf("duplicate JSON field %q", name)
		}
		seen[folded] = true
		for _, field := range allowed {
			if field == name {
				return nil
			}
		}
		for _, field := range caseFolded {
			if strings.EqualFold(field, name) {
				return nil
			}
		}
		if allowUnknown {
			return nil
		}
		return fmt.Errorf("unsupported JSON field %q", name)
	})
}

func validateAdminProviderInput(req adminProviderUpsert) error {
	name := strings.TrimSpace(req.Name)
	if err := validateManagedDeferredDeclarations(name, req.hasDeferredDeclarations()); err != nil {
		return err
	}
	if req.unsupportedTLSDeclaration {
		return fmt.Errorf("provider %q: tls_client_hello file and inline specifications are static-config-only; managed providers support built-in profiles only", name)
	}
	if req.inputJSONError != nil {
		return req.inputJSONError
	}
	if _, err := config.DecodeEncryptedReasoningJSON(req.EncryptedReasoning, fmt.Sprintf("provider %q", name)); err != nil {
		return err
	}
	if req.Models != nil {
		for _, model := range *req.Models {
			if model.inputJSONError != nil {
				return model.inputJSONError
			}
			if _, err := config.DecodeEncryptedReasoningJSON(model.EncryptedReasoning, fmt.Sprintf("provider %q: model %q", name, model.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func encodeAdminEncryptedReasoning(raw json.RawMessage, owner string) (json.RawMessage, error) {
	policy, err := config.DecodeEncryptedReasoningJSON(raw, owner)
	if err != nil {
		return nil, err
	}
	return config.EncodeEncryptedReasoningJSON(policy, owner)
}

func adminReasoningEffective(policy config.EncryptedReasoning) adminEncryptedReason {
	return adminEncryptedReason{
		Passthrough: policy.Passthrough, OnCallerMismatch: policy.OnCallerMismatch,
		MatchMessages: append([]string(nil), policy.MatchMessages...),
	}
}

func resolveAdminEncryptedReasoning(deps Dependencies, provider config.Provider, model config.Model) config.EncryptedReasoningResolution {
	if deps.Resolver != nil {
		return deps.Resolver.EncryptedReasoning(provider, model, nil)
	}
	return config.ResolveEncryptedReasoning(nil, provider, model, nil)
}

func setProviderReasoningView(deps Dependencies, view *adminProviderView, provider config.Provider) {
	if !provider.EncryptedReasoningInherited {
		view.EncryptedReasoning = config.DeclaredEncryptedReasoning(provider.EncryptedReasoning)
	}
	resolved := resolveAdminEncryptedReasoning(deps, provider, config.Model{})
	view.EncryptedReasoningEffective = adminReasoningEffective(resolved.Policy)
	view.EncryptedReasoningSource = resolved.Source
	view.EncryptedReasoningInherited = resolved.Inherited
	view.ModelsInherited = provider.Extends != ""
	view.Models = make([]adminModelView, 0, len(provider.Models))
	for _, model := range provider.Models {
		caps := make([]string, 0, len(model.Capabilities))
		for _, capability := range model.Capabilities {
			caps = append(caps, string(capability))
		}
		mv := adminModelView{
			Name: model.Name, DisplayName: model.DisplayName, UpstreamName: model.UpstreamName,
			Protocol: string(model.Protocol), Capabilities: caps, Pricing: modelPricingView(model.Pricing),
		}
		if !model.EncryptedReasoningInherited {
			mv.EncryptedReasoning = config.DeclaredEncryptedReasoning(model.EncryptedReasoning)
		}
		resolved := resolveAdminEncryptedReasoning(deps, provider, model)
		mv.EncryptedReasoningEffective = adminReasoningEffective(resolved.Policy)
		mv.EncryptedReasoningSource = resolved.Source
		mv.EncryptedReasoningInherited = resolved.Inherited
		view.Models = append(view.Models, mv)
	}
}

func setAliasReasoningView(view *adminAliasView, policy *config.EncryptedReasoning) {
	view.EncryptedReasoning = config.DeclaredEncryptedReasoning(policy)
	view.EncryptedReasoningEffective = nil
	if policy != nil {
		effective := adminReasoningEffective(*config.NormalizeEncryptedReasoning(policy))
		view.EncryptedReasoningEffective = &effective
	}
}

func validateProviderModelTransition(current, candidate store.DBProvider, models []store.DBProviderModel, replacement *[]adminModelUpsert) error {
	if replacement != nil || (current.Extends == "") == (candidate.Extends == "") {
		return nil
	}
	for _, model := range models {
		policy, err := config.DecodeEncryptedReasoningJSON(model.EncryptedReasoning, fmt.Sprintf("provider %q: model %q", current.Name, model.Name))
		if err != nil {
			return err
		}
		if policy != nil {
			return fmt.Errorf("provider %q: switching concrete and derived providers with model encrypted_reasoning requires an explicit models array", current.Name)
		}
	}
	return nil
}
