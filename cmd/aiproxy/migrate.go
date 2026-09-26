package main

import (
	"context"
	"fmt"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/spf13/cobra"
)

func newMigrateCommand() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage database migrations (requires multi_tenancy + database)",
	}
	cmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file (overrides $AIPROXY_CONFIG)")
	cmd.AddCommand(newMigrateUpCommand(&cfgPath), newMigrateStatusCommand(&cfgPath))
	return cmd
}

func migrateStore(ctx context.Context, cfgPath string, cmd *cobra.Command) (*store.Store, error) {
	rt, err := config.LoadFileOrEnv(cfgPath, cmd.Flags().Changed("config"))
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if !rt.MultiTenancyEnabled() {
		return nil, fmt.Errorf("multi_tenancy not enabled: enable multi_tenancy + database first")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return store.Open(ctx, rt.Database.URL)
}

func newMigrateUpCommand(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply pending migrations and seed the admin user",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := migrateStore(ctx, *cfgPath, cmd)
			if err != nil {
				return err
			}
			defer st.Close()
			applied, err := st.MigrateUp(ctx)
			if err != nil {
				return err
			}
			if len(applied) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "migrations up to date")
			} else {
				for _, name := range applied {
					fmt.Fprintln(cmd.OutOrStdout(), "applied "+name)
				}
			}
			seeded, err := store.EnsureAdmin(ctx, st)
			if err != nil {
				return err
			}
			if seeded {
				fmt.Fprintln(cmd.OutOrStdout(), "seeded admin user from AIPROXY_ADMIN_EMAIL")
			}
			if err := st.EnsureSystemOrg(ctx); err != nil {
				return err
			}
			return nil
		},
	}
}

func newMigrateStatusCommand(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show applied and pending migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			st, err := migrateStore(ctx, *cfgPath, cmd)
			if err != nil {
				return err
			}
			defer st.Close()
			status, err := st.Status(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "applied: %d pending: %d\n", len(status.Applied), len(status.Pending))
			for _, name := range status.Pending {
				fmt.Fprintln(cmd.OutOrStdout(), "pending "+name)
			}
			return nil
		},
	}
}
