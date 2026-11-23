package config

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

const upstreamHeadersAttribute = "upstream_headers"

func ValidateUpstreamHeaderDeclarations(src []byte, filename string) error {
	file, err := parseConfig(expandEnvCalls(trimLeadingWhitespace(src)), filename)
	if err != nil {
		return err
	}
	return validateUpstreamHeaderDeclarations(file.Body)
}

func validateUpstreamHeaderDeclarations(body hcl.Body) error {
	_, err := inspectUpstreamHeaderDeclarations(body)
	return err
}

func inspectUpstreamHeaderDeclarations(body hcl.Body) (int, error) {
	count := 0
	check := func(body hcl.Body, owner string) error {
		present, err := validateUpstreamHeaderScope(body, owner)
		if present {
			count++
		}
		return err
	}
	if err := check(body, "root"); err != nil {
		return 0, err
	}
	content, _, _ := body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{
		{Type: "provider", LabelNames: []string{"type", "name"}},
		{Type: "alias", LabelNames: []string{"name"}},
	}})
	for _, block := range content.Blocks {
		if block.Type == "alias" {
			if err := check(block.Body, fmt.Sprintf("alias %q", block.Labels[0])); err != nil {
				return 0, err
			}
			continue
		}
		owner := fmt.Sprintf("provider %q", block.Labels[1])
		if err := check(block.Body, owner); err != nil {
			return 0, err
		}
		models, _, _ := block.Body.PartialContent(&hcl.BodySchema{Blocks: []hcl.BlockHeaderSchema{{Type: "model", LabelNames: []string{"name"}}}})
		for _, model := range models.Blocks {
			if err := check(model.Body, fmt.Sprintf("%s: model %q", owner, model.Labels[0])); err != nil {
				return 0, err
			}
		}
	}
	return count, nil
}

func validateUpstreamHeaderScope(body hcl.Body, owner string) (bool, error) {
	attrs, _, diags := body.PartialContent(&hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: upstreamHeadersAttribute}}})
	if native, ok := body.(*hclsyntax.Body); ok {
		for _, block := range native.Blocks {
			if block.Type == upstreamHeadersAttribute {
				return false, fmt.Errorf("%s: upstream_headers must be a map attribute, not a block", owner)
			}
		}
	}
	attr := attrs.Attributes[upstreamHeadersAttribute]
	if attr == nil {
		return false, nil
	}
	if diags.HasErrors() {
		return false, fmt.Errorf("%s: duplicate upstream_headers declaration or invalid enclosing body", owner)
	}
	if err := validateUpstreamHeaderExpression(attr.Expr); err != nil {
		return false, fmt.Errorf("%s: %w", owner, err)
	}
	return true, nil
}

func validateUpstreamHeaderExpression(expr hcl.Expression) error {
	for {
		group, ok := expr.(*hclsyntax.ParenthesesExpr)
		if !ok {
			break
		}
		expr = group.Expression
	}
	pairs, diags := hcl.ExprMap(expr)
	if diags.HasErrors() {
		return fmt.Errorf("upstream_headers must be a map of non-null strings")
	}
	if len(pairs) > maxUpstreamHeaderCount {
		return fmt.Errorf("upstream_headers must contain at most %d names", maxUpstreamHeaderCount)
	}
	seen := make(map[string]bool, len(pairs))
	byteCount := 0
	for _, pair := range pairs {
		key, diags := pair.Key.Value(nil)
		if diags.HasErrors() || !key.IsKnown() || key.IsNull() || key.Type() != cty.String {
			return fmt.Errorf("upstream_headers names must be HTTP field names")
		}
		name := key.AsString()
		if len(name) > maxUpstreamHeaderNameBytes || !isValidHeaderName(name) {
			return fmt.Errorf("upstream_headers names must be valid HTTP field names of at most %d bytes", maxUpstreamHeaderNameBytes)
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			return fmt.Errorf("upstream_headers has duplicate header name %q (case-insensitive)", name)
		}
		seen[lower] = true
		if IsReservedUpstreamHeader("", name) {
			return fmt.Errorf("upstream_headers name %q is managed by the proxy", name)
		}
		value, diags := pair.Value.Value(nil)
		if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
			return fmt.Errorf("upstream_headers value for %q must be a non-null string", name)
		}
		text := value.AsString()
		if len(text) > maxUpstreamHeaderValueBytes || strings.TrimSpace(text) == "" {
			return fmt.Errorf("upstream_headers value for %q must be nonblank and at most %d bytes", name, maxUpstreamHeaderValueBytes)
		}
		if !utf8.ValidString(text) || strings.ContainsFunc(text, unicode.IsControl) {
			return fmt.Errorf("upstream_headers value for %q must not contain control characters or invalid UTF-8", name)
		}
		byteCount += len(name) + len(text)
		if byteCount > maxUpstreamHeaderBytes {
			return fmt.Errorf("upstream_headers must contain at most %d name/value bytes", maxUpstreamHeaderBytes)
		}
	}
	return nil
}

func upstreamHeadersParseError(file *hcl.File, diags hcl.Diagnostics) error {
	if file == nil {
		return nil
	}
	for _, diag := range diags {
		if diag.Severity != hcl.DiagError || diag.Subject == nil {
			continue
		}
		subject := *diag.Subject
		if diag.Summary == "Attribute redefined" && subject.CanSliceBytes(file.Bytes) && bytes.Equal(subject.SliceBytes(file.Bytes), []byte(upstreamHeadersAttribute)) {
			return fmt.Errorf("%s: duplicate upstream_headers declaration", subject)
		}
		if attr := file.AttributeAtPos(subject.Start); attr != nil && attr.Name == upstreamHeadersAttribute {
			return fmt.Errorf("%s: invalid upstream_headers syntax", subject)
		}
		if diag.Context != nil {
			if attr := file.AttributeAtPos(diag.Context.Start); attr != nil && attr.Name == upstreamHeadersAttribute {
				return fmt.Errorf("%s: invalid upstream_headers syntax", subject)
			}
		}
	}
	return nil
}
