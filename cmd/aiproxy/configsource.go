package main

import (
	"github.com/egose/aiproxy/internal/config"
	"github.com/spf13/cobra"
)

func configFlagExplicit(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("config")
}

func configFromEnvRequested(cmd *cobra.Command) bool {
	if configFlagExplicit(cmd) {
		return false
	}
	_, ok := config.EnvConfigContent()
	return ok
}

func loadConfigForCommand(cfgPath string, cmd *cobra.Command) (*config.Runtime, error) {
	return config.LoadFileOrEnv(cfgPath, configFlagExplicit(cmd))
}
