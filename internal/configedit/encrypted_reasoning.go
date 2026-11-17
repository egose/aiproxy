package configedit

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

type EncryptedReasoningInput struct {
	Passthrough         *bool
	OnCallerMismatch    string
	MatchMessages       []string
	HasOnCallerMismatch bool
	HasMatchMessages    bool
	Block               string
	Expressions         map[string]string
}

type EncryptedReasoningEdit struct {
	Remove           bool
	Reset            bool
	Passthrough      *bool
	OnCallerMismatch *string
	MatchMessages    *[]string
	Expressions      map[string]*string
}

type EncryptedReasoningScope struct {
	Provider string
	Model    string
	Alias    string
}

var encryptedReasoningAttributes = []string{"passthrough", "on_caller_mismatch", "match_messages"}

func EncryptedReasoningInputFromDeclaration(declaration *config.EncryptedReasoningDeclaration) *EncryptedReasoningInput {
	if declaration == nil {
		return nil
	}
	declaration = config.CloneEncryptedReasoningDeclaration(declaration)
	input := &EncryptedReasoningInput{Passthrough: declaration.Passthrough}
	if declaration.OnCallerMismatch != nil {
		input.OnCallerMismatch = *declaration.OnCallerMismatch
		input.HasOnCallerMismatch = true
	}
	if declaration.MatchMessages != nil {
		input.MatchMessages = *declaration.MatchMessages
		input.HasMatchMessages = true
	}
	return input
}

func CloneEncryptedReasoningInput(input *EncryptedReasoningInput) *EncryptedReasoningInput {
	if input == nil {
		return nil
	}
	out := *input
	if input.Passthrough != nil {
		value := *input.Passthrough
		out.Passthrough = &value
	}
	out.MatchMessages = slices.Clone(input.MatchMessages)
	if input.Expressions != nil {
		out.Expressions = make(map[string]string, len(input.Expressions))
		for name, expression := range input.Expressions {
			out.Expressions[name] = expression
		}
	}
	return &out
}

func RenderEncryptedReasoningBlock(input *EncryptedReasoningInput) string {
	if input == nil {
		return ""
	}
	if input.Block != "" {
		return strings.TrimRight(input.Block, "\n") + "\n"
	}
	expressions := make(map[string]string, len(input.Expressions)+3)
	if input.Passthrough != nil {
		expressions["passthrough"] = strconv.FormatBool(*input.Passthrough)
	}
	if input.HasOnCallerMismatch || input.OnCallerMismatch != "" {
		expressions["on_caller_mismatch"] = RenderStringOrExpression(input.OnCallerMismatch)
	}
	if input.HasMatchMessages || input.MatchMessages != nil {
		expressions["match_messages"] = RenderQuotedList(input.MatchMessages)
	}
	for name, expression := range input.Expressions {
		expressions[name] = expression
	}
	var b strings.Builder
	b.WriteString("encrypted_reasoning {\n")
	names := slices.Clone(encryptedReasoningAttributes)
	var extra []string
	for name := range expressions {
		if !slices.Contains(names, name) {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	for _, name := range append(names, extra...) {
		if expression, exists := expressions[name]; exists {
			b.WriteString("  " + name + " = " + expression + "\n")
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func renderEncryptedReasoningBlock(b *strings.Builder, input *EncryptedReasoningInput, indent string) {
	block := RenderEncryptedReasoningBlock(input)
	if block == "" {
		return
	}
	b.WriteString("\n" + indent)
	if input.Block != "" {
		b.WriteString(strings.TrimSpace(block))
		b.WriteString("\n")
		return
	}
	b.WriteString(strings.ReplaceAll(strings.TrimSuffix(block, "\n"), "\n", "\n"+indent))
	b.WriteString("\n")
}

func ParseEncryptedReasoningBlock(source string) (*EncryptedReasoningInput, error) {
	body, err := encryptedReasoningSyntax(source)
	if err != nil {
		return nil, err
	}
	if len(body.Attributes) != 0 || len(body.Blocks) != 1 || body.Blocks[0].Type != "encrypted_reasoning" || len(body.Blocks[0].Labels) != 0 {
		return nil, fmt.Errorf("expected one unlabeled encrypted_reasoning block")
	}
	block := body.Blocks[0]
	if len(block.Body.Blocks) != 0 {
		return nil, fmt.Errorf("encrypted_reasoning supports attributes only")
	}
	input := &EncryptedReasoningInput{Block: source, Expressions: make(map[string]string, len(block.Body.Attributes))}
	declaration := &config.EncryptedReasoningDeclaration{}
	for name, attr := range block.Body.Attributes {
		if !slices.Contains(encryptedReasoningAttributes, name) {
			return nil, fmt.Errorf("encrypted_reasoning has unsupported field %q", name)
		}
		input.Expressions[name] = source[attr.Expr.Range().Start.Byte:attr.Expr.Range().End.Byte]
		input.HasOnCallerMismatch = input.HasOnCallerMismatch || name == "on_caller_mismatch"
		input.HasMatchMessages = input.HasMatchMessages || name == "match_messages"
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !value.IsWhollyKnown() {
			continue
		}
		if value.IsNull() {
			return nil, fmt.Errorf("encrypted_reasoning.%s must not be null", name)
		}
		switch name {
		case "passthrough":
			if value.Type() != cty.Bool {
				return nil, fmt.Errorf("encrypted_reasoning.passthrough must be a boolean")
			}
			passthrough := value.True()
			input.Passthrough, declaration.Passthrough = &passthrough, &passthrough
		case "on_caller_mismatch":
			if value.Type() != cty.String {
				return nil, fmt.Errorf("encrypted_reasoning.on_caller_mismatch must be a string")
			}
			input.OnCallerMismatch = value.AsString()
			declaration.OnCallerMismatch = &input.OnCallerMismatch
		case "match_messages":
			if !value.Type().IsTupleType() && !value.Type().IsListType() {
				return nil, fmt.Errorf("encrypted_reasoning.match_messages must be a list of strings")
			}
			messages := make([]string, 0, value.LengthInt())
			for it := value.ElementIterator(); it.Next(); {
				_, message := it.Element()
				if message.IsNull() || message.Type() != cty.String {
					return nil, fmt.Errorf("encrypted_reasoning.match_messages must contain non-null strings")
				}
				messages = append(messages, message.AsString())
			}
			input.MatchMessages = messages
			declaration.MatchMessages = &messages
		}
	}
	if err := config.ValidateEncryptedReasoning(config.NewEncryptedReasoning(declaration), "config edit"); err != nil {
		return nil, err
	}
	return input, nil
}

func PatchEncryptedReasoningBlock(source string, expressions map[string]*string) (string, error) {
	if _, err := ParseEncryptedReasoningBlock(source); err != nil {
		return "", err
	}
	body, err := encryptedReasoningSyntax(source)
	if err != nil {
		return "", err
	}
	block := body.Blocks[0]
	if block.OpenBraceRange.Start.Line == block.CloseBraceRange.Start.Line {
		for name, expression := range expressions {
			if expression != nil && block.Body.Attributes[name] == nil {
				return PatchEncryptedReasoningBlock(multilineHCLBlock(source, block), expressions)
			}
		}
	}
	type replacement struct {
		start int
		end   int
		text  string
	}
	var replacements []replacement
	var additions strings.Builder
	indent := encryptedReasoningAttributeIndent(source, block)
	for name := range expressions {
		if !slices.Contains(encryptedReasoningAttributes, name) {
			return "", fmt.Errorf("encrypted_reasoning has unsupported field %q", name)
		}
	}
	for _, name := range encryptedReasoningAttributes {
		expression, changed := expressions[name]
		if !changed {
			continue
		}
		if expression != nil {
			if _, diags := hclsyntax.ParseExpression([]byte(*expression), "encrypted_reasoning.hcl", hcl.Pos{Line: 1, Column: 1}); diags.HasErrors() {
				return "", fmt.Errorf("encrypted_reasoning.%s: invalid expression: %s", name, diags.Error())
			}
		}
		attr := block.Body.Attributes[name]
		if attr == nil {
			if expression != nil {
				additions.WriteString(indent + name + " = " + *expression + "\n")
			}
			continue
		}
		attrRange, text := attr.Range(), ""
		if expression != nil {
			attrRange, text = attr.Expr.Range(), *expression
		}
		replacements = append(replacements, replacement{attrRange.Start.Byte, attrRange.End.Byte, text})
	}
	if additions.Len() > 0 {
		close := block.CloseBraceRange.Start.Byte
		lineStart := strings.LastIndex(source[:close], "\n") + 1
		if strings.TrimSpace(source[lineStart:close]) == "" {
			replacements = append(replacements, replacement{lineStart, lineStart, additions.String()})
		} else {
			blockIndent := hclBlockIndent(source, block.TypeRange.Start.Byte)
			replacements = append(replacements, replacement{close, close, "\n" + additions.String() + blockIndent})
		}
	}
	sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].start > replacements[j].start })
	out := source
	for _, replacement := range replacements {
		out = out[:replacement.start] + replacement.text + out[replacement.end:]
	}
	if _, err := ParseEncryptedReasoningBlock(out); err != nil {
		return "", err
	}
	return out, nil
}

func encryptedReasoningAttributeIndent(source string, block *hclsyntax.Block) string {
	for _, name := range encryptedReasoningAttributes {
		if attr := block.Body.Attributes[name]; attr != nil {
			start := attr.NameRange.Start.Byte
			lineStart := strings.LastIndex(source[:start], "\n") + 1
			if strings.TrimSpace(source[lineStart:start]) == "" {
				return source[lineStart:start]
			}
		}
	}
	return hclBlockIndent(source, block.TypeRange.Start.Byte) + "  "
}

func hclBlockIndent(source string, start int) string {
	lineStart := strings.LastIndex(source[:start], "\n") + 1
	end := lineStart
	for end < start && (source[end] == ' ' || source[end] == '\t') {
		end++
	}
	return source[lineStart:end]
}

func multilineHCLBlock(source string, block *hclsyntax.Block) string {
	open, close := block.OpenBraceRange.End.Byte, block.CloseBraceRange.Start.Byte
	indent := hclBlockIndent(source, block.TypeRange.Start.Byte)
	body := strings.TrimSpace(source[open:close])
	if body != "" {
		body = indent + "  " + body + "\n"
	}
	return source[:open] + "\n" + body + indent + source[close:]
}

func ApplyEncryptedReasoningEdit(input *EncryptedReasoningInput, edit EncryptedReasoningEdit) (*EncryptedReasoningInput, error) {
	expressions := make(map[string]*string, len(edit.Expressions)+3)
	if edit.Passthrough != nil {
		value := strconv.FormatBool(*edit.Passthrough)
		expressions["passthrough"] = &value
	}
	if edit.OnCallerMismatch != nil {
		value := strconv.Quote(*edit.OnCallerMismatch)
		expressions["on_caller_mismatch"] = &value
	}
	if edit.MatchMessages != nil {
		value := RenderQuotedList(*edit.MatchMessages)
		expressions["match_messages"] = &value
	}
	for name, expression := range edit.Expressions {
		if !slices.Contains(encryptedReasoningAttributes, name) {
			return nil, fmt.Errorf("encrypted_reasoning has unsupported field %q", name)
		}
		if _, exists := expressions[name]; exists {
			return nil, fmt.Errorf("encrypted_reasoning.%s has multiple edits", name)
		}
		expressions[name] = expression
	}
	if (edit.Remove || edit.Reset) && (len(expressions) != 0 || edit.Remove && edit.Reset) {
		return nil, fmt.Errorf("encrypted_reasoning remove/reset cannot be combined with other edits")
	}
	if edit.Remove {
		return nil, nil
	}
	if !edit.Reset && len(expressions) == 0 {
		return CloneEncryptedReasoningInput(input), nil
	}
	if input == nil && !edit.Reset {
		create := false
		for _, expression := range expressions {
			create = create || expression != nil
		}
		if !create {
			return nil, nil
		}
	}
	source := RenderEncryptedReasoningBlock(input)
	if source == "" {
		source = RenderEncryptedReasoningBlock(&EncryptedReasoningInput{})
	}
	if edit.Reset {
		for _, name := range encryptedReasoningAttributes {
			expressions[name] = nil
		}
	}
	out, err := PatchEncryptedReasoningBlock(source, expressions)
	if err != nil {
		return nil, err
	}
	return ParseEncryptedReasoningBlock(out)
}

func ReadEncryptedReasoning(source string, scope EncryptedReasoningScope) (*EncryptedReasoningInput, error) {
	owner, _, err := encryptedReasoningOwner(source, scope)
	if err != nil {
		return nil, err
	}
	block, err := findEncryptedReasoningBlock(owner)
	if err != nil || block == nil {
		return nil, err
	}
	return ParseEncryptedReasoningBlock(source[block.Range().Start.Byte:block.Range().End.Byte])
}

func UpdateEncryptedReasoning(source string, scope EncryptedReasoningScope, edit EncryptedReasoningEdit) (string, error) {
	owner, ownerBlock, err := encryptedReasoningOwner(source, scope)
	if err != nil {
		return "", err
	}
	block, err := findEncryptedReasoningBlock(owner)
	if err != nil {
		return "", err
	}
	var input *EncryptedReasoningInput
	if block != nil {
		input, err = ParseEncryptedReasoningBlock(source[block.Range().Start.Byte:block.Range().End.Byte])
		if err != nil {
			return "", err
		}
	}
	updated, err := ApplyEncryptedReasoningEdit(input, edit)
	if err != nil {
		return "", err
	}
	if updated == nil {
		if block == nil {
			return source, nil
		}
		remove := block
		if scope.Model != "" && len(owner.Attributes) == 0 && len(owner.Blocks) == 1 {
			provider, _, err := encryptedReasoningOwner(source, EncryptedReasoningScope{Provider: scope.Provider})
			if err != nil {
				return "", err
			}
			if extends := provider.Attributes["extends"]; extends != nil {
				value, diags := extends.Expr.Value(nil)
				if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String {
					return "", fmt.Errorf("cannot remove the last model overlay without a literal extends value")
				}
				if value.AsString() != "" {
					remove = ownerBlock
				}
			}
		}
		return source[:remove.Range().Start.Byte] + source[remove.Range().End.Byte:], nil
	}
	text := strings.TrimRight(RenderEncryptedReasoningBlock(updated), "\n")
	if block != nil {
		return source[:block.Range().Start.Byte] + text + source[block.Range().End.Byte:], nil
	}
	if ownerBlock == nil {
		return UpsertBlock(source, text, func(block TopLevelBlock) bool { return block.Type == "encrypted_reasoning" })
	}
	if ownerBlock.OpenBraceRange.Start.Line == ownerBlock.CloseBraceRange.Start.Line {
		return UpdateEncryptedReasoning(multilineHCLBlock(source, ownerBlock), scope, edit)
	}
	close := ownerBlock.CloseBraceRange.Start.Byte
	lineStart := strings.LastIndex(source[:close], "\n") + 1
	indent := source[lineStart:close]
	if strings.TrimSpace(indent) != "" {
		indent = hclBlockIndent(source, ownerBlock.TypeRange.Start.Byte)
		text = "\n" + indent + "  " + strings.ReplaceAll(text, "\n", "\n"+indent+"  ") + "\n" + indent
		return source[:close] + text + source[close:], nil
	}
	text = indent + "  " + strings.ReplaceAll(text, "\n", "\n"+indent+"  ") + "\n"
	return source[:lineStart] + text + source[lineStart:], nil
}

func encryptedReasoningSyntax(source string) (*hclsyntax.Body, error) {
	file, diags := hclsyntax.ParseConfig([]byte(source), "config.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, fmt.Errorf("parse config: %s", diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("parse config: unexpected body type")
	}
	return body, nil
}

func encryptedReasoningOwner(source string, scope EncryptedReasoningScope) (*hclsyntax.Body, *hclsyntax.Block, error) {
	if scope.Alias != "" && (scope.Provider != "" || scope.Model != "") || scope.Model != "" && scope.Provider == "" {
		return nil, nil, fmt.Errorf("encrypted_reasoning scope must be root, provider, model, or alias")
	}
	body, err := encryptedReasoningSyntax(source)
	if err != nil {
		return nil, nil, err
	}
	var owner *hclsyntax.Block
	find := func(kind, name string, labelCount int) error {
		owner = nil
		for _, block := range body.Blocks {
			if block.Type == kind && len(block.Labels) == labelCount && block.Labels[labelCount-1] == name {
				if owner != nil {
					return fmt.Errorf("multiple %s blocks named %q", kind, name)
				}
				owner = block
			}
		}
		if owner == nil {
			return fmt.Errorf("local %s block %q not found", kind, name)
		}
		body = owner.Body
		return nil
	}
	if scope.Alias != "" {
		err = find("alias", scope.Alias, 1)
	} else if scope.Provider != "" {
		err = find("provider", scope.Provider, 2)
		if err == nil && scope.Model != "" {
			err = find("model", scope.Model, 1)
		}
	}
	return body, owner, err
}

func findEncryptedReasoningBlock(body *hclsyntax.Body) (*hclsyntax.Block, error) {
	if body.Attributes["encrypted_reasoning"] != nil {
		return nil, fmt.Errorf("encrypted_reasoning must be an unlabeled block")
	}
	var found *hclsyntax.Block
	for _, block := range body.Blocks {
		if block.Type == "encrypted_reasoning" {
			if len(block.Labels) != 0 || found != nil {
				return nil, fmt.Errorf("expected at most one unlabeled encrypted_reasoning block")
			}
			found = block
		}
	}
	return found, nil
}
