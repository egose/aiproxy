package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/egose/aiproxy/internal/app"
	"github.com/spf13/cobra"
)

var (
	configPath string
	version    = "dev"
)

func main() {
	rootCmd := &cobra.Command{Use: "aiproxy"}

	rootCmd.AddCommand(newServeCommand())
	rootCmd.AddCommand(newValidateCommand())
	rootCmd.AddCommand(newVersionCommand())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
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
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to config file (required)")
	cmd.MarkFlagRequired("config")
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
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "", "path to config file (required)")
	cmd.MarkFlagRequired("config")
	return cmd
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
