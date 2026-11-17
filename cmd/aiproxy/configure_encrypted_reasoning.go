package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/configedit"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type encryptedReasoningOptions struct {
	Passthrough         bool
	HasPassthrough      bool
	OnCallerMismatch    string
	HasOnCallerMismatch bool
	MatchMessages       []string
	HasMatchMessages    bool
	Remove              bool
	Reset               bool
}

type modelReasoningOptions struct {
	Passthrough      []string
	OnCallerMismatch []string
	MatchMessages    []string
	Remove           []string
	Reset            []string
}

const encryptedReasoningHelp = "Encrypted reasoning uses the first whole block: explicit alias, selected model, provider, root, then built-ins. Omitted flags preserve the local block; omitted members of a new block use built-ins, not lower layers. --no-encrypted-reasoning removes the local block (inherit); --reset-encrypted-reasoning declares {} (passthrough=true, mismatch=fail, default patterns). Remove/reset cannot be combined with member edits at the same scope. Custom lists allow at most 16 distinct patterns of 4-256 bytes after lowercase/trim normalization; an empty list selects built-ins. Applies to chat completions and Responses, not established streams."

func addEncryptedReasoningFlags(cmd *cobra.Command, options *encryptedReasoningOptions) {
	cmd.Flags().BoolVar(&options.Passthrough, "encrypted-reasoning-passthrough", true, "forward reasoning history (use =false to strip; omitted preserves the local policy)")
	cmd.Flags().StringVar(&options.OnCallerMismatch, "encrypted-reasoning-on-caller-mismatch", "", "caller-mismatch policy: fail or strip_and_retry (explicit empty writes the fail default)")
	cmd.Flags().StringArrayVar(&options.MatchMessages, "encrypted-reasoning-match-message", nil, "error substring (repeatable; replaces this scope's list; a single empty value writes [] for built-ins)")
	cmd.Flags().BoolVar(&options.Remove, "no-encrypted-reasoning", false, "remove the local encrypted_reasoning block to inherit")
	cmd.Flags().BoolVar(&options.Reset, "reset-encrypted-reasoning", false, "declare an empty encrypted_reasoning {} block with built-in safe defaults")
}

func (options *encryptedReasoningOptions) readPresence(cmd *cobra.Command) {
	options.HasPassthrough = cmd.Flags().Changed("encrypted-reasoning-passthrough")
	options.HasOnCallerMismatch = cmd.Flags().Changed("encrypted-reasoning-on-caller-mismatch")
	options.HasMatchMessages = cmd.Flags().Changed("encrypted-reasoning-match-message")
}

func (options encryptedReasoningOptions) hasEdits() bool {
	return options.HasPassthrough || options.HasOnCallerMismatch || options.OnCallerMismatch != "" ||
		options.HasMatchMessages || options.MatchMessages != nil || options.Remove || options.Reset
}

func (options encryptedReasoningOptions) edit() (configedit.EncryptedReasoningEdit, error) {
	edit := configedit.EncryptedReasoningEdit{Remove: options.Remove, Reset: options.Reset}
	if options.HasPassthrough {
		edit.Passthrough = boolPtr(options.Passthrough)
	}
	if options.HasOnCallerMismatch || options.OnCallerMismatch != "" {
		value := options.OnCallerMismatch
		edit.OnCallerMismatch = &value
	}
	if options.HasMatchMessages || options.MatchMessages != nil {
		messages := append([]string{}, options.MatchMessages...)
		if len(messages) == 1 && messages[0] == "" {
			messages = []string{}
		}
		edit.MatchMessages = &messages
	}
	if _, err := configedit.ApplyEncryptedReasoningEdit(nil, edit); err != nil {
		return configedit.EncryptedReasoningEdit{}, err
	}
	return edit, nil
}

func addModelReasoningFlags(cmd *cobra.Command, options *modelReasoningOptions) {
	cmd.Flags().StringArrayVar(&options.Passthrough, "model-reasoning-passthrough", nil, "model policy spec: name=true|false (repeatable per model; never creates a model definition)")
	cmd.Flags().StringArrayVar(&options.OnCallerMismatch, "model-reasoning-on-caller-mismatch", nil, "model policy spec: name=fail|strip_and_retry (name= writes the fail default)")
	cmd.Flags().StringArrayVar(&options.MatchMessages, "model-reasoning-match-message", nil, "model pattern spec: name=substring (repeatable per model; replaces its list; a single name= writes [])")
	cmd.Flags().StringArrayVar(&options.Remove, "no-model-reasoning", nil, "model name whose local reasoning block should be removed (repeatable; inherit)")
	cmd.Flags().StringArrayVar(&options.Reset, "reset-model-reasoning", nil, "model name whose local reasoning block should become {} (repeatable; safe reset)")
}

func (options modelReasoningOptions) hasEdits() bool {
	return len(options.Passthrough)+len(options.OnCallerMismatch)+len(options.MatchMessages)+len(options.Remove)+len(options.Reset) > 0
}

func parseModelReasoningSpec(spec string) (string, string, error) {
	name, value, ok := strings.Cut(spec, "=")
	name = strings.TrimSpace(name)
	if !ok {
		return "", "", fmt.Errorf("model reasoning spec %q must be name=value", spec)
	}
	if err := validateModelName(name); err != nil {
		return "", "", fmt.Errorf("model reasoning spec %q: %w", spec, err)
	}
	return name, value, nil
}

func (options modelReasoningOptions) edits() (map[string]configedit.EncryptedReasoningEdit, error) {
	byName := make(map[string]encryptedReasoningOptions)
	for _, spec := range options.Passthrough {
		name, value, err := parseModelReasoningSpec(spec)
		if err != nil {
			return nil, err
		}
		if value != "true" && value != "false" {
			return nil, fmt.Errorf("model %q: reasoning passthrough must be true or false", name)
		}
		current := byName[name]
		if current.HasPassthrough {
			return nil, fmt.Errorf("model %q: duplicate reasoning passthrough edit", name)
		}
		current.Passthrough, current.HasPassthrough = value == "true", true
		byName[name] = current
	}
	for _, spec := range options.OnCallerMismatch {
		name, value, err := parseModelReasoningSpec(spec)
		if err != nil {
			return nil, err
		}
		current := byName[name]
		if current.HasOnCallerMismatch {
			return nil, fmt.Errorf("model %q: duplicate reasoning mismatch edit", name)
		}
		current.OnCallerMismatch, current.HasOnCallerMismatch = value, true
		byName[name] = current
	}
	for _, spec := range options.MatchMessages {
		name, value, err := parseModelReasoningSpec(spec)
		if err != nil {
			return nil, err
		}
		current := byName[name]
		current.MatchMessages = append(current.MatchMessages, value)
		current.HasMatchMessages = true
		byName[name] = current
	}
	for _, control := range []struct {
		names []string
		reset bool
	}{{options.Remove, false}, {options.Reset, true}} {
		for _, name := range control.names {
			name = strings.TrimSpace(name)
			if err := validateModelName(name); err != nil {
				return nil, fmt.Errorf("model reasoning control: %w", err)
			}
			current := byName[name]
			if control.reset {
				if current.Reset {
					return nil, fmt.Errorf("model %q: duplicate reasoning reset", name)
				}
				current.Reset = true
			} else {
				if current.Remove {
					return nil, fmt.Errorf("model %q: duplicate reasoning removal", name)
				}
				current.Remove = true
			}
			byName[name] = current
		}
	}
	edits := make(map[string]configedit.EncryptedReasoningEdit, len(byName))
	for _, name := range sortedReasoningModelNames(byName) {
		edit, err := byName[name].edit()
		if err != nil {
			return nil, fmt.Errorf("model %q: %w", name, err)
		}
		edits[name] = edit
	}
	return edits, nil
}

func sortedReasoningModelNames[T any](models map[string]T) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func reasoningOnlyFlags(cmd *cobra.Command) bool {
	only := true
	cmd.Flags().Visit(func(flag *pflag.Flag) {
		name := flag.Name
		if name == "config" || name == "name" || name == "non-interactive" ||
			strings.HasPrefix(name, "encrypted-reasoning-") || name == "no-encrypted-reasoning" || name == "reset-encrypted-reasoning" ||
			strings.HasPrefix(name, "model-reasoning-") || name == "no-model-reasoning" || name == "reset-model-reasoning" {
			return
		}
		only = false
	})
	return only
}

func optionalReasoningChoice(prompts *promptSession, label, description string, choices []string, def string) (string, error) {
	if !prompts.interactiveTUI {
		if _, err := prompts.in.Peek(1); errors.Is(err, io.EOF) {
			return def, nil
		}
	}
	return prompts.askChoiceWithDescription(label, description, choices, def)
}

func promptEncryptedReasoningEdit(prompts *promptSession, label string, existing *configedit.EncryptedReasoningInput, options encryptedReasoningOptions, nonInteractive bool) (configedit.EncryptedReasoningEdit, error) {
	edit, err := options.edit()
	if err != nil || nonInteractive || options.hasEdits() {
		return edit, err
	}
	action, err := optionalReasoningChoice(prompts, label+" encrypted reasoning", "Preserve keeps the local declaration and raw expressions. Inherit removes it; reset declares {}; override edits local members without copying lower layers.", []string{"preserve", "inherit", "reset", "override"}, "preserve")
	if err != nil {
		return edit, err
	}
	switch action {
	case "preserve":
		return edit, nil
	case "inherit":
		return configedit.EncryptedReasoningEdit{Remove: true}, nil
	case "reset":
		return configedit.EncryptedReasoningEdit{Reset: true}, nil
	}
	if existing != nil {
		_, _ = fmt.Fprintln(prompts.out, configedit.RenderEncryptedReasoningBlock(existing))
	}
	passthrough, err := prompts.askChoice("Reasoning passthrough", []string{"preserve", "true", "false"}, "preserve")
	if err != nil {
		return edit, err
	}
	if passthrough != "preserve" {
		edit.Passthrough = boolPtr(passthrough == "true")
	}
	mismatch, err := prompts.askChoice("Reasoning caller mismatch", []string{"preserve", "fail", "strip_and_retry", "default"}, "preserve")
	if err != nil {
		return edit, err
	}
	if mismatch != "preserve" {
		if mismatch == "default" {
			mismatch = ""
		}
		edit.OnCallerMismatch = &mismatch
	}
	patterns, err := prompts.askChoice("Reasoning match messages", []string{"preserve", "default", "replace"}, "preserve")
	if err != nil {
		return edit, err
	}
	if patterns != "preserve" {
		messages := []string{}
		if patterns == "replace" {
			for {
				message, err := prompts.askRequired("Reasoning error substring", "")
				if err != nil {
					return edit, err
				}
				messages = append(messages, message)
				more, err := prompts.askYesNo("Add another reasoning substring", false)
				if err != nil {
					return edit, err
				}
				if !more {
					break
				}
			}
		}
		edit.MatchMessages = &messages
	}
	if _, err := configedit.ApplyEncryptedReasoningEdit(existing, edit); err != nil {
		return configedit.EncryptedReasoningEdit{}, err
	}
	return edit, nil
}

func promptEncryptedReasoningInput(prompts *promptSession, label string, existing *configedit.EncryptedReasoningInput, options encryptedReasoningOptions, nonInteractive bool) (*configedit.EncryptedReasoningInput, error) {
	edit, err := promptEncryptedReasoningEdit(prompts, label, existing, options, nonInteractive)
	if err != nil {
		return nil, err
	}
	return configedit.ApplyEncryptedReasoningEdit(existing, edit)
}

func providerReasoningModelInventory(source, filename string, input providerInput) ([]string, error) {
	if input.Extends == "" {
		return providerModelNames(input.Models), nil
	}
	rt, err := config.Load([]byte(source), filename)
	if err != nil {
		return nil, fmt.Errorf("load reasoning base inventory: %w", err)
	}
	baseName := tlsClientHelloInputValue(input.Extends)
	base, ok := rt.Catalog.Provider(baseName)
	if !ok || base.Extends != "" || string(base.Type) != input.ProviderType || base.Name == input.Name {
		return nil, fmt.Errorf("provider %q: reasoning overlays require an enabled concrete base of the same type", input.Name)
	}
	names := make([]string, 0, len(base.Models))
	for _, model := range base.Models {
		names = append(names, model.Name)
	}
	return names, nil
}

func applyProviderModelReasoningEdit(input *providerInput, name string, edit configedit.EncryptedReasoningEdit, inventory []string) error {
	if !containsName(inventory, name) {
		return fmt.Errorf("provider %q: reasoning policy requires existing model %q in the resulting inventory", input.Name, name)
	}
	model := findProviderModel(input.Models, name)
	if model == nil {
		if input.Extends == "" {
			return fmt.Errorf("provider %q: local model %q not found", input.Name, name)
		}
		if edit.Remove {
			return nil
		}
		input.Models = append(input.Models, providerModelInput{Name: name})
		model = &input.Models[len(input.Models)-1]
	}
	updated, err := configedit.ApplyEncryptedReasoningEdit(model.EncryptedReasoning, edit)
	if err != nil {
		return fmt.Errorf("model %q: %w", name, err)
	}
	model.EncryptedReasoning = updated
	if input.Extends != "" && updated == nil && strings.TrimSpace(model.DeferredTransportBlock) == "" {
		input.Models = removeProviderModel(input.Models, name)
		return nil
	}
	input.Models, err = upsertProviderModel(input.Models, *model, name)
	return err
}

func configureProviderReasoningInput(prompts *promptSession, source string, input *providerInput, options providerOptions) error {
	policy, err := promptEncryptedReasoningInput(prompts, "Provider", input.EncryptedReasoning, options.Reasoning, options.NonInteractive)
	if err != nil {
		return err
	}
	input.EncryptedReasoning = policy
	edits, err := options.ModelReasoning.edits()
	if err != nil {
		return err
	}
	if len(edits) > 0 {
		inventory, err := providerReasoningModelInventory(source, options.ConfigFilename, *input)
		if err != nil {
			return err
		}
		for _, name := range sortedReasoningModelNames(edits) {
			if err := applyProviderModelReasoningEdit(input, name, edits[name], inventory); err != nil {
				return err
			}
		}
	}
	if options.NonInteractive || options.ModelReasoning.hasEdits() {
		return nil
	}
	more, err := optionalReasoningChoice(prompts, "Edit model reasoning policies", "Model overrides replace the whole lower-layer policy; inherited models receive sparse local overlays only.", []string{"no", "yes"}, "no")
	if err != nil || more == "no" {
		return err
	}
	inventory, err := providerReasoningModelInventory(source, options.ConfigFilename, *input)
	if err != nil {
		return err
	}
	if len(inventory) == 0 {
		return fmt.Errorf("provider %q: no models available for reasoning edits", input.Name)
	}
	for {
		name, err := prompts.askChoice("Reasoning model", inventory, inventory[0])
		if err != nil {
			return err
		}
		var existing *configedit.EncryptedReasoningInput
		if model := findProviderModel(input.Models, name); model != nil {
			existing = model.EncryptedReasoning
		}
		edit, err := promptEncryptedReasoningEdit(prompts, "Model "+name, existing, encryptedReasoningOptions{}, false)
		if err != nil {
			return err
		}
		if err := applyProviderModelReasoningEdit(input, name, edit, inventory); err != nil {
			return err
		}
		more, err := optionalReasoningChoice(prompts, "Edit another model reasoning policy", "", []string{"no", "yes"}, "no")
		if err != nil || more == "no" {
			return err
		}
	}
}

func updateProviderReasoningSource(source string, input providerInput, options providerOptions) (string, error) {
	edit, err := options.Reasoning.edit()
	if err != nil {
		return "", err
	}
	updated, err := configedit.UpdateEncryptedReasoning(source, configedit.EncryptedReasoningScope{Provider: input.Name}, edit)
	if err != nil {
		return "", err
	}
	edits, err := options.ModelReasoning.edits()
	if err != nil || len(edits) == 0 {
		return updated, err
	}
	inventory, err := providerReasoningModelInventory(source, options.ConfigFilename, input)
	if err != nil {
		return "", err
	}
	for _, name := range sortedReasoningModelNames(edits) {
		if !containsName(inventory, name) {
			return "", fmt.Errorf("provider %q: reasoning policy requires existing model %q in the resulting inventory", input.Name, name)
		}
		updated, err = updateProviderModelReasoningSource(updated, input.Name, name, input.Extends != "", edits[name])
		if err != nil {
			return "", err
		}
	}
	return updated, nil
}

func updateProviderModelReasoningSource(source, provider, model string, derived bool, edit configedit.EncryptedReasoningEdit) (string, error) {
	blocks, err := configedit.ParseTopLevelBlocks(source)
	if err != nil {
		return "", err
	}
	block := findBlock(blocks, func(block topLevelBlock) bool {
		return block.Type == "provider" && len(block.Labels) == 2 && block.Labels[1] == provider
	})
	if block == nil {
		return "", fmt.Errorf("local provider %q not found", provider)
	}
	owner, _, err := parseBlockSyntax(block.Text)
	if err != nil {
		return "", err
	}
	for _, local := range findNestedBlocks(owner.Body, "model") {
		if len(local.Labels) != 1 || local.Labels[0] != model {
			continue
		}
		if derived && edit.Remove && len(local.Body.Attributes) == 0 && len(local.Body.Blocks) == 1 && local.Body.Blocks[0].Type == "encrypted_reasoning" {
			start, end := block.Start+local.Range().Start.Byte, block.Start+local.Range().End.Byte
			return source[:start] + source[end:], nil
		}
		return configedit.UpdateEncryptedReasoning(source, configedit.EncryptedReasoningScope{Provider: provider, Model: model}, edit)
	}
	if !derived {
		return "", fmt.Errorf("provider %q: local model %q not found", provider, model)
	}
	policy, err := configedit.ApplyEncryptedReasoningEdit(nil, edit)
	if err != nil || policy == nil {
		return source, err
	}
	text := "model " + strconv.Quote(model) + " {\n  " + strings.ReplaceAll(strings.TrimSuffix(configedit.RenderEncryptedReasoningBlock(policy), "\n"), "\n", "\n  ") + "\n}"
	replacement := appendConfigureNestedBlock(block.Text, owner, text)
	return source[:block.Start] + replacement + source[block.End:], nil
}

func appendConfigureNestedBlock(source string, owner *hclsyntax.Block, text string) string {
	close := owner.CloseBraceRange.Start.Byte
	lineStart := strings.LastIndex(source[:close], "\n") + 1
	indent := source[lineStart:close]
	if strings.TrimSpace(indent) == "" {
		text = indent + "  " + strings.ReplaceAll(text, "\n", "\n"+indent+"  ") + "\n"
		return source[:lineStart] + text + source[lineStart:]
	}
	if owner.OpenBraceRange.Start.Line == owner.CloseBraceRange.Start.Line {
		open := owner.OpenBraceRange.End.Byte
		body := strings.TrimSpace(source[open:close])
		if body != "" {
			body = "  " + body + "\n"
		}
		return source[:open] + "\n" + body + "  " + strings.ReplaceAll(text, "\n", "\n  ") + "\n" + source[close:]
	}
	return source[:close] + "\n  " + strings.ReplaceAll(text, "\n", "\n  ") + "\n" + source[close:]
}

func writeReasoningOnlyConfig(prompts *promptSession, configPath, source, updated, label string) error {
	if err := validateGeneratedConfig([]byte(updated), configPath); err != nil {
		return err
	}
	if updated == source {
		_, _ = fmt.Fprintf(prompts.out, "no changes to %s in %s\n", label, configPath)
		return nil
	}
	if err := prompts.confirmWrite("Review "+label+" changes", buildReviewSummary([]string{"Config path: " + configPath, "Action: update " + label + " (other settings preserved)"}, "")); err != nil {
		return err
	}
	if err := writeConfigFile(configPath, updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "updated %s in %s\n", label, configPath)
	return nil
}
