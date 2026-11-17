package dbmerge

import (
	"fmt"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func decodeStoredEncryptedReasoning(raw []byte, owner string) (*config.EncryptedReasoning, error) {
	policy, err := config.DecodeEncryptedReasoningJSON(raw, owner)
	if err != nil {
		return nil, fmt.Errorf("%s: stored encrypted_reasoning is corrupt: %w", owner, err)
	}
	return policy, nil
}

func applyStoredModelOverlays(p *config.Provider, models []store.DBProviderModel) error {
	positions := make(map[string]int, len(p.Models))
	for i, model := range p.Models {
		positions[model.Name] = i
	}
	seen := make(map[string]bool, len(models))
	for _, row := range models {
		owner := fmt.Sprintf("provider %q: model %q", p.Name, row.Name)
		policy, err := decodeStoredEncryptedReasoning(row.EncryptedReasoning, owner)
		if err != nil {
			return err
		}
		if policy == nil {
			continue
		}
		if seen[row.Name] {
			return fmt.Errorf("%s: duplicate encrypted_reasoning overlay", owner)
		}
		seen[row.Name] = true
		position, ok := positions[row.Name]
		if !ok {
			return fmt.Errorf("%s: encrypted_reasoning overlay requires an existing inherited model", owner)
		}
		if row.DisplayName != "" || row.UpstreamName != "" || row.Protocol != "" || len(row.Capabilities) != 0 || jsonPresent(row.Pricing) {
			return fmt.Errorf("%s: derived model overlay cannot redefine inherited model fields", owner)
		}
		p.Models[position].EncryptedReasoning = config.CloneEncryptedReasoning(policy)
		p.Models[position].EncryptedReasoningInherited = false
	}
	p.ModelByName = make(map[string]config.Model, len(p.Models))
	for _, model := range p.Models {
		p.ModelByName[model.Name] = cloneModel(model)
	}
	return nil
}

func validateMaterializedEncryptedReasoning(p config.Provider) error {
	owner := fmt.Sprintf("provider %q", p.Name)
	if err := config.ValidateEncryptedReasoning(p.EncryptedReasoning, owner); err != nil {
		return err
	}
	for _, model := range p.Models {
		if err := config.ValidateEncryptedReasoning(model.EncryptedReasoning, fmt.Sprintf("%s: model %q", owner, model.Name)); err != nil {
			return err
		}
	}
	return nil
}
