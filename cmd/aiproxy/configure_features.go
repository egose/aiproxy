package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

const (
	configureDashboardMenu    = "Dashboard & Web UI"
	configureMultiTenancyMenu = "Multi-tenancy"
)

type dashboardUIOptions struct {
	dashboard, webUI *bool
	tokenEnv         *string
	nonInteractive   bool
}

type multiTenancyOptions struct {
	enabled, publicRegistration *bool
	databaseURLEnv              *string
	nonInteractive              bool
}

func newConfigureDashboardUICommand() *cobra.Command {
	var dashboard, webUI, nonInteractive bool
	var tokenEnv string
	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Configure the Dashboard API and embedded Web UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := dashboardUIOptions{nonInteractive: nonInteractive}
			if cmd.Flags().Changed("dashboard") {
				opts.dashboard = &dashboard
			}
			if cmd.Flags().Changed("web-ui") {
				opts.webUI = &webUI
			}
			if cmd.Flags().Changed("token-env") {
				opts.tokenEnv = &tokenEnv
			}
			prompts := newPromptSession(cmd.InOrStdin(), cmd.OutOrStdout())
			return runConfigureDashboardUI(&prompts, inheritedConfigPath(cmd), opts)
		},
	}
	cmd.Flags().BoolVar(&dashboard, "dashboard", false, "enable the Dashboard API (use --dashboard=false to disable)")
	cmd.Flags().BoolVar(&webUI, "web-ui", false, "serve the embedded Web UI (use --web-ui=false to disable)")
	cmd.Flags().StringVar(&tokenEnv, "token-env", "", "dashboard token environment variable (empty selects a server-generated token)")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "apply only the settings explicitly supplied, without prompting")
	return cmd
}

func newConfigureMultiTenancyCommand() *cobra.Command {
	var enabled, publicRegistration, nonInteractive bool
	var databaseURLEnv string
	cmd := &cobra.Command{
		Use:   "multi-tenancy",
		Short: "Configure multi-tenancy, PostgreSQL, and public registration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts := multiTenancyOptions{nonInteractive: nonInteractive}
			if cmd.Flags().Changed("enabled") {
				opts.enabled = &enabled
			}
			if cmd.Flags().Changed("allow-public-registration") {
				opts.publicRegistration = &publicRegistration
			}
			if cmd.Flags().Changed("database-url-env") {
				opts.databaseURLEnv = &databaseURLEnv
			}
			prompts := newPromptSession(cmd.InOrStdin(), cmd.OutOrStdout())
			return runConfigureMultiTenancy(&prompts, inheritedConfigPath(cmd), opts)
		},
	}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "enable multi-tenancy (use --enabled=false to disable)")
	cmd.Flags().BoolVar(&publicRegistration, "allow-public-registration", false, "allow public registration")
	cmd.Flags().StringVar(&databaseURLEnv, "database-url-env", "", "environment variable supplying the PostgreSQL URL")
	cmd.Flags().BoolVar(&nonInteractive, "non-interactive", false, "apply the supplied settings without prompting")
	return cmd
}

func runConfigureDashboardUI(prompts *promptSession, path string, opts dashboardUIOptions) error {
	if opts.nonInteractive && opts.dashboard == nil && opts.webUI == nil && opts.tokenEnv == nil {
		return fmt.Errorf("specify --dashboard, --web-ui, or --token-env in non-interactive mode")
	}
	path, doc, err := loadFeatureConfig(prompts, path, opts.nonInteractive)
	if err != nil {
		return err
	}
	before := doc.source
	hadDashboard := doc.blocks["dashboard"] != nil
	dashboard, err := configureFeatureSwitch(prompts, "Enable Dashboard API (for the TUI and live browser dashboard)", hadDashboard, opts.dashboard, opts.nonInteractive)
	if err != nil {
		return err
	}
	hadWebUI, err := doc.boolValue("web_ui", "enabled")
	if err != nil {
		return err
	}
	webUI, err := configureFeatureSwitch(prompts, "Serve embedded Web UI", hadWebUI, opts.webUI, opts.nonInteractive)
	if err != nil {
		return err
	}
	if !dashboard && opts.tokenEnv != nil {
		return fmt.Errorf("--token-env requires the Dashboard API to be enabled")
	}
	tokenEnv := opts.tokenEnv
	if dashboard && tokenEnv == nil && !opts.nonInteractive {
		tokenEnv, err = promptDashboardToken(prompts, doc.attribute("dashboard", "token") != nil)
		if err != nil {
			return err
		}
	}
	details := []string{"Dashboard API: " + featureState(dashboard), "Embedded Web UI: " + featureState(webUI)}
	if dashboard {
		if err := doc.ensureBlock("dashboard"); err != nil {
			return err
		}
		if tokenEnv != nil {
			expr := ""
			if *tokenEnv != "" {
				if err := validateConfigureEnvName(*tokenEnv); err != nil {
					return err
				}
				expr = fmt.Sprintf("env(%q)", *tokenEnv)
			}
			if err := doc.setAttribute("dashboard", "token", expr); err != nil {
				return err
			}
		}
		switch {
		case doc.attribute("dashboard", "token") == nil:
			details = append(details, "Dashboard token: server-generated")
		case tokenEnv == nil:
			details = append(details, "Dashboard token: existing setting kept")
		default:
			details = append(details, "Dashboard token: environment variable "+*tokenEnv)
		}
	} else if hadDashboard {
		if err := doc.replaceRange(doc.blocks["dashboard"].Range(), ""); err != nil {
			return err
		}
		details = append(details, "The dashboard block and any explicit token setting will be removed.")
	}
	if webUI != hadWebUI {
		if err := doc.setAttribute("web_ui", "enabled", strconv.FormatBool(webUI)); err != nil {
			return err
		}
	}
	var warning string
	if webUI && !dashboard {
		multi, err := doc.boolValue("multi_tenancy", "enabled")
		switch {
		case err != nil:
			warning = "Live dashboard views are disabled. Browser workspace management requires multi-tenancy."
		case multi:
			details = append(details, "Browser workspace management is available; live dashboard views are disabled.")
		default:
			warning = "Only the Web UI shell will be served. Live views need the Dashboard API; workspace management needs multi-tenancy."
		}
	}
	return saveFeatureConfig(prompts, path, before, doc.source, opts.nonInteractive, dashboard && !hadDashboard, details, warning)
}

func promptDashboardToken(prompts *promptSession, hasSetting bool) (*string, error) {
	const keep, auto, env = "keep existing setting", "server-generated", "environment variable"
	choices := []string{auto, env}
	if hasSetting {
		choices = append([]string{keep}, choices...)
	}
	choice, err := prompts.askChoice("Dashboard token", choices, choices[0])
	if err != nil || choice == keep {
		return nil, err
	}
	if choice == env {
		name, err := prompts.askValidated("Dashboard token environment variable", "AIPROXY_DASHBOARD_TOKEN", validateConfigureEnvName)
		return &name, err
	}
	empty := ""
	return &empty, nil
}

func runConfigureMultiTenancy(prompts *promptSession, path string, opts multiTenancyOptions) error {
	if opts.nonInteractive && opts.enabled == nil && opts.publicRegistration == nil && opts.databaseURLEnv == nil {
		return fmt.Errorf("specify --enabled, --allow-public-registration, or --database-url-env in non-interactive mode")
	}
	path, doc, err := loadFeatureConfig(prompts, path, opts.nonInteractive)
	if err != nil {
		return err
	}
	before := doc.source
	wasEnabled, err := doc.boolValue("multi_tenancy", "enabled")
	if err != nil {
		return err
	}
	enabled, err := configureFeatureSwitch(prompts, "Enable multi-tenancy (users, teams, and database-backed editing)", wasEnabled, opts.enabled, opts.nonInteractive)
	if err != nil {
		return err
	}
	details := []string{"Multi-tenancy: " + featureState(enabled)}
	if !enabled {
		if opts.databaseURLEnv != nil || opts.publicRegistration != nil {
			return fmt.Errorf("database URL and public registration options require multi-tenancy to be enabled")
		}
		if wasEnabled {
			if err := doc.setAttribute("multi_tenancy", "enabled", "false"); err != nil {
				return err
			}
			details = append(details, "The database block and stored data are left unchanged.")
		}
		return saveFeatureConfig(prompts, path, before, doc.source, opts.nonInteractive, wasEnabled, details, "")
	}

	registrationConfigured := wasEnabled || doc.attribute("multi_tenancy", "allow_public_registration") != nil
	var oldRegistration bool
	if registrationConfigured {
		oldRegistration, err = doc.boolValue("multi_tenancy", "allow_public_registration")
		if err != nil {
			return err
		}
	}
	registration, err := configureFeatureSwitch(prompts, "Allow public registration", oldRegistration, opts.publicRegistration, opts.nonInteractive)
	if err != nil {
		return err
	}
	dbEnv := opts.databaseURLEnv
	hasDBURL := doc.hasDatabaseURL()
	if dbEnv == nil && !opts.nonInteractive {
		replace := !hasDBURL
		if hasDBURL {
			replace, err = prompts.askYesNo("Replace the configured PostgreSQL URL with an environment variable", false)
			if err != nil {
				return err
			}
		}
		if replace {
			name, err := prompts.askValidated("PostgreSQL URL environment variable", "AIPROXY_DB_URL", validateConfigureEnvName)
			if err != nil {
				return err
			}
			dbEnv = &name
		}
	}
	if dbEnv == nil && !hasDBURL {
		return fmt.Errorf("multi-tenancy requires database.url; supply --database-url-env")
	}
	dbChanged, dbNote := false, ""
	if dbEnv != nil {
		if err := validateConfigureEnvName(*dbEnv); err != nil {
			return err
		}
		if err := doc.setAttribute("database", "url", fmt.Sprintf("env(%q)", *dbEnv)); err != nil {
			return err
		}
		dbChanged = before != doc.source
		details = append(details, "PostgreSQL URL: environment variable "+*dbEnv)
		dbNote = "Set " + *dbEnv + " in the environment of both the migration command and the server."
	} else {
		details = append(details, "PostgreSQL URL: existing setting kept")
	}
	if !wasEnabled {
		if err := doc.setAttribute("multi_tenancy", "enabled", "true"); err != nil {
			return err
		}
	}
	if !registrationConfigured || registration != oldRegistration {
		if err := doc.setAttribute("multi_tenancy", "allow_public_registration", strconv.FormatBool(registration)); err != nil {
			return err
		}
	}
	details = append(details, "Public registration: "+featureState(registration))
	nextSteps := dbNote
	if !wasEnabled || dbChanged {
		nextSteps = buildReviewSummary([]string{
			dbNote,
			"Set AIPROXY_JWT_SECRET in the server environment for account sign-in.",
			"For a fresh database, set AIPROXY_ADMIN_EMAIL and AIPROXY_ADMIN_PASSWORD before running migrations to seed the first system admin.",
			"Run `aiproxy migrate up` with the same --config path before starting the server.",
		}, "")
	}
	return saveFeatureConfig(prompts, path, before, doc.source, opts.nonInteractive, !wasEnabled || dbChanged, details, nextSteps)
}

func configureFeatureSwitch(prompts *promptSession, label string, current bool, requested *bool, nonInteractive bool) (bool, error) {
	if requested != nil {
		return *requested, nil
	}
	if nonInteractive {
		return current, nil
	}
	return prompts.askYesNo(label, current)
}

func featureState(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

var configureEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateConfigureEnvName(name string) error {
	if !configureEnvName.MatchString(name) {
		return fmt.Errorf("enter an environment variable name, not its value")
	}
	return nil
}

func saveFeatureConfig(prompts *promptSession, path, before, after string, nonInteractive, restart bool, details []string, note string) error {
	if before == after {
		_, _ = fmt.Fprintf(prompts.out, "No changes to %s.\n", path)
		if note != "" {
			_, _ = fmt.Fprintln(prompts.out, note)
		}
		return nil
	}
	apply := "If the server is running, reload it with SIGHUP (or restart it) to apply."
	if restart {
		apply = "If the server is running, these changes require a restart."
	}
	lines := append([]string{"Config path: " + path}, details...)
	review := buildReviewSummary(append(lines, note, apply), "")
	if !nonInteractive && prompts.interactiveTUI {
		if err := prompts.confirmWrite("Review configuration changes", review); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(prompts.out, review)
		if !nonInteractive {
			confirmed, err := prompts.askYesNo("Apply these changes", false)
			if err != nil {
				return err
			}
			if !confirmed {
				_, _ = fmt.Fprintln(prompts.out, "No changes saved.")
				return nil
			}
		}
	}
	if err := writeConfigFile(path, after); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(prompts.out, "Updated %s.\n", path)
	if !nonInteractive && prompts.interactiveTUI {
		if note != "" {
			_, _ = fmt.Fprintln(prompts.out, note)
		}
		_, _ = fmt.Fprintln(prompts.out, apply)
	}
	return nil
}

type featureConfig struct {
	source string
	blocks map[string]*hclsyntax.Block
}

func loadFeatureConfig(prompts *promptSession, path string, nonInteractive bool) (string, *featureConfig, error) {
	if path == "" && nonInteractive {
		path = defaultConfigPath()
	}
	path, err := resolveConfigPath(prompts, path)
	if err != nil {
		return "", nil, err
	}
	source, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if strings.HasPrefix(strings.TrimSpace(string(source)), "{") {
		return "", nil, fmt.Errorf("configure supports HCL file editing only; use aiproxy convert to convert JSON to HCL before editing")
	}
	cfg := &featureConfig{}
	if err := cfg.update(string(source)); err != nil {
		return "", nil, fmt.Errorf("config %s: %w", path, err)
	}
	return path, cfg, nil
}

func (c *featureConfig) update(source string) error {
	file, diags := hclsyntax.ParseConfig([]byte(source), "config.hcl", hcl.InitialPos)
	for _, diag := range diags {
		if diag.Severity == hcl.DiagError {
			return fmt.Errorf("cannot parse HCL configuration at %s", diag.Subject)
		}
	}
	blocks := make(map[string]*hclsyntax.Block)
	for _, block := range file.Body.(*hclsyntax.Body).Blocks {
		switch block.Type {
		case "dashboard", "web_ui", "multi_tenancy", "database":
			if len(block.Labels) != 0 || blocks[block.Type] != nil {
				return fmt.Errorf("expected at most one unlabeled %s block", block.Type)
			}
			blocks[block.Type] = block
		}
	}
	c.source, c.blocks = source, blocks
	return nil
}

func (c *featureConfig) attribute(block, name string) *hclsyntax.Attribute {
	if b := c.blocks[block]; b != nil {
		return b.Body.Attributes[name]
	}
	return nil
}

func (c *featureConfig) boolValue(block, name string) (bool, error) {
	attr := c.attribute(block, name)
	if attr == nil {
		return c.blocks[block] != nil, nil
	}
	value, diags := c.expandedValue(attr, os.Getenv)
	if diags.HasErrors() {
		return false, fmt.Errorf("%s.%s must resolve to a boolean", block, name)
	}
	if value.IsNull() {
		return true, nil
	}
	normalized, err := convert.Convert(value, cty.Bool)
	if err != nil {
		return false, fmt.Errorf("%s.%s must resolve to a boolean", block, name)
	}
	return normalized.True(), nil
}

func (c *featureConfig) hasDatabaseURL() bool {
	attr := c.attribute("database", "url")
	if attr == nil {
		return false
	}
	value, diags := c.expandedValue(attr, func(string) string { return "configured" })
	return !diags.HasErrors() && !value.IsNull() && value.Type() == cty.String && strings.TrimSpace(value.AsString()) != ""
}

var featureEnvCall = regexp.MustCompile(`env\("([^"]+)"\)`)

func (c *featureConfig) expandedValue(attr *hclsyntax.Attribute, lookup func(string) string) (cty.Value, hcl.Diagnostics) {
	r := attr.Expr.Range()
	source := c.source[r.Start.Byte:r.End.Byte]
	expanded := featureEnvCall.ReplaceAllStringFunc(source, func(call string) string {
		return strconv.Quote(lookup(call[len(`env("`) : len(call)-len(`")`)]))
	})
	if expanded == source {
		return attr.Expr.Value(nil)
	}
	expr, diags := hclsyntax.ParseExpression([]byte(expanded), "config.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		return cty.NilVal, diags
	}
	return expr.Value(nil)
}

func (c *featureConfig) replaceRange(r hcl.Range, text string) error {
	return c.update(c.source[:r.Start.Byte] + text + c.source[r.End.Byte:])
}

func (c *featureConfig) ensureBlock(block string) error {
	if c.blocks[block] != nil {
		return nil
	}
	source := strings.TrimRight(c.source, "\n")
	if strings.TrimSpace(source) != "" {
		source += "\n\n"
	}
	return c.update(source + block + " {}\n")
}

func (c *featureConfig) setAttribute(blockName, name, expr string) error {
	if err := c.ensureBlock(blockName); err != nil {
		return err
	}
	source := c.source
	block := c.blocks[blockName]
	if attr := c.attribute(blockName, name); attr != nil {
		r := attr.Expr.Range()
		if expr == "" {
			r = attr.Range()
		}
		return c.replaceRange(r, expr)
	}
	if expr == "" {
		return nil
	}
	open, close := block.OpenBraceRange.End.Byte, block.CloseBraceRange.Start.Byte
	if block.OpenBraceRange.End.Line == block.CloseBraceRange.Start.Line {
		body := strings.TrimSpace(source[open:close])
		if body != "" {
			body = "  " + body + "\n"
		}
		return c.update(source[:open] + "\n" + body + "  " + name + " = " + expr + "\n" + source[close:])
	}
	lineStart := strings.LastIndexByte(source[:close], '\n') + 1
	addition := "  " + name + " = " + expr + "\n"
	if strings.TrimSpace(source[lineStart:close]) == "" {
		return c.update(source[:lineStart] + addition + source[lineStart:])
	}
	return c.update(source[:close] + "\n" + addition + source[close:])
}
