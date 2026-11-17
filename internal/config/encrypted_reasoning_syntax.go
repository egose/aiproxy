package config

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func validateEncryptedReasoningDeclarations(body hcl.Body) error {
	if err := validateEncryptedReasoningBlock(body, "root"); err != nil {
		return err
	}
	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "provider", LabelNames: []string{"type", "name"}},
			{Type: "alias", LabelNames: []string{"name"}},
		},
	})
	if diags.HasErrors() {
		return fmt.Errorf("encrypted_reasoning: cannot inspect provider and alias declarations")
	}
	for _, block := range content.Blocks {
		if block.Type == "alias" {
			if err := validateEncryptedReasoningBlock(block.Body, fmt.Sprintf("alias %q", block.Labels[0])); err != nil {
				return err
			}
			continue
		}
		owner := fmt.Sprintf("provider %q", block.Labels[1])
		if err := validateEncryptedReasoningBlock(block.Body, owner); err != nil {
			return err
		}
		models, _, diags := block.Body.PartialContent(&hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{{Type: "model", LabelNames: []string{"name"}}},
		})
		if diags.HasErrors() {
			return fmt.Errorf("%s: encrypted_reasoning: cannot inspect model declarations", owner)
		}
		for _, model := range models.Blocks {
			if err := validateEncryptedReasoningBlock(model.Body, fmt.Sprintf("%s: model %q", owner, model.Labels[0])); err != nil {
				return err
			}
		}
	}
	return nil
}

func encryptedReasoningParseError(file *hcl.File, diags hcl.Diagnostics) error {
	if file == nil {
		return nil
	}
	for _, diag := range diags {
		if diag.Severity != hcl.DiagError || diag.Subject == nil {
			continue
		}
		owner := "root"
		for _, block := range file.BlocksAtPos(diag.Subject.Start) {
			switch block.Type {
			case "provider":
				if len(block.Labels) == 2 {
					owner = fmt.Sprintf("provider %q", block.Labels[1])
				}
			case "model":
				if len(block.Labels) == 1 {
					owner = fmt.Sprintf("%s: model %q", owner, block.Labels[0])
				}
			case "alias":
				if len(block.Labels) == 1 {
					owner = fmt.Sprintf("alias %q", block.Labels[0])
				}
			case "encrypted_reasoning":
				return fmt.Errorf("%s: encrypted_reasoning: %s", owner, diag.Error())
			}
		}
	}
	return nil
}

func validateEncryptedReasoningBlock(body hcl.Body, owner string) error {
	attrs, _, diags := body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "encrypted_reasoning"}}})
	if diags.HasErrors() {
		return fmt.Errorf("%s: cannot inspect encrypted_reasoning declarations", owner)
	}
	if attr := attrs.Attributes["encrypted_reasoning"]; attr != nil {
		if _, native := body.(*hclsyntax.Body); native {
			return fmt.Errorf("%s: encrypted_reasoning must be an unlabeled block", owner)
		}
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !value.IsKnown() || value.IsNull() {
			return fmt.Errorf("%s: encrypted_reasoning must be a non-null block object", owner)
		}
		if value.Type().IsTupleType() || value.Type().IsListType() {
			if value.LengthInt() != 1 {
				return fmt.Errorf("%s: exactly one encrypted_reasoning block object is required when declared", owner)
			}
			it := value.ElementIterator()
			it.Next()
			_, value = it.Element()
		}
		if !value.IsKnown() || value.IsNull() || !value.Type().IsObjectType() {
			return fmt.Errorf("%s: encrypted_reasoning must be a non-null block object", owner)
		}
	}
	content, _, diags := body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "encrypted_reasoning"}}})
	if diags.HasErrors() {
		return fmt.Errorf("%s: encrypted_reasoning requires an unlabeled block object", owner)
	}
	if len(content.Blocks) > 1 {
		return fmt.Errorf("%s: only one encrypted_reasoning block is supported", owner)
	}
	for _, block := range content.Blocks {
		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			return fmt.Errorf("%s: encrypted_reasoning supports attributes only", owner)
		}
		for _, attr := range attrs {
			if err := validateEncryptedReasoningAttribute(attr, owner); err != nil {
				return err
			}
		}
		var raw rawEncryptedReasoning
		if diags := gohcl.DecodeBody(block.Body, nil, &raw); diags.HasErrors() {
			return fmt.Errorf("%s: encrypted_reasoning: %s", owner, diags.Error())
		}
		if err := ValidateEncryptedReasoning(buildEncryptedReasoning(&raw), owner); err != nil {
			return err
		}
	}
	return nil
}

func validateEncryptedReasoningAttribute(attr *hcl.Attribute, owner string) error {
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || !value.IsKnown() || value.IsNull() {
		return fmt.Errorf("%s: encrypted_reasoning.%s must not be null or unknown", owner, attr.Name)
	}
	switch attr.Name {
	case "passthrough":
		if value.Type() != cty.Bool {
			return fmt.Errorf("%s: encrypted_reasoning.passthrough must be a boolean", owner)
		}
	case "on_caller_mismatch":
		if value.Type() != cty.String {
			return fmt.Errorf("%s: encrypted_reasoning.on_caller_mismatch must be a string", owner)
		}
	case "match_messages":
		if !value.Type().IsTupleType() && !value.Type().IsListType() {
			return fmt.Errorf("%s: encrypted_reasoning.match_messages must be a list of strings", owner)
		}
		for it := value.ElementIterator(); it.Next(); {
			_, entry := it.Element()
			if !entry.IsKnown() || entry.IsNull() || entry.Type() != cty.String {
				return fmt.Errorf("%s: encrypted_reasoning.match_messages must contain non-null strings", owner)
			}
		}
	default:
		return fmt.Errorf("%s: encrypted_reasoning has unsupported field %q", owner, attr.Name)
	}
	return nil
}
