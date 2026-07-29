package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/egose/aiproxy/internal/app"
	"github.com/spf13/cobra"
)

var (
	configPath string
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
		Long:  "aiproxy proxies multiple AI providers behind a single OpenAI-compatible API.\n\nDefault config path: $XDG_CONFIG_HOME/aiproxy/config.hcl\nFallback config path: ~/.config/aiproxy/config.hcl\nDefault secrets path: $XDG_CONFIG_HOME/aiproxy/keys.json\nFallback secrets path: ~/.config/aiproxy/keys.json\n\nUse `aiproxy paths` to print resolved paths, `aiproxy examples` for boxed config examples, and `aiproxy configure` to create or update config blocks interactively.",
	}
	rootCmd.AddCommand(newServeCommand())
	rootCmd.AddCommand(newValidateCommand())
	rootCmd.AddCommand(newPathsCommand())
	rootCmd.AddCommand(newConfigureCommand())
	rootCmd.AddCommand(newExamplesCommand())
	rootCmd.AddCommand(newVersionCommand())
	return rootCmd
}

func newServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the AI proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := app.Build(context.Background(), app.BuildOptions{
				ConfigPath: configPath,
				Version:    version,
			})
			if err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			return a.Run(ctx)
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath(), "path to config file")
	return cmd
}

func newValidateCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the config file without running the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := app.Build(context.Background(), app.BuildOptions{
				ConfigPath: cfgPath,
				Version:    version,
			})
			if err != nil {
				return err
			}
			fmt.Println("config is valid")
			return nil
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file")
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

func newPathsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "Print resolved default config and secrets paths",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "config: %s\nsecrets: %s\n", defaultConfigPath(), defaultSecretsPath())
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
aiproxy validate
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy paths
aiproxy configure
aiproxy configure provider
aiproxy configure provider --config /etc/aiproxy/config.hcl --non-interactive --name backup --type openai-compatible --base-url https://llm.internal/v1 --secrets-key localai --api-key "$LOCALAI_API_KEY" --model qwen3-32b
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
