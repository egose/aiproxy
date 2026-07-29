package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/cobra"
)

var envExprPattern = regexp.MustCompile(`^env\("[^"]+"\)$`)

type promptSession struct {
	in  *bufio.Reader
	out io.Writer
}

type topLevelBlock struct {
	Type   string
	Labels []string
	Start  int
	End    int
	Text   string
}

type providerInput struct {
	ProviderType string
	Name         string
	DisplayName  string
	BaseURL      string
	Credential   providerCredentialInput
	Models       []providerModelInput
}

type providerCredentialInput struct {
	Mode        string
	APIKeyValue string
	SecretsPath string
	SecretsKey  string
}

type providerModelInput struct {
	Name         string
	DisplayName  string
	UpstreamName string
	Capabilities []string
}

type listenerInput struct {
	Name       string
	Address    string
	ReadHeader string
	Idle       string
	Write      string
}

type authInput struct {
	Name      string
	Mode      string
	RateLimit *authRateLimitInput
	Clients   []authClientInput
}

type authRateLimitInput struct {
	RequestsPerMinute string
	Burst             string
}

type authClientInput struct {
	Name          string
	Token         string
	Tenant        string
	AllowedModels []string
}

type aliasInput struct {
	Name      string
	Algorithm string
	Targets   []aliasTargetInput
}

type aliasTargetInput struct {
	Provider string
	Model    string
}

type providerHealthInput struct {
	RedisURL  string
	KeyPrefix string
	Cooldown  string
}

type listenerOptions struct {
	Name           string
	Address        string
	ReadHeader     string
	Idle           string
	Write          string
	NonInteractive bool
}

type authOptions struct {
	Name                string
	Mode                string
	RateLimitRPM        string
	RateLimitBurst      string
	Clients             []string
	ClientTokens        []string
	ClientTokenEnvs     []string
	ClientTenants       []string
	ClientAllowedModels []string
	NonInteractive      bool
}

type providerOptions struct {
	ProviderType     string
	Name             string
	DisplayName      string
	BaseURL          string
	APIKey           string
	APIKeyEnv        string
	SecretsPath      string
	SecretsKey       string
	Models           []string
	ModelUpstreams   []string
	ModelDisplayName []string
	ModelCaps        []string
	NonInteractive   bool
}

type aliasOptions struct {
	Name           string
	Algorithm      string
	Targets        []string
	NonInteractive bool
}

type providerHealthOptions struct {
	RedisURL       string
	KeyPrefix      string
	Cooldown       string
	NonInteractive bool
}

func newConfigureCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Interactively create or update config blocks",
		Long:  "Create, update, or delete top-level config blocks. Run `aiproxy configure` for a guided selector, or call a specific subcommand such as `aiproxy configure provider`. Use `--non-interactive` on block subcommands to script changes in CI or provisioning workflows.",
		Example: "aiproxy configure\n" +
			"aiproxy configure provider\n" +
			"aiproxy configure provider --config /etc/aiproxy/config.hcl --non-interactive --name backup --type openai-compatible --base-url https://llm.internal/v1 --secrets-key localai --api-key \"$LOCALAI_API_KEY\" --model qwen3-32b\n" +
			"aiproxy configure alias --config /etc/aiproxy/config.hcl --non-interactive --name chat_default --target primary/gpt-4o-mini --target backup/qwen3-32b",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompts := promptSession{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
			choice, err := prompts.askChoice("Block to configure", []string{"provider", "alias", "auth", "listener", "provider-health"}, "provider")
			if err != nil {
				return err
			}
			resolvedConfigPath := cfgPath
			if resolvedConfigPath == "" {
				resolvedConfigPath = defaultConfigPath()
			}
			switch choice {
			case "listener":
				return runConfigureListener(&prompts, resolvedConfigPath, false, listenerOptions{})
			case "auth":
				return runConfigureAuth(&prompts, resolvedConfigPath, false, authOptions{})
			case "alias":
				return runConfigureAlias(&prompts, resolvedConfigPath, false, aliasOptions{})
			case "provider-health":
				return runConfigureProviderHealth(&prompts, resolvedConfigPath, false, providerHealthOptions{})
			default:
				return runConfigureProvider(&prompts, resolvedConfigPath, false, providerOptions{})
			}
		},
	}
	cmd.PersistentFlags().StringVar(&cfgPath, "config", "", "path to config file")
	cmd.AddCommand(newConfigureListenerCommand())
	cmd.AddCommand(newConfigureAuthCommand())
	cmd.AddCommand(newConfigureProviderCommand())
	cmd.AddCommand(newConfigureAliasCommand())
	cmd.AddCommand(newConfigureProviderHealthCommand())
	return cmd
}

func newConfigureListenerCommand() *cobra.Command {
	var deleteBlock bool
	var options listenerOptions
	cmd := &cobra.Command{
		Use:   "listener",
		Short: "Interactively create or replace the listener block",
		Example: "aiproxy configure listener\n" +
			"aiproxy configure listener --config /etc/aiproxy/config.hcl --non-interactive --name public --address :8080 --read-header-timeout 30s",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompts := promptSession{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
			return runConfigureListener(&prompts, inheritedConfigPath(cmd), deleteBlock, options)
		},
	}
	cmd.Flags().BoolVar(&deleteBlock, "delete", false, "delete the listener block")
	cmd.Flags().StringVar(&options.Name, "name", "", "listener name")
	cmd.Flags().StringVar(&options.Address, "address", "", "listener address")
	cmd.Flags().StringVar(&options.ReadHeader, "read-header-timeout", "", "listener read_header timeout")
	cmd.Flags().StringVar(&options.Idle, "idle-timeout", "", "listener idle timeout")
	cmd.Flags().StringVar(&options.Write, "write-timeout", "", "listener write timeout")
	cmd.Flags().BoolVar(&options.NonInteractive, "non-interactive", false, "fail instead of prompting for missing values")
	return cmd
}

func newConfigureAuthCommand() *cobra.Command {
	var deleteBlock bool
	var options authOptions
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Interactively create or replace the auth block",
		Example: "aiproxy configure auth\n" +
			"aiproxy configure auth --config /etc/aiproxy/config.hcl --non-interactive --name main --mode bearer_static --rate-limit-rpm 120 --client internal-app --client-token-env internal-app=AIPROXY_CLIENT_TOKEN",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompts := promptSession{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
			return runConfigureAuth(&prompts, inheritedConfigPath(cmd), deleteBlock, options)
		},
	}
	cmd.Flags().BoolVar(&deleteBlock, "delete", false, "delete the auth block")
	cmd.Flags().StringVar(&options.Name, "name", "", "auth block name")
	cmd.Flags().StringVar(&options.Mode, "mode", "", "auth mode")
	cmd.Flags().StringVar(&options.RateLimitRPM, "rate-limit-rpm", "", "rate_limit.requests_per_minute value")
	cmd.Flags().StringVar(&options.RateLimitBurst, "rate-limit-burst", "", "rate_limit.burst value")
	cmd.Flags().StringArrayVar(&options.Clients, "client", nil, "client name; repeat for multiple clients")
	cmd.Flags().StringArrayVar(&options.ClientTokens, "client-token", nil, "client token spec: name=value")
	cmd.Flags().StringArrayVar(&options.ClientTokenEnvs, "client-token-env", nil, "client token env spec: name=ENV_VAR")
	cmd.Flags().StringArrayVar(&options.ClientTenants, "client-tenant", nil, "client tenant spec: name=tenant")
	cmd.Flags().StringArrayVar(&options.ClientAllowedModels, "client-allowed-models", nil, "client allowed models spec: name=model1,model2")
	cmd.Flags().BoolVar(&options.NonInteractive, "non-interactive", false, "fail instead of prompting for missing values")
	return cmd
}

func newConfigureProviderCommand() *cobra.Command {
	var deleteBlock bool
	var options providerOptions
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Interactively create or update a provider block",
		Example: "aiproxy configure provider\n" +
			"aiproxy configure provider --config /etc/aiproxy/config.hcl --non-interactive --name backup --type openai-compatible --base-url https://llm.internal/v1 --secrets-key localai --api-key \"$LOCALAI_API_KEY\" --model qwen3-32b\n" +
			"aiproxy configure provider --config /etc/aiproxy/config.hcl --delete --name backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompts := promptSession{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
			return runConfigureProvider(&prompts, inheritedConfigPath(cmd), deleteBlock, options)
		},
	}
	cmd.Flags().BoolVar(&deleteBlock, "delete", false, "delete a provider block")
	cmd.Flags().StringVar(&options.Name, "name", "", "provider name")
	cmd.Flags().StringVar(&options.ProviderType, "type", "", "provider type")
	cmd.Flags().StringVar(&options.DisplayName, "display-name", "", "provider display_name")
	cmd.Flags().StringVar(&options.BaseURL, "base-url", "", "provider base_url")
	cmd.Flags().StringVar(&options.APIKey, "api-key", "", "provider API key or secret value")
	cmd.Flags().StringVar(&options.APIKeyEnv, "api-key-env", "", "provider API key environment variable name")
	cmd.Flags().StringVar(&options.SecretsPath, "secrets-path", "", "secrets file path for api_key_ref")
	cmd.Flags().StringVar(&options.SecretsKey, "secrets-key", "", "secrets file key for api_key_ref")
	cmd.Flags().StringArrayVar(&options.Models, "model", nil, "model spec: name or name=upstream")
	cmd.Flags().StringArrayVar(&options.ModelUpstreams, "model-upstream", nil, "model upstream spec: name=upstream")
	cmd.Flags().StringArrayVar(&options.ModelDisplayName, "model-display-name", nil, "model display name spec: name=display")
	cmd.Flags().StringArrayVar(&options.ModelCaps, "model-capabilities", nil, "model capabilities spec: name=cap1,cap2")
	cmd.Flags().BoolVar(&options.NonInteractive, "non-interactive", false, "fail instead of prompting for missing values")
	return cmd
}

func newConfigureAliasCommand() *cobra.Command {
	var deleteBlock bool
	var options aliasOptions
	cmd := &cobra.Command{
		Use:   "alias",
		Short: "Interactively create or update an alias block",
		Example: "aiproxy configure alias\n" +
			"aiproxy configure alias --config /etc/aiproxy/config.hcl --non-interactive --name chat_default --algorithm round_robin --target primary/gpt-4o-mini --target backup/qwen3-32b\n" +
			"aiproxy configure alias --config /etc/aiproxy/config.hcl --delete --name chat_default",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompts := promptSession{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
			return runConfigureAlias(&prompts, inheritedConfigPath(cmd), deleteBlock, options)
		},
	}
	cmd.Flags().BoolVar(&deleteBlock, "delete", false, "delete an alias block")
	cmd.Flags().StringVar(&options.Name, "name", "", "alias name")
	cmd.Flags().StringVar(&options.Algorithm, "algorithm", "", "alias routing algorithm")
	cmd.Flags().StringArrayVar(&options.Targets, "target", nil, "alias target spec: provider/model")
	cmd.Flags().BoolVar(&options.NonInteractive, "non-interactive", false, "fail instead of prompting for missing values")
	return cmd
}

func newConfigureProviderHealthCommand() *cobra.Command {
	var deleteBlock bool
	var options providerHealthOptions
	cmd := &cobra.Command{
		Use:   "provider-health",
		Short: "Interactively create or replace the provider_health block",
		Example: "aiproxy configure provider-health\n" +
			"aiproxy configure provider-health --config /etc/aiproxy/config.hcl --non-interactive --redis-url redis://localhost:6379/0 --key-prefix aiproxy:provider-health --cooldown 30s",
		RunE: func(cmd *cobra.Command, args []string) error {
			prompts := promptSession{in: bufio.NewReader(cmd.InOrStdin()), out: cmd.OutOrStdout()}
			return runConfigureProviderHealth(&prompts, inheritedConfigPath(cmd), deleteBlock, options)
		},
	}
	cmd.Flags().BoolVar(&deleteBlock, "delete", false, "delete the provider_health block")
	cmd.Flags().StringVar(&options.RedisURL, "redis-url", "", "provider_health.redis_url")
	cmd.Flags().StringVar(&options.KeyPrefix, "key-prefix", "", "provider_health.key_prefix")
	cmd.Flags().StringVar(&options.Cooldown, "cooldown", "", "provider_health.cooldown")
	cmd.Flags().BoolVar(&options.NonInteractive, "non-interactive", false, "fail instead of prompting for missing values")
	return cmd
}

func runConfigureListener(prompts *promptSession, configPath string, deleteBlock bool, options listenerOptions) error {
	var err error
	configPath, err = resolveConfigPath(prompts, configPath)
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(configPath)
	if err != nil {
		return err
	}
	if deleteBlock {
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool { return block.Type == "listener" })
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("listener block not found in %s", configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted listener block from %s\n", configPath)
		return nil
	}
	if hasBlockType(doc.blocks, "listener") && !options.NonInteractive {
		action, err := prompts.askChoice("Listener action", []string{"replace", "delete"}, "replace")
		if err != nil {
			return err
		}
		if action == "delete" {
			updated, _, err := removeBlock(doc.source, func(block topLevelBlock) bool { return block.Type == "listener" })
			if err != nil {
				return err
			}
			if err := writeConfigFile(configPath, updated); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(prompts.out, "deleted listener block from %s\n", configPath)
			return nil
		}
	}

	input, err := promptListenerInput(prompts, existingListenerInput(doc.blocks), options)
	if err != nil {
		return err
	}

	updated, err := upsertBlock(doc.source, renderListenerBlock(input), func(block topLevelBlock) bool {
		return block.Type == "listener"
	})
	if err != nil {
		return err
	}

	if err := writeConfigFile(configPath, updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "updated listener block in %s\n", configPath)
	return nil
}

func runConfigureAuth(prompts *promptSession, configPath string, deleteBlock bool, options authOptions) error {
	var err error
	configPath, err = resolveConfigPath(prompts, configPath)
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(configPath)
	if err != nil {
		return err
	}
	if deleteBlock {
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool { return block.Type == "auth" })
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("auth block not found in %s", configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted auth block from %s\n", configPath)
		return nil
	}
	if hasBlockType(doc.blocks, "auth") && !options.NonInteractive {
		action, err := prompts.askChoice("Auth action", []string{"replace", "delete"}, "replace")
		if err != nil {
			return err
		}
		if action == "delete" {
			updated, _, err := removeBlock(doc.source, func(block topLevelBlock) bool { return block.Type == "auth" })
			if err != nil {
				return err
			}
			if err := writeConfigFile(configPath, updated); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(prompts.out, "deleted auth block from %s\n", configPath)
			return nil
		}
	}

	input, err := promptAuthInput(prompts, existingAuthInput(doc.blocks), options)
	if err != nil {
		return err
	}

	updated, err := upsertBlock(doc.source, renderAuthBlock(input), func(block topLevelBlock) bool {
		return block.Type == "auth"
	})
	if err != nil {
		return err
	}

	if err := writeConfigFile(configPath, updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "updated auth block in %s\n", configPath)
	return nil
}

func runConfigureProvider(prompts *promptSession, configPath string, deleteBlock bool, options providerOptions) error {
	var err error
	configPath, err = resolveConfigPath(prompts, configPath)
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(configPath)
	if err != nil {
		return err
	}

	providerNames := providerBlockNames(doc.blocks)
	selectedName := options.Name
	if deleteBlock {
		name, err := selectNamedBlock(prompts, "Provider to delete", providerNames, selectedName)
		if err != nil {
			return err
		}
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool {
			return block.Type == "provider" && len(block.Labels) >= 2 && block.Labels[1] == name
		})
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("provider %q not found in %s", name, configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted provider %q from %s\n", name, configPath)
		return nil
	}

	defaultAction := actionFromExistingName(selectedName, providerNames)
	action := defaultAction
	if len(providerNames) > 0 {
		if !options.NonInteractive {
			action, err = prompts.askChoice("Provider action", []string{"create", "update", "delete"}, defaultAction)
			if err != nil {
				return err
			}
		}
	}

	nameForAction := selectedName
	if action == "update" || action == "delete" {
		if options.NonInteractive && nameForAction == "" {
			return fmt.Errorf("provider update/delete requires --name in non-interactive mode")
		}
		nameForAction, err = selectNamedBlock(prompts, strings.Title(action)+" provider", providerNames, nameForAction)
		if err != nil {
			return err
		}
	}
	if action == "delete" {
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool {
			return block.Type == "provider" && len(block.Labels) >= 2 && block.Labels[1] == nameForAction
		})
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("provider %q not found in %s", nameForAction, configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted provider %q from %s\n", nameForAction, configPath)
		return nil
	}

	input, secretsUpdate, err := promptProviderInput(prompts, existingProviderInput(doc.blocks, nameForAction), options)
	if err != nil {
		return err
	}

	updated, err := upsertBlock(doc.source, renderProviderBlock(input), func(block topLevelBlock) bool {
		return block.Type == "provider" && len(block.Labels) >= 2 && block.Labels[1] == targetProviderName(action, nameForAction, input.Name)
	})
	if err != nil {
		return err
	}

	if err := writeSecretsUpdate(secretsUpdate); err != nil {
		return err
	}
	if err := writeConfigFile(configPath, updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "updated provider %q in %s\n", input.Name, configPath)
	if secretsUpdate.path != "" {
		_, _ = fmt.Fprintf(prompts.out, "updated secrets key %q in %s\n", secretsUpdate.key, secretsUpdate.path)
	}
	return nil
}

func runConfigureAlias(prompts *promptSession, configPath string, deleteBlock bool, options aliasOptions) error {
	var err error
	configPath, err = resolveConfigPath(prompts, configPath)
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(configPath)
	if err != nil {
		return err
	}

	aliasNames := aliasBlockNames(doc.blocks)
	selectedName := options.Name
	if deleteBlock {
		name, err := selectNamedBlock(prompts, "Alias to delete", aliasNames, selectedName)
		if err != nil {
			return err
		}
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool {
			return block.Type == "alias" && len(block.Labels) >= 1 && block.Labels[0] == name
		})
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("alias %q not found in %s", name, configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted alias %q from %s\n", name, configPath)
		return nil
	}

	defaultAction := actionFromExistingName(selectedName, aliasNames)
	action := defaultAction
	if len(aliasNames) > 0 {
		if !options.NonInteractive {
			action, err = prompts.askChoice("Alias action", []string{"create", "update", "delete"}, defaultAction)
			if err != nil {
				return err
			}
		}
	}

	nameForAction := selectedName
	if action == "update" || action == "delete" {
		if options.NonInteractive && nameForAction == "" {
			return fmt.Errorf("alias update/delete requires --name in non-interactive mode")
		}
		nameForAction, err = selectNamedBlock(prompts, strings.Title(action)+" alias", aliasNames, nameForAction)
		if err != nil {
			return err
		}
	}
	if action == "delete" {
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool {
			return block.Type == "alias" && len(block.Labels) >= 1 && block.Labels[0] == nameForAction
		})
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("alias %q not found in %s", nameForAction, configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted alias %q from %s\n", nameForAction, configPath)
		return nil
	}

	input, err := promptAliasInput(prompts, doc.blocks, existingAliasInput(doc.blocks, nameForAction), options)
	if err != nil {
		return err
	}

	updated, err := upsertBlock(doc.source, renderAliasBlock(input), func(block topLevelBlock) bool {
		return block.Type == "alias" && len(block.Labels) >= 1 && block.Labels[0] == targetAliasName(action, nameForAction, input.Name)
	})
	if err != nil {
		return err
	}

	if err := writeConfigFile(configPath, updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "updated alias %q in %s\n", input.Name, configPath)
	return nil
}

func runConfigureProviderHealth(prompts *promptSession, configPath string, deleteBlock bool, options providerHealthOptions) error {
	var err error
	configPath, err = resolveConfigPath(prompts, configPath)
	if err != nil {
		return err
	}
	doc, err := loadConfigDocument(configPath)
	if err != nil {
		return err
	}
	if deleteBlock {
		updated, removed, err := removeBlock(doc.source, func(block topLevelBlock) bool { return block.Type == "provider_health" })
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("provider_health block not found in %s", configPath)
		}
		if err := writeConfigFile(configPath, updated); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(prompts.out, "deleted provider_health block from %s\n", configPath)
		return nil
	}
	if hasBlockType(doc.blocks, "provider_health") && !options.NonInteractive {
		action, err := prompts.askChoice("Provider health action", []string{"replace", "delete"}, "replace")
		if err != nil {
			return err
		}
		if action == "delete" {
			updated, _, err := removeBlock(doc.source, func(block topLevelBlock) bool { return block.Type == "provider_health" })
			if err != nil {
				return err
			}
			if err := writeConfigFile(configPath, updated); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(prompts.out, "deleted provider_health block from %s\n", configPath)
			return nil
		}
	}

	input, err := promptProviderHealthInput(prompts, existingProviderHealthInput(doc.blocks), options)
	if err != nil {
		return err
	}

	updated, err := upsertBlock(doc.source, renderProviderHealthBlock(input), func(block topLevelBlock) bool {
		return block.Type == "provider_health"
	})
	if err != nil {
		return err
	}

	if err := writeConfigFile(configPath, updated); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "updated provider_health block in %s\n", configPath)
	return nil
}

type configDocument struct {
	source string
	blocks []topLevelBlock
}

func inheritedConfigPath(cmd *cobra.Command) string {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return ""
	}
	return path
}

func resolveConfigPath(prompts *promptSession, configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	return prompts.askRequired("Config path", defaultConfigPath())
}

func loadConfigDocument(path string) (*configDocument, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &configDocument{}, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	blocks, err := parseTopLevelBlocks(string(src))
	if err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &configDocument{source: string(src), blocks: blocks}, nil
}

func hasBlockType(blocks []topLevelBlock, blockType string) bool {
	for _, block := range blocks {
		if block.Type == blockType {
			return true
		}
	}
	return false
}

func findBlock(blocks []topLevelBlock, match func(topLevelBlock) bool) *topLevelBlock {
	for _, block := range blocks {
		if match(block) {
			copyBlock := block
			return &copyBlock
		}
	}
	return nil
}

func selectNamedBlock(prompts *promptSession, label string, names []string, selectedName string) (string, error) {
	if len(names) == 0 {
		return "", fmt.Errorf("%s: no existing blocks found", label)
	}
	if selectedName != "" {
		for _, name := range names {
			if name == selectedName {
				return selectedName, nil
			}
		}
		return "", fmt.Errorf("%s %q not found", strings.ToLower(label), selectedName)
	}
	return prompts.askChoice(label, names, names[0])
}

func actionFromExistingName(name string, existing []string) string {
	if name != "" && containsName(existing, name) {
		return "update"
	}
	return "create"
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func hasAuthClientOptions(options authOptions) bool {
	return len(options.Clients) > 0 || len(options.ClientTokens) > 0 || len(options.ClientTokenEnvs) > 0 || len(options.ClientTenants) > 0 || len(options.ClientAllowedModels) > 0
}

func buildAuthClientsFromOptions(options authOptions) ([]authClientInput, error) {
	ordered := append([]string(nil), options.Clients...)
	seen := make(map[string]bool)
	for _, name := range ordered {
		seen[name] = true
	}
	tokens, err := parseNamedValueSpecs(options.ClientTokens)
	if err != nil {
		return nil, err
	}
	tokenEnvs, err := parseNamedValueSpecs(options.ClientTokenEnvs)
	if err != nil {
		return nil, err
	}
	tenants, err := parseNamedValueSpecs(options.ClientTenants)
	if err != nil {
		return nil, err
	}
	allowedModels, err := parseNamedCSVSpecs(options.ClientAllowedModels)
	if err != nil {
		return nil, err
	}
	for name := range tokens {
		if !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}
	for name := range tokenEnvs {
		if !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
		if _, ok := tokens[name]; ok {
			return nil, fmt.Errorf("client %q has both --client-token and --client-token-env", name)
		}
	}
	for name := range tenants {
		if !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}
	for name := range allowedModels {
		if !seen[name] {
			ordered = append(ordered, name)
			seen[name] = true
		}
	}
	clients := make([]authClientInput, 0, len(ordered))
	for _, name := range ordered {
		client := authClientInput{Name: name, Tenant: tenants[name], AllowedModels: allowedModels[name]}
		if token, ok := tokens[name]; ok {
			client.Token = token
		}
		if envName, ok := tokenEnvs[name]; ok {
			client.Token = fmt.Sprintf(`env("%s")`, envName)
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func applyProviderCredentialOptions(input *providerInput, options providerOptions) error {
	credentialModes := 0
	useSecrets := options.SecretsKey != "" || options.SecretsPath != "" // pragma: allowlist secret
	if options.APIKey != "" && !useSecrets {
		credentialModes++
	}
	if options.APIKeyEnv != "" {
		credentialModes++
	}
	if useSecrets {
		credentialModes++
	}
	if credentialModes > 1 {
		return fmt.Errorf("provider flags may set only one credential source")
	}
	if useSecrets {
		input.Credential.Mode = "secrets_file"
		if options.SecretsPath != "" {
			input.Credential.SecretsPath = options.SecretsPath // pragma: allowlist secret
		}
		if input.Credential.SecretsPath == "" {
			input.Credential.SecretsPath = defaultSecretsPath()
		}
		if options.SecretsKey != "" {
			input.Credential.SecretsKey = options.SecretsKey // pragma: allowlist secret
		}
		if input.Credential.SecretsKey == "" && input.Name != "" {
			input.Credential.SecretsKey = input.Name // pragma: allowlist secret
		}
		input.Credential.APIKeyValue = ""
		return nil
	}
	if options.APIKeyEnv != "" {
		input.Credential = providerCredentialInput{Mode: "env_expression", APIKeyValue: fmt.Sprintf(`env("%s")`, options.APIKeyEnv)}
		return nil
	}
	if options.APIKey != "" {
		input.Credential = providerCredentialInput{Mode: "inline", APIKeyValue: options.APIKey}
	}
	return nil
}

func providerSecretsUpdate(input providerInput, options providerOptions) secretsUpdate {
	if input.Credential.Mode != "secrets_file" || options.APIKey == "" {
		return secretsUpdate{}
	}
	return secretsUpdate{path: input.Credential.SecretsPath, key: input.Credential.SecretsKey, value: options.APIKey}
}

func hasProviderModelOptions(options providerOptions) bool {
	return len(options.Models) > 0 || len(options.ModelUpstreams) > 0 || len(options.ModelDisplayName) > 0 || len(options.ModelCaps) > 0
}

func buildProviderModelsFromOptions(providerType string, options providerOptions) ([]providerModelInput, error) {
	ordered := make([]string, 0, len(options.Models))
	models := make(map[string]*providerModelInput)
	for _, spec := range options.Models {
		name, upstream, err := parseModelSpec(spec)
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, name)
		models[name] = &providerModelInput{Name: name, UpstreamName: upstream, Capabilities: append([]string(nil), defaultCapabilities(providerType)...)}
	}
	upstreams, err := parseNamedValueSpecs(options.ModelUpstreams)
	if err != nil {
		return nil, err
	}
	displayNames, err := parseNamedValueSpecs(options.ModelDisplayName)
	if err != nil {
		return nil, err
	}
	capabilities, err := parseNamedCSVSpecs(options.ModelCaps)
	if err != nil {
		return nil, err
	}
	for name, upstream := range upstreams {
		model := ensureProviderModel(models, &ordered, name, providerType)
		model.UpstreamName = upstream
	}
	for name, display := range displayNames {
		model := ensureProviderModel(models, &ordered, name, providerType)
		model.DisplayName = display
	}
	for name, caps := range capabilities {
		model := ensureProviderModel(models, &ordered, name, providerType)
		model.Capabilities = caps
	}
	out := make([]providerModelInput, 0, len(ordered))
	for _, name := range ordered {
		model := *models[name]
		if model.UpstreamName == "" {
			model.UpstreamName = model.Name
		}
		if len(model.Capabilities) == 0 {
			model.Capabilities = append([]string(nil), defaultCapabilities(providerType)...)
		}
		out = append(out, model)
	}
	return out, nil
}

func ensureProviderModel(models map[string]*providerModelInput, ordered *[]string, name, providerType string) *providerModelInput {
	if model, ok := models[name]; ok {
		return model
	}
	model := &providerModelInput{Name: name, UpstreamName: name, Capabilities: append([]string(nil), defaultCapabilities(providerType)...)}
	models[name] = model
	*ordered = append(*ordered, name)
	return model
}

func buildAliasTargetsFromOptions(specs []string) ([]aliasTargetInput, error) {
	targets := make([]aliasTargetInput, 0, len(specs))
	for _, spec := range specs {
		parts := strings.SplitN(spec, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid target spec %q, want provider/model", spec)
		}
		targets = append(targets, aliasTargetInput{Provider: parts[0], Model: parts[1]})
	}
	return targets, nil
}

func parseNamedValueSpecs(specs []string) (map[string]string, error) {
	out := make(map[string]string, len(specs))
	for _, spec := range specs {
		name, value, err := splitNamedSpec(spec)
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseNamedCSVSpecs(specs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(specs))
	for _, spec := range specs {
		name, value, err := splitNamedSpec(spec)
		if err != nil {
			return nil, err
		}
		out[name] = splitCSV(value)
	}
	return out, nil
}

func splitNamedSpec(spec string) (string, string, error) {
	parts := strings.SplitN(spec, "=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return "", "", fmt.Errorf("invalid spec %q, want name=value", spec)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func parseModelSpec(spec string) (string, string, error) {
	name, upstream, err := splitNamedSpec(spec)
	if err == nil {
		return name, upstream, nil
	}
	if strings.TrimSpace(spec) == "" {
		return "", "", fmt.Errorf("invalid model spec %q", spec)
	}
	trimmed := strings.TrimSpace(spec)
	return trimmed, trimmed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

type secretsUpdate struct {
	path  string
	key   string
	value string
}

func writeSecretsUpdate(update secretsUpdate) error {
	if update.path == "" {
		return nil
	}
	secrets, err := readSecretsFile(update.path)
	if err != nil {
		return err
	}
	secrets[update.key] = update.value
	body, err := json.MarshalIndent(secrets, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal secrets %s: %w", update.path, err)
	}
	body = append(body, '\n')
	return writeFile(update.path, body, 0o600)
}

func readSecretsFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read secrets %s: %w", path, err)
	}
	var out map[string]string
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse secrets %s: %w", path, err)
	}
	if out == nil {
		out = map[string]string{}
	}
	return out, nil
}

func promptListenerInput(prompts *promptSession, existing *listenerInput, options listenerOptions) (listenerInput, error) {
	defaults := listenerInput{Name: "public", Address: ":8080"}
	if existing != nil {
		defaults = *existing
	}
	if options.Name != "" {
		defaults.Name = options.Name
	}
	if options.Address != "" {
		defaults.Address = options.Address
	}
	if options.ReadHeader != "" {
		defaults.ReadHeader = options.ReadHeader
	}
	if options.Idle != "" {
		defaults.Idle = options.Idle
	}
	if options.Write != "" {
		defaults.Write = options.Write
	}
	if options.NonInteractive {
		if defaults.Name == "" || defaults.Address == "" {
			return listenerInput{}, fmt.Errorf("listener requires name and address in non-interactive mode")
		}
		return defaults, nil
	}
	name, err := prompts.askRequired("Listener name", defaults.Name)
	if err != nil {
		return listenerInput{}, err
	}
	address, err := prompts.askRequired("Listener address", defaults.Address)
	if err != nil {
		return listenerInput{}, err
	}
	configureTimeouts, err := prompts.askYesNo("Configure listener timeouts", defaults.ReadHeader != "" || defaults.Idle != "" || defaults.Write != "")
	if err != nil {
		return listenerInput{}, err
	}
	input := listenerInput{Name: name, Address: address}
	if configureTimeouts {
		readHeaderDefault := defaults.ReadHeader
		if readHeaderDefault == "" {
			readHeaderDefault = "30s"
		}
		input.ReadHeader, err = prompts.ask("read_header timeout", readHeaderDefault)
		if err != nil {
			return listenerInput{}, err
		}
		input.Idle, err = prompts.ask("idle timeout", defaults.Idle)
		if err != nil {
			return listenerInput{}, err
		}
		input.Write, err = prompts.ask("write timeout", defaults.Write)
		if err != nil {
			return listenerInput{}, err
		}
	}
	return input, nil
}

func promptAuthInput(prompts *promptSession, existing *authInput, options authOptions) (authInput, error) {
	defaults := authInput{Name: "main", Mode: "none"}
	if existing != nil {
		defaults = *existing
	}
	if options.Name != "" {
		defaults.Name = options.Name
	}
	if options.Mode != "" {
		defaults.Mode = options.Mode
	}
	if options.RateLimitRPM != "" || options.RateLimitBurst != "" {
		rpm := options.RateLimitRPM
		if rpm == "" && defaults.RateLimit != nil {
			rpm = defaults.RateLimit.RequestsPerMinute
		}
		burst := options.RateLimitBurst
		if burst == "" {
			if defaults.RateLimit != nil && defaults.RateLimit.Burst != "" {
				burst = defaults.RateLimit.Burst
			} else {
				burst = rpm
			}
		}
		defaults.RateLimit = &authRateLimitInput{RequestsPerMinute: rpm, Burst: burst}
	}
	if hasAuthClientOptions(options) {
		clients, err := buildAuthClientsFromOptions(options)
		if err != nil {
			return authInput{}, err
		}
		defaults.Clients = clients
	}
	if options.NonInteractive {
		if defaults.Name == "" || defaults.Mode == "" {
			return authInput{}, fmt.Errorf("auth requires name and mode in non-interactive mode")
		}
		if defaults.Mode == "bearer_static" && len(defaults.Clients) == 0 {
			return authInput{}, fmt.Errorf("auth bearer_static requires at least one client in non-interactive mode")
		}
		return defaults, nil
	}
	name, err := prompts.askRequired("Auth block name", defaults.Name)
	if err != nil {
		return authInput{}, err
	}
	mode, err := prompts.askChoice("Auth mode", []string{"none", "bearer_static"}, defaults.Mode)
	if err != nil {
		return authInput{}, err
	}
	input := authInput{Name: name, Mode: mode}
	configureRateLimit, err := prompts.askYesNo("Configure rate limit", defaults.RateLimit != nil)
	if err != nil {
		return authInput{}, err
	}
	if configureRateLimit {
		rpmDefault := "120"
		burstDefault := rpmDefault
		if defaults.RateLimit != nil {
			rpmDefault = defaults.RateLimit.RequestsPerMinute
			if defaults.RateLimit.Burst != "" {
				burstDefault = defaults.RateLimit.Burst
			}
		}
		rpm, err := prompts.askRequired("Requests per minute", rpmDefault)
		if err != nil {
			return authInput{}, err
		}
		burst, err := prompts.ask("Burst", burstDefault)
		if err != nil {
			return authInput{}, err
		}
		input.RateLimit = &authRateLimitInput{RequestsPerMinute: rpm, Burst: burst}
	}
	if mode == "bearer_static" {
		if len(defaults.Clients) > 0 {
			keepClients, err := prompts.askYesNo("Keep existing clients", true)
			if err != nil {
				return authInput{}, err
			}
			if keepClients {
				input.Clients = append(input.Clients, defaults.Clients...)
				return input, nil
			}
		}
		for {
			clientName, err := prompts.askRequired("Client name", "")
			if err != nil {
				return authInput{}, err
			}
			token, err := prompts.askRequired("Client token or env(\"VAR\") expression", "")
			if err != nil {
				return authInput{}, err
			}
			tenant, err := prompts.ask("Client tenant", "")
			if err != nil {
				return authInput{}, err
			}
			allowedModels, err := prompts.askCSV("Allowed models (comma-separated)", "")
			if err != nil {
				return authInput{}, err
			}
			input.Clients = append(input.Clients, authClientInput{Name: clientName, Token: token, Tenant: tenant, AllowedModels: allowedModels})
			more, err := prompts.askYesNo("Add another client", false)
			if err != nil {
				return authInput{}, err
			}
			if !more {
				break
			}
		}
	}
	return input, nil
}

func promptProviderInput(prompts *promptSession, existing *providerInput, options providerOptions) (providerInput, secretsUpdate, error) {
	defaults := providerInput{ProviderType: "openai"}
	if existing != nil {
		defaults = *existing
	}
	if options.ProviderType != "" {
		defaults.ProviderType = options.ProviderType
	}
	if options.Name != "" {
		defaults.Name = options.Name
	}
	if options.DisplayName != "" {
		defaults.DisplayName = options.DisplayName
	}
	if options.BaseURL != "" {
		defaults.BaseURL = options.BaseURL
	}
	if err := applyProviderCredentialOptions(&defaults, options); err != nil {
		return providerInput{}, secretsUpdate{}, err
	}
	if hasProviderModelOptions(options) {
		models, err := buildProviderModelsFromOptions(defaults.ProviderType, options)
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
		defaults.Models = models
	}
	if options.NonInteractive {
		if defaults.ProviderType == "" || defaults.Name == "" {
			return providerInput{}, secretsUpdate{}, fmt.Errorf("provider requires type and name in non-interactive mode")
		}
		if defaults.ProviderType == "openai-compatible" && defaults.BaseURL == "" {
			return providerInput{}, secretsUpdate{}, fmt.Errorf("provider type openai-compatible requires --base-url in non-interactive mode")
		}
		if defaults.Credential.Mode == "" {
			return providerInput{}, secretsUpdate{}, fmt.Errorf("provider requires credential flags or existing credentials in non-interactive mode")
		}
		if len(defaults.Models) == 0 {
			return providerInput{}, secretsUpdate{}, fmt.Errorf("provider requires at least one model in non-interactive mode")
		}
		return defaults, providerSecretsUpdate(defaults, options), nil
	}
	providerType, err := prompts.askChoice("Provider type", []string{"openai", "openai-compatible", "anthropic", "gemini"}, defaults.ProviderType)
	if err != nil {
		return providerInput{}, secretsUpdate{}, err
	}
	providerName, err := prompts.askRequired("Provider name", defaults.Name)
	if err != nil {
		return providerInput{}, secretsUpdate{}, err
	}
	displayName, err := prompts.ask("Display name", defaults.DisplayName)
	if err != nil {
		return providerInput{}, secretsUpdate{}, err
	}
	baseURL := defaults.BaseURL
	if providerType == "openai-compatible" {
		if baseURL == "" {
			baseURL = "https://llm.internal/v1"
		}
		baseURL, err = prompts.askRequired("Base URL", baseURL)
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
	} else {
		baseURL = ""
	}
	credentialDefault := "secrets_file"
	if defaults.Credential.Mode != "" {
		credentialDefault = defaults.Credential.Mode
	}
	credentialMode, err := prompts.askChoice("Credential storage", []string{"secrets_file", "env_expression", "inline"}, credentialDefault)
	if err != nil {
		return providerInput{}, secretsUpdate{}, err
	}
	credential := providerCredentialInput{Mode: credentialMode}
	update := secretsUpdate{}
	switch credentialMode {
	case "secrets_file":
		secretsPathDefault := defaults.Credential.SecretsPath // pragma: allowlist secret
		if secretsPathDefault == "" {
			secretsPathDefault = defaultSecretsPath()
		}
		credential.SecretsPath, err = prompts.askRequired("Secrets path", secretsPathDefault)
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
		secretsKeyDefault := defaults.Credential.SecretsKey // pragma: allowlist secret
		if secretsKeyDefault == "" {
			secretsKeyDefault = providerName // pragma: allowlist secret
		}
		credential.SecretsKey, err = prompts.askRequired("Secrets key", secretsKeyDefault)
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
		apiKey, err := prompts.askRequired("API key value", "")
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
		update = secretsUpdate{path: credential.SecretsPath, key: credential.SecretsKey, value: apiKey}
	case "env_expression":
		defaultEnv := defaultProviderEnvExpression(providerType)
		if defaults.Credential.Mode == "env_expression" && defaults.Credential.APIKeyValue != "" {
			defaultEnv = defaults.Credential.APIKeyValue
		}
		credential.APIKeyValue, err = prompts.askRequired("API key env expression", defaultEnv)
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
	case "inline":
		credential.APIKeyValue, err = prompts.askRequired("API key value", defaults.Credential.APIKeyValue)
		if err != nil {
			return providerInput{}, secretsUpdate{}, err
		}
	}
	models, err := promptProviderModels(prompts, providerType, defaults.Models)
	if err != nil {
		return providerInput{}, secretsUpdate{}, err
	}
	return providerInput{
		ProviderType: providerType,
		Name:         providerName,
		DisplayName:  displayName,
		BaseURL:      baseURL,
		Credential:   credential,
		Models:       models,
	}, update, nil
}

func promptProviderModels(prompts *promptSession, providerType string, existing []providerModelInput) ([]providerModelInput, error) {
	if len(existing) > 0 {
		keepModels, err := prompts.askYesNo("Keep existing models", true)
		if err != nil {
			return nil, err
		}
		if keepModels {
			return append([]providerModelInput(nil), existing...), nil
		}
	}
	var models []providerModelInput
	defaultCaps := defaultCapabilities(providerType)
	for {
		name, err := prompts.askRequired("Model name", "")
		if err != nil {
			return nil, err
		}
		displayName, err := prompts.ask("Model display name", "")
		if err != nil {
			return nil, err
		}
		upstreamName, err := prompts.ask("Upstream model name", name)
		if err != nil {
			return nil, err
		}
		capabilities, err := prompts.askCSV("Capabilities (comma-separated)", strings.Join(defaultCaps, ","))
		if err != nil {
			return nil, err
		}
		models = append(models, providerModelInput{Name: name, DisplayName: displayName, UpstreamName: upstreamName, Capabilities: capabilities})
		more, err := prompts.askYesNo("Add another model", false)
		if err != nil {
			return nil, err
		}
		if !more {
			break
		}
	}
	return models, nil
}

func promptAliasInput(prompts *promptSession, blocks []topLevelBlock, existing *aliasInput, options aliasOptions) (aliasInput, error) {
	defaults := aliasInput{Name: "", Algorithm: "round_robin"}
	if existing != nil {
		defaults = *existing
	}
	if options.Name != "" {
		defaults.Name = options.Name
	}
	if options.Algorithm != "" {
		defaults.Algorithm = options.Algorithm
	}
	if len(options.Targets) > 0 {
		targets, err := buildAliasTargetsFromOptions(options.Targets)
		if err != nil {
			return aliasInput{}, err
		}
		defaults.Targets = targets
	}
	if options.NonInteractive {
		if defaults.Name == "" || defaults.Algorithm == "" || len(defaults.Targets) == 0 {
			return aliasInput{}, fmt.Errorf("alias requires name, algorithm, and at least one target in non-interactive mode")
		}
		return defaults, nil
	}
	if available := availableProviderModels(blocks); len(available) > 0 {
		_, _ = fmt.Fprintln(prompts.out, "Available provider/model targets:")
		for _, item := range available {
			_, _ = fmt.Fprintf(prompts.out, "- %s\n", item)
		}
	}
	name, err := prompts.askRequired("Alias name", defaults.Name)
	if err != nil {
		return aliasInput{}, err
	}
	algorithm, err := prompts.askChoice("Alias algorithm", []string{"round_robin", "least_connections"}, defaults.Algorithm)
	if err != nil {
		return aliasInput{}, err
	}
	input := aliasInput{Name: name, Algorithm: algorithm}
	if len(defaults.Targets) > 0 {
		keepTargets, err := prompts.askYesNo("Keep existing targets", true)
		if err != nil {
			return aliasInput{}, err
		}
		if keepTargets {
			input.Targets = append(input.Targets, defaults.Targets...)
			return input, nil
		}
	}
	for {
		providerName, err := prompts.askRequired("Target provider", "")
		if err != nil {
			return aliasInput{}, err
		}
		modelName, err := prompts.askRequired("Target model", "")
		if err != nil {
			return aliasInput{}, err
		}
		input.Targets = append(input.Targets, aliasTargetInput{Provider: providerName, Model: modelName})
		more, err := prompts.askYesNo("Add another target", false)
		if err != nil {
			return aliasInput{}, err
		}
		if !more {
			break
		}
	}
	return input, nil
}

func promptProviderHealthInput(prompts *promptSession, existing *providerHealthInput, options providerHealthOptions) (providerHealthInput, error) {
	defaults := providerHealthInput{Cooldown: "30s"}
	if existing != nil {
		defaults = *existing
	}
	if options.RedisURL != "" {
		defaults.RedisURL = options.RedisURL
	}
	if options.KeyPrefix != "" {
		defaults.KeyPrefix = options.KeyPrefix
	}
	if options.Cooldown != "" {
		defaults.Cooldown = options.Cooldown
	}
	if options.NonInteractive {
		return defaults, nil
	}
	redisURL, err := prompts.ask("Redis URL", defaults.RedisURL)
	if err != nil {
		return providerHealthInput{}, err
	}
	keyPrefix := ""
	if redisURL != "" {
		keyPrefixDefault := defaults.KeyPrefix
		if keyPrefixDefault == "" {
			keyPrefixDefault = "aiproxy:provider-health"
		}
		keyPrefix, err = prompts.ask("Redis key prefix", keyPrefixDefault)
		if err != nil {
			return providerHealthInput{}, err
		}
	}
	cooldownDefault := defaults.Cooldown
	if cooldownDefault == "" {
		cooldownDefault = "30s"
	}
	cooldown, err := prompts.ask("Cooldown", cooldownDefault)
	if err != nil {
		return providerHealthInput{}, err
	}
	return providerHealthInput{RedisURL: redisURL, KeyPrefix: keyPrefix, Cooldown: cooldown}, nil
}

func (p *promptSession) ask(label, def string) (string, error) {
	prompt := label
	if def != "" {
		prompt += " [" + def + "]"
	}
	_, _ = fmt.Fprintf(p.out, "%s: ", prompt)
	line, err := p.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

func (p *promptSession) askRequired(label, def string) (string, error) {
	for {
		value, err := p.ask(label, def)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) != "" {
			return value, nil
		}
		_, _ = fmt.Fprintln(p.out, "Value is required.")
	}
}

func (p *promptSession) askYesNo(label string, def bool) (bool, error) {
	defaultValue := "y/N"
	if def {
		defaultValue = "Y/n"
	}
	for {
		value, err := p.ask(label+" (y/n)", defaultValue)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		case "y/n", "Y/n":
			return def, nil
		case "", "y/N":
			return def, nil
		}
		_, _ = fmt.Fprintln(p.out, "Enter y or n.")
	}
}

func (p *promptSession) askChoice(label string, options []string, def string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("%s: no options available", label)
	}
	for i, option := range options {
		_, _ = fmt.Fprintf(p.out, "%d. %s\n", i+1, option)
	}
	for {
		value, err := p.ask(label, def)
		if err != nil {
			return "", err
		}
		for _, option := range options {
			if value == option {
				return option, nil
			}
		}
		idx, err := strconv.Atoi(value)
		if err == nil && idx >= 1 && idx <= len(options) {
			return options[idx-1], nil
		}
		_, _ = fmt.Fprintln(p.out, "Choose one of the listed values or numbers.")
	}
}

func (p *promptSession) askCSV(label, def string) ([]string, error) {
	value, err := p.ask(label, def)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out, nil
}

func defaultProviderEnvExpression(providerType string) string {
	switch providerType {
	case "anthropic":
		return `env("ANTHROPIC_API_KEY")`
	case "gemini":
		return `env("GEMINI_API_KEY")`
	default:
		return `env("OPENAI_API_KEY")`
	}
}

func defaultCapabilities(providerType string) []string {
	switch providerType {
	case "anthropic":
		return []string{"chat", "responses"}
	case "gemini":
		return []string{"chat", "responses", "embeddings"}
	default:
		return []string{"chat", "responses"}
	}
}

func existingListenerInput(blocks []topLevelBlock) *listenerInput {
	block := findBlock(blocks, func(block topLevelBlock) bool { return block.Type == "listener" })
	if block == nil {
		return nil
	}
	parsed, src, err := parseBlockSyntax(block.Text)
	if err != nil {
		return nil
	}
	input := &listenerInput{}
	if len(parsed.Labels) >= 2 {
		input.Name = parsed.Labels[1]
	}
	input.Address = parseLiteralOrExpression(attributeExpr(src, parsed.Body, "address"))
	if timeouts := findNestedBlock(parsed.Body, "timeouts"); timeouts != nil {
		input.ReadHeader = parseLiteralOrExpression(attributeExpr(src, timeouts.Body, "read_header"))
		input.Idle = parseLiteralOrExpression(attributeExpr(src, timeouts.Body, "idle"))
		input.Write = parseLiteralOrExpression(attributeExpr(src, timeouts.Body, "write"))
	}
	return input
}

func existingAuthInput(blocks []topLevelBlock) *authInput {
	block := findBlock(blocks, func(block topLevelBlock) bool { return block.Type == "auth" })
	if block == nil {
		return nil
	}
	parsed, src, err := parseBlockSyntax(block.Text)
	if err != nil {
		return nil
	}
	input := &authInput{}
	if len(parsed.Labels) >= 1 {
		input.Name = parsed.Labels[0]
	}
	input.Mode = parseLiteralOrExpression(attributeExpr(src, parsed.Body, "mode"))
	if rateLimit := findNestedBlock(parsed.Body, "rate_limit"); rateLimit != nil {
		input.RateLimit = &authRateLimitInput{
			RequestsPerMinute: strings.TrimSpace(attributeExpr(src, rateLimit.Body, "requests_per_minute")),
			Burst:             strings.TrimSpace(attributeExpr(src, rateLimit.Body, "burst")),
		}
	}
	for _, clientBlock := range findNestedBlocks(parsed.Body, "client") {
		client := authClientInput{}
		if len(clientBlock.Labels) >= 1 {
			client.Name = clientBlock.Labels[0]
		}
		client.Token = parseLiteralOrExpression(attributeExpr(src, clientBlock.Body, "token"))
		client.Tenant = parseLiteralOrExpression(attributeExpr(src, clientBlock.Body, "tenant"))
		client.AllowedModels = parseQuotedListExpr(attributeExpr(src, clientBlock.Body, "allowed_models"))
		input.Clients = append(input.Clients, client)
	}
	return input
}

func existingProviderInput(blocks []topLevelBlock, name string) *providerInput {
	if name == "" {
		return nil
	}
	block := findBlock(blocks, func(block topLevelBlock) bool {
		return block.Type == "provider" && len(block.Labels) >= 2 && block.Labels[1] == name
	})
	if block == nil {
		return nil
	}
	parsed, src, err := parseBlockSyntax(block.Text)
	if err != nil {
		return nil
	}
	input := &providerInput{}
	if len(parsed.Labels) >= 2 {
		input.ProviderType = parsed.Labels[0]
		input.Name = parsed.Labels[1]
	}
	input.DisplayName = parseLiteralOrExpression(attributeExpr(src, parsed.Body, "display_name"))
	input.BaseURL = parseLiteralOrExpression(attributeExpr(src, parsed.Body, "base_url"))
	if apiKeyRef := findNestedBlock(parsed.Body, "api_key_ref"); apiKeyRef != nil {
		input.Credential.Mode = "secrets_file"
		input.Credential.SecretsPath = parseLiteralOrExpression(attributeExpr(src, apiKeyRef.Body, "path"))
		if input.Credential.SecretsPath == "" {
			input.Credential.SecretsPath = defaultSecretsPath()
		}
		input.Credential.SecretsKey = parseLiteralOrExpression(attributeExpr(src, apiKeyRef.Body, "key"))
	} else {
		apiKeyExpr := parseLiteralOrExpression(attributeExpr(src, parsed.Body, "api_key"))
		if envExprPattern.MatchString(apiKeyExpr) {
			input.Credential.Mode = "env_expression"
		} else if apiKeyExpr != "" {
			input.Credential.Mode = "inline"
		}
		input.Credential.APIKeyValue = apiKeyExpr // pragma: allowlist secret
	}
	for _, modelBlock := range findNestedBlocks(parsed.Body, "model") {
		model := providerModelInput{}
		if len(modelBlock.Labels) >= 1 {
			model.Name = modelBlock.Labels[0]
		}
		model.DisplayName = parseLiteralOrExpression(attributeExpr(src, modelBlock.Body, "display_name"))
		model.UpstreamName = parseLiteralOrExpression(attributeExpr(src, modelBlock.Body, "upstream_name"))
		model.Capabilities = parseQuotedListExpr(attributeExpr(src, modelBlock.Body, "capabilities"))
		input.Models = append(input.Models, model)
	}
	return input
}

func existingAliasInput(blocks []topLevelBlock, name string) *aliasInput {
	if name == "" {
		return nil
	}
	block := findBlock(blocks, func(block topLevelBlock) bool {
		return block.Type == "alias" && len(block.Labels) >= 1 && block.Labels[0] == name
	})
	if block == nil {
		return nil
	}
	parsed, src, err := parseBlockSyntax(block.Text)
	if err != nil {
		return nil
	}
	input := &aliasInput{}
	if len(parsed.Labels) >= 1 {
		input.Name = parsed.Labels[0]
	}
	input.Algorithm = parseLiteralOrExpression(attributeExpr(src, parsed.Body, "algorithm"))
	for _, targetBlock := range findNestedBlocks(parsed.Body, "target") {
		input.Targets = append(input.Targets, aliasTargetInput{
			Provider: parseLiteralOrExpression(attributeExpr(src, targetBlock.Body, "provider")),
			Model:    parseLiteralOrExpression(attributeExpr(src, targetBlock.Body, "model")),
		})
	}
	return input
}

func existingProviderHealthInput(blocks []topLevelBlock) *providerHealthInput {
	block := findBlock(blocks, func(block topLevelBlock) bool { return block.Type == "provider_health" })
	if block == nil {
		return nil
	}
	parsed, src, err := parseBlockSyntax(block.Text)
	if err != nil {
		return nil
	}
	return &providerHealthInput{
		RedisURL:  parseLiteralOrExpression(attributeExpr(src, parsed.Body, "redis_url")),
		KeyPrefix: parseLiteralOrExpression(attributeExpr(src, parsed.Body, "key_prefix")),
		Cooldown:  parseLiteralOrExpression(attributeExpr(src, parsed.Body, "cooldown")),
	}
}

func parseBlockSyntax(blockText string) (*hclsyntax.Block, []byte, error) {
	src := []byte(blockText)
	file, diags := hclsyntax.ParseConfig(src, "configure.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, nil, errors.New(diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok || len(body.Blocks) != 1 {
		return nil, nil, fmt.Errorf("expected exactly one block")
	}
	return body.Blocks[0], src, nil
}

func findNestedBlock(body *hclsyntax.Body, blockType string) *hclsyntax.Block {
	for _, block := range body.Blocks {
		if block.Type == blockType {
			return block
		}
	}
	return nil
}

func findNestedBlocks(body *hclsyntax.Body, blockType string) []*hclsyntax.Block {
	var out []*hclsyntax.Block
	for _, block := range body.Blocks {
		if block.Type == blockType {
			out = append(out, block)
		}
	}
	return out
}

func attributeExpr(src []byte, body *hclsyntax.Body, name string) string {
	if body == nil {
		return ""
	}
	attr, ok := body.Attributes[name]
	if !ok {
		return ""
	}
	rng := attr.Expr.Range()
	if rng.Start.Byte < 0 || rng.End.Byte > len(src) || rng.End.Byte < rng.Start.Byte {
		return ""
	}
	return strings.TrimSpace(string(src[rng.Start.Byte:rng.End.Byte]))
}

func parseLiteralOrExpression(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(expr); err == nil {
		return unquoted
	}
	return expr
}

func parseQuotedListExpr(expr string) []string {
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	var out []string
	for _, raw := range regexp.MustCompile(`"(?:\\.|[^"\\])*"`).FindAllString(expr, -1) {
		value, err := strconv.Unquote(raw)
		if err == nil {
			out = append(out, value)
		}
	}
	return out
}

func renderListenerBlock(input listenerInput) string {
	var b strings.Builder
	b.WriteString("listener \"http\" ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	b.WriteString("  address = ")
	b.WriteString(strconv.Quote(input.Address))
	b.WriteString("\n")
	if input.ReadHeader != "" || input.Idle != "" || input.Write != "" {
		b.WriteString("\n  timeouts {\n")
		if input.ReadHeader != "" {
			b.WriteString("    read_header = ")
			b.WriteString(strconv.Quote(input.ReadHeader))
			b.WriteString("\n")
		}
		if input.Idle != "" {
			b.WriteString("    idle = ")
			b.WriteString(strconv.Quote(input.Idle))
			b.WriteString("\n")
		}
		if input.Write != "" {
			b.WriteString("    write = ")
			b.WriteString(strconv.Quote(input.Write))
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderAuthBlock(input authInput) string {
	var b strings.Builder
	b.WriteString("auth ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	b.WriteString("  mode = ")
	b.WriteString(strconv.Quote(input.Mode))
	b.WriteString("\n")
	if input.RateLimit != nil {
		b.WriteString("\n  rate_limit {\n")
		b.WriteString("    requests_per_minute = ")
		b.WriteString(input.RateLimit.RequestsPerMinute)
		b.WriteString("\n")
		if input.RateLimit.Burst != "" {
			b.WriteString("    burst               = ")
			b.WriteString(input.RateLimit.Burst)
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	for _, client := range input.Clients {
		b.WriteString("\n  client ")
		b.WriteString(strconv.Quote(client.Name))
		b.WriteString(" {\n")
		b.WriteString("    token = ")
		b.WriteString(renderStringOrExpression(client.Token))
		b.WriteString("\n")
		if client.Tenant != "" {
			b.WriteString("    tenant = ")
			b.WriteString(strconv.Quote(client.Tenant))
			b.WriteString("\n")
		}
		if len(client.AllowedModels) > 0 {
			b.WriteString("    allowed_models = ")
			b.WriteString(renderQuotedList(client.AllowedModels))
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderProviderBlock(input providerInput) string {
	var b strings.Builder
	b.WriteString("provider ")
	b.WriteString(strconv.Quote(input.ProviderType))
	b.WriteString(" ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	if input.DisplayName != "" {
		b.WriteString("  display_name = ")
		b.WriteString(strconv.Quote(input.DisplayName))
		b.WriteString("\n")
	}
	if input.BaseURL != "" {
		b.WriteString("  base_url = ")
		b.WriteString(strconv.Quote(input.BaseURL))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	switch input.Credential.Mode {
	case "secrets_file":
		b.WriteString("  api_key_ref {\n")
		if input.Credential.SecretsPath != defaultSecretsPath() {
			b.WriteString("    path = ")
			b.WriteString(strconv.Quote(input.Credential.SecretsPath))
			b.WriteString("\n")
		}
		b.WriteString("    key  = ")
		b.WriteString(strconv.Quote(input.Credential.SecretsKey))
		b.WriteString("\n")
		b.WriteString("  }\n")
	default:
		b.WriteString("  api_key = ")
		b.WriteString(renderStringOrExpression(input.Credential.APIKeyValue))
		b.WriteString("\n")
	}
	for _, model := range input.Models {
		b.WriteString("\n  model ")
		b.WriteString(strconv.Quote(model.Name))
		b.WriteString(" {\n")
		if model.DisplayName != "" {
			b.WriteString("    display_name = ")
			b.WriteString(strconv.Quote(model.DisplayName))
			b.WriteString("\n")
		}
		if model.UpstreamName != "" && model.UpstreamName != model.Name {
			b.WriteString("    upstream_name = ")
			b.WriteString(strconv.Quote(model.UpstreamName))
			b.WriteString("\n")
		}
		if len(model.Capabilities) > 0 {
			b.WriteString("    capabilities = ")
			b.WriteString(renderQuotedList(model.Capabilities))
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderAliasBlock(input aliasInput) string {
	var b strings.Builder
	b.WriteString("alias ")
	b.WriteString(strconv.Quote(input.Name))
	b.WriteString(" {\n")
	b.WriteString("  algorithm = ")
	b.WriteString(strconv.Quote(input.Algorithm))
	b.WriteString("\n")
	for _, target := range input.Targets {
		b.WriteString("\n  target {\n")
		b.WriteString("    provider = ")
		b.WriteString(strconv.Quote(target.Provider))
		b.WriteString("\n")
		b.WriteString("    model    = ")
		b.WriteString(strconv.Quote(target.Model))
		b.WriteString("\n")
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderProviderHealthBlock(input providerHealthInput) string {
	var b strings.Builder
	b.WriteString("provider_health {\n")
	if input.RedisURL != "" {
		b.WriteString("  redis_url = ")
		b.WriteString(strconv.Quote(input.RedisURL))
		b.WriteString("\n")
	}
	if input.KeyPrefix != "" {
		b.WriteString("  key_prefix = ")
		b.WriteString(strconv.Quote(input.KeyPrefix))
		b.WriteString("\n")
	}
	if input.Cooldown != "" {
		b.WriteString("  cooldown = ")
		b.WriteString(strconv.Quote(input.Cooldown))
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderStringOrExpression(value string) string {
	if envExprPattern.MatchString(value) {
		return value
	}
	return strconv.Quote(value)
}

func renderQuotedList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.Quote(value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func writeConfigFile(path, source string) error {
	if err := validateGeneratedConfig([]byte(source), path); err != nil {
		return err
	}
	if err := writeFile(path, []byte(source), 0o600); err != nil {
		return err
	}
	return nil
}

func writeFile(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func validateGeneratedConfig(source []byte, filename string) error {
	_, diags := hclsyntax.ParseConfig(source, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return fmt.Errorf("parse config %s: %s", filename, diags.Error())
	}
	return nil
}

func upsertBlock(source, blockText string, match func(topLevelBlock) bool) (string, error) {
	if strings.TrimSpace(source) == "" {
		return strings.TrimRight(blockText, "\n") + "\n", nil
	}
	blocks, err := parseTopLevelBlocks(source)
	if err != nil {
		return "", err
	}
	for _, block := range blocks {
		if match(block) {
			return strings.TrimRight(source[:block.Start], "\n") + "\n\n" + strings.TrimRight(blockText, "\n") + "\n" + strings.TrimLeft(source[block.End:], "\n"), nil
		}
	}
	trimmed := strings.TrimRight(source, "\n")
	if trimmed == "" {
		return strings.TrimRight(blockText, "\n") + "\n", nil
	}
	return trimmed + "\n\n" + strings.TrimRight(blockText, "\n") + "\n", nil
}

func removeBlock(source string, match func(topLevelBlock) bool) (string, bool, error) {
	if strings.TrimSpace(source) == "" {
		return source, false, nil
	}
	blocks, err := parseTopLevelBlocks(source)
	if err != nil {
		return "", false, err
	}
	for _, block := range blocks {
		if match(block) {
			prefix := strings.TrimRight(source[:block.Start], "\n")
			suffix := strings.TrimLeft(source[block.End:], "\n")
			switch {
			case prefix == "" && suffix == "":
				return "", true, nil
			case prefix == "":
				return suffix, true, nil
			case suffix == "":
				return prefix + "\n", true, nil
			default:
				return prefix + "\n\n" + suffix, true, nil
			}
		}
	}
	return source, false, nil
}

func parseTopLevelBlocks(source string) ([]topLevelBlock, error) {
	var blocks []topLevelBlock
	for i := 0; i < len(source); {
		next, err := skipSpaceAndComments(source, i)
		if err != nil {
			return nil, err
		}
		i = next
		if i >= len(source) {
			break
		}
		if !isIdentStart(rune(source[i])) {
			return nil, fmt.Errorf("unexpected character %q at offset %d", source[i], i)
		}
		start := i
		typ, next := readIdentifier(source, i)
		i = next
		var labels []string
		for {
			next, err = skipInlineSpace(source, i)
			if err != nil {
				return nil, err
			}
			i = next
			if i >= len(source) {
				return nil, fmt.Errorf("unexpected end of input after block header %q", typ)
			}
			if source[i] == '{' {
				break
			}
			if source[i] != '"' {
				return nil, fmt.Errorf("invalid block header for %q at offset %d", typ, i)
			}
			label, end, err := readQuotedString(source, i)
			if err != nil {
				return nil, err
			}
			labels = append(labels, label)
			i = end
		}
		end, err := scanBlockEnd(source, i)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, topLevelBlock{Type: typ, Labels: labels, Start: start, End: end, Text: source[start:end]})
		i = end
	}
	return blocks, nil
}

func skipSpaceAndComments(source string, i int) (int, error) {
	for i < len(source) {
		switch source[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '#':
			for i < len(source) && source[i] != '\n' {
				i++
			}
		case '/':
			if i+1 >= len(source) {
				return i, nil
			}
			switch source[i+1] {
			case '/':
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
			case '*':
				end := strings.Index(source[i+2:], "*/")
				if end < 0 {
					return 0, fmt.Errorf("unterminated block comment at offset %d", i)
				}
				i += end + 4
			default:
				return i, nil
			}
		default:
			return i, nil
		}
	}
	return i, nil
}

func skipInlineSpace(source string, i int) (int, error) {
	for i < len(source) {
		switch source[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '#':
			for i < len(source) && source[i] != '\n' {
				i++
			}
		case '/':
			if i+1 >= len(source) {
				return i, nil
			}
			switch source[i+1] {
			case '/':
				i += 2
				for i < len(source) && source[i] != '\n' {
					i++
				}
			case '*':
				end := strings.Index(source[i+2:], "*/")
				if end < 0 {
					return 0, fmt.Errorf("unterminated block comment at offset %d", i)
				}
				i += end + 4
			default:
				return i, nil
			}
		default:
			return i, nil
		}
	}
	return i, nil
}

func readIdentifier(source string, start int) (string, int) {
	i := start
	for i < len(source) && isIdentPart(rune(source[i])) {
		i++
	}
	return source[start:i], i
}

func readQuotedString(source string, start int) (string, int, error) {
	i := start + 1
	escaped := false
	for i < len(source) {
		if escaped {
			escaped = false
			i++
			continue
		}
		switch source[i] {
		case '\\':
			escaped = true
		case '"':
			value, err := strconv.Unquote(source[start : i+1])
			if err != nil {
				return "", 0, err
			}
			return value, i + 1, nil
		}
		i++
	}
	return "", 0, fmt.Errorf("unterminated string at offset %d", start)
}

func scanBlockEnd(source string, openBrace int) (int, error) {
	depth := 0
	inString := false
	escaped := false
	inLineComment := false
	inBlockComment := false
	for i := openBrace; i < len(source); i++ {
		ch := source[i]
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(source) && source[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == '#' {
			inLineComment = true
			continue
		}
		if ch == '/' && i+1 < len(source) {
			switch source[i+1] {
			case '/':
				inLineComment = true
				i++
				continue
			case '*':
				inBlockComment = true
				i++
				continue
			}
		}
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("unterminated block starting at offset %d", openBrace)
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

func providerBlockNames(blocks []topLevelBlock) []string {
	var names []string
	for _, block := range blocks {
		if block.Type == "provider" && len(block.Labels) >= 2 {
			names = append(names, block.Labels[1])
		}
	}
	sort.Strings(names)
	return names
}

func aliasBlockNames(blocks []topLevelBlock) []string {
	var names []string
	for _, block := range blocks {
		if block.Type == "alias" && len(block.Labels) >= 1 {
			names = append(names, block.Labels[0])
		}
	}
	sort.Strings(names)
	return names
}

func availableProviderModels(blocks []topLevelBlock) []string {
	var out []string
	for _, block := range blocks {
		if block.Type != "provider" || len(block.Labels) < 2 {
			continue
		}
		providerName := block.Labels[1]
		for _, modelName := range modelNamesFromProviderBlock(block.Text) {
			out = append(out, providerName+"/"+modelName)
		}
	}
	sort.Strings(out)
	return out
}

func modelNamesFromProviderBlock(block string) []string {
	var names []string
	for _, match := range regexp.MustCompile(`(?m)^\s*model\s+"([^"]+)"\s*\{`).FindAllStringSubmatch(block, -1) {
		names = append(names, match[1])
	}
	return names
}

func targetProviderName(action, existingName, inputName string) string {
	if action == "update" && existingName != "" {
		return existingName
	}
	return inputName
}

func targetAliasName(action, existingName, inputName string) string {
	if action == "update" && existingName != "" {
		return existingName
	}
	return inputName
}
