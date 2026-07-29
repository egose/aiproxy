package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
		Long:  "aiproxy proxies multiple AI providers behind a single OpenAI-compatible API.\n\nDefault config path: $XDG_CONFIG_HOME/aiproxy/config.hcl\nFallback config path: ~/.config/aiproxy/config.hcl\nDefault secrets path: $XDG_CONFIG_HOME/aiproxy/keys.json\nFallback secrets path: ~/.config/aiproxy/keys.json",
	}
	rootCmd.AddCommand(newServeCommand())
	rootCmd.AddCommand(newValidateCommand())
	rootCmd.AddCommand(newPathsCommand())
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
	return configExamplesText() + "\n" + dockerExamplesText() + "\n" + systemdExamplesText()
}

func configExamplesText() string {
	return commandExamplesText() + "\n" + minimalConfigExamplesText() + "\n" + aliasExamplesText() + "\n" + secretsExamplesText() + "\n" + apiKeyRefExamplesText()
}

func commandExamplesText() string {
	return `Common commands:
aiproxy serve
aiproxy validate
aiproxy serve --config /etc/aiproxy/config.hcl
aiproxy validate --config /etc/aiproxy/config.hcl
aiproxy paths
aiproxy version
`
}

func minimalConfigExamplesText() string {
	return `

Minimal config:
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  api_key = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {}
}
`
}

func authExamplesText() string {
	return `Auth example:
auth "main" {
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
}
`
}

func aliasExamplesText() string {
	return authExamplesText() + `

Alias failover example:
listener "http" "public" {
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
}
`
}

func secretsExamplesText() string {
	return `Secrets file example:
{
  "openai": "sk-...",
  "localai": "secret"
}
`
}

func apiKeyRefExamplesText() string {
	return `api_key_ref override example:
provider "openai-compatible" "localai" {
  base_url = "https://llm.internal/v1"

  api_key_ref {
    path = "/etc/aiproxy/keys.json"
    key  = "localai"
  }

  model "qwen3-32b" {}
}
`
}

func dockerExamplesText() string {
	return `Docker example:
docker run --rm \
  -p 8080:8080 \
  -v /etc/aiproxy/config.hcl:/etc/aiproxy/config.hcl:ro \
  -v /etc/aiproxy/keys.json:/etc/aiproxy/keys.json:ro \
  aiproxy:latest serve --config /etc/aiproxy/config.hcl
`
}

func systemdExamplesText() string {
	return `systemd example:
[Unit]
Description=aiproxy
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/aiproxy serve --config /etc/aiproxy/config.hcl
Restart=on-failure
Environment=OPENAI_API_KEY=

[Install]
WantedBy=multi-user.target
`
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
