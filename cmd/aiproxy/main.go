package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/config"
	"github.com/spf13/cobra"
)

var (
	configPath string
	daemon     bool
	version    = "dev"
)

func main() {
	rootCmd := newRootCommand()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "aiproxy",
		Short: "Proxy multiple AI providers behind a single API",
		Long:  rootLongText(),
	}
	rootCmd.AddCommand(newServeCommand())
	rootCmd.AddCommand(newValidateCommand())
	rootCmd.AddCommand(newPathsCommand())
	rootCmd.AddCommand(newConfigureCommand())
	rootCmd.AddCommand(newModelsCommand())
	rootCmd.AddCommand(newLoginCommand())
	rootCmd.AddCommand(newExamplesCommand())
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newDashboardCommand())
	rootCmd.AddCommand(newStopCommand())
	rootCmd.AddCommand(newStatusCommand())
	rootCmd.AddCommand(newRestartCommand())
	return rootCmd
}

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the AI proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if daemon {
				if configFromEnvRequested(cmd) {
					return errors.New("daemon mode requires a config file; unset AIPROXY_CONFIG or pass --config")
				}
				return spawnDaemon(cmd, configPath)
			}
			opts, err := buildOptionsForServe(configPath, cmd)
			if err != nil {
				return err
			}
			a, err := app.Build(context.Background(), opts)
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
			defer stop()

			return a.RunReady(ctx, notifyDaemonReady)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath(), "path to config file (overrides $AIPROXY_CONFIG)")
	cmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "run the server in the background (Linux only)")
	return cmd
}

func newValidateCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config file without running the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := loadConfigForCommand(cfgPath, cmd); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "config is valid")
			return nil
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file (overrides $AIPROXY_CONFIG)")
	return cmd
}

func defaultConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", "config.hcl")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("aiproxy", "config.hcl")
	}
	return filepath.Join(home, ".config", "aiproxy", "config.hcl")
}

func defaultSecretsPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", "keys.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("aiproxy", "keys.json")
	}
	return filepath.Join(home, ".config", "aiproxy", "keys.json")
}

func rootLongText() string {
	return "aiproxy proxies multiple AI providers behind a single OpenAI-compatible API.\n\nDefault config path: $XDG_CONFIG_HOME/aiproxy/config.hcl\nFallback config path: ~/.config/aiproxy/config.hcl\nDefault secrets path: $XDG_CONFIG_HOME/aiproxy/keys.json\nFallback secrets path: ~/.config/aiproxy/keys.json\n\nSet $AIPROXY_CONFIG to inline HCL to skip the config file (explicit --config overrides it).\n\nDaemon lifecycle commands (`serve -d`, `stop`, `status`, `restart`) are Linux-only.\n\nUse `aiproxy paths` to print resolved paths, `aiproxy examples` for boxed config examples, and `aiproxy configure` to create or update config blocks interactively.\n\n" + currentConfigStatusText()
}

func currentConfigStatusText() string {
	var b strings.Builder
	b.WriteString("Current status:")
	if src, ok := config.EnvConfigContent(); ok {
		fmt.Fprintf(&b, "\n  config: %s (set, %d bytes; --config overrides)", config.EnvConfigFilename, len(src))
	} else if st, err := os.Stat(defaultConfigPath()); err == nil && !st.IsDir() {
		fmt.Fprintf(&b, "\n  config: %s (exists, %d bytes)", defaultConfigPath(), st.Size())
	} else {
		fmt.Fprintf(&b, "\n  config: %s (missing)", defaultConfigPath())
	}
	if st, err := os.Stat(defaultSecretsPath()); err == nil && !st.IsDir() {
		fmt.Fprintf(&b, "\n  secrets: %s (exists, %d bytes, mode %04o)", defaultSecretsPath(), st.Size(), st.Mode().Perm())
	} else {
		fmt.Fprintf(&b, "\n  secrets: %s (missing)", defaultSecretsPath())
	}
	return b.String()
}

func newPathsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Print resolved default config and secrets paths",
		Run: func(cmd *cobra.Command, args []string) {
			configDisplay := defaultConfigPath()
			if _, ok := config.EnvConfigContent(); ok {
				configDisplay = config.EnvConfigFilename + " (AIPROXY_CONFIG is set; --config overrides)"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\nsecrets: %s\n", configDisplay, defaultSecretsPath())
		},
	}
}

func newExamplesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "examples",
		Short: "Print common command and config examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), allExamplesText())
		},
	}
	cmd.AddCommand(newAllExamplesCommand())
	cmd.AddCommand(newConfigExamplesCommand())
	cmd.AddCommand(newAuthExamplesCommand())
	cmd.AddCommand(newAliasExamplesCommand())
	cmd.AddCommand(newDockerExamplesCommand())
	cmd.AddCommand(newSystemdExamplesCommand())
	return cmd
}

func newAllExamplesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "all",
		Short: "Print all command and deployment examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), allExamplesText())
		},
	}
}

func newConfigExamplesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print command and configuration examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), configExamplesText())
		},
	}
}

func newAuthExamplesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Print authentication configuration examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), authExamplesText())
		},
	}
}

func newAliasExamplesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "alias",
		Short: "Print alias routing and failover examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), aliasExamplesText())
		},
	}
}

func newDockerExamplesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "docker",
		Short: "Print Docker deployment examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), dockerExamplesText())
		},
	}
}

func newSystemdExamplesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "systemd",
		Short: "Print systemd service examples",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), systemdExamplesText())
		},
	}
}

func allExamplesText() string {
	return strings.Join([]string{configExamplesText(), dockerExamplesText(), systemdExamplesText()}, "\n\n")
}

func configExamplesText() string {
	return strings.Join([]string{commandExamplesText(), minimalConfigExamplesText(), aliasExamplesText(), secretsExamplesText(), apiKeyRefExamplesText()}, "\n\n")
}

func commandExamplesText() string {
	return renderExampleBox("Common commands", `aiproxy serve
aiproxy serve -d  # Linux only
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy stop      # Linux only
aiproxy status    # Linux only
aiproxy restart   # Linux only
aiproxy dashboard --config /etc/aiproxy/config.hcl
aiproxy validate
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy paths
aiproxy configure
aiproxy configure provider
aiproxy configure provider --config /etc/aiproxy/config.hcl --non-interactive --name backup --type openai-compatible --base-url https://llm.internal/v1 --secrets-key localai --api-key "$LOCALAI_API_KEY" --model qwen3-32b
aiproxy models
aiproxy models --provider openai
aiproxy models --provider openai --upstream
aiproxy version`)
}

func minimalConfigExamplesText() string {
	return renderExampleBox("Minimal config", `listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

logging {
  level      = "info"
  access_log = true
}

provider "openai" "openai" {
  api_key = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {}
}`)
}

func authExamplesText() string {
	return renderExampleBox("Auth example", `auth "main" {
  mode = "bearer_static"

  rate_limit {
    requests_per_minute = 120
    burst               = 120
  }

  client "internal-app" {
    token          = env("AIPROXY_CLIENT_TOKEN")
    tenant         = "internal"
    allowed_models = ["alias/chat_default", "openai/gpt-4o-mini"]
  }
}`)
}

func aliasExamplesText() string {
	return strings.Join([]string{
		authExamplesText(),
		renderExampleBox("Alias failover example", `listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "bearer_static"

  client "internal-app" {
    token = env("AIPROXY_CLIENT_TOKEN")
  }
}

provider "openai" "primary" {
  api_key = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {}
}

provider "openai-compatible" "backup" {
  base_url = "https://llm.internal/v1"

  api_key_ref {
    key = "localai"
  }

  model "qwen3-32b" {}
}

alias "chat_default" {
  algorithm = "round_robin"

  target {
    provider = "primary"
    model    = "gpt-4o-mini"
  }

  target {
    provider = "backup"
    model    = "qwen3-32b"
  }
}`),
	}, "\n\n")
}

func secretsExamplesText() string {
	return renderExampleBox("Secrets file example", `{
  "openai": "sk-...",
  "localai": "secret"
}`)
}

func apiKeyRefExamplesText() string {
	return renderExampleBox("api_key_ref override example", `provider "openai-compatible" "localai" {
  base_url = "https://llm.internal/v1"

  api_key_ref {
    path = "/etc/aiproxy/keys.json"
    key  = "localai"
  }

  model "qwen3-32b" {}
}`)
}

func dockerExamplesText() string {
	return renderExampleBox("Docker example", `docker run --rm \
  -p 8080:8080 \
  -v /etc/aiproxy/config.hcl:/etc/aiproxy/config.hcl:ro \
  -v /etc/aiproxy/keys.json:/etc/aiproxy/keys.json:ro \
  aiproxy:latest serve --config /etc/aiproxy/config.hcl`)
}

func systemdExamplesText() string {
	return renderExampleBox("systemd example", `[Unit]
Description=aiproxy
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/aiproxy serve --config /etc/aiproxy/config.hcl
Restart=on-failure
Environment=OPENAI_API_KEY=

[Install]
WantedBy=multi-user.target
`)
}

func renderExampleBox(title, body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	width := len(title)
	for _, line := range lines {
		if len(line) > width {
			width = len(line)
		}
	}

	var b strings.Builder
	border := "+-" + strings.Repeat("-", width) + "-+\n"
	b.WriteString(border)
	b.WriteString("| " + padRight(title, width) + " |\n")
	b.WriteString(border)
	for _, line := range lines {
		b.WriteString("| " + padRight(line, width) + " |\n")
	}
	b.WriteString("+-" + strings.Repeat("-", width) + "-+")
	return b.String()
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
}
