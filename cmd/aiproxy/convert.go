package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/spf13/cobra"
)

func newConvertCommand() *cobra.Command {
	var cfgPath string
	var compact bool
	var force bool
	cmd := &cobra.Command{
		Use:   "convert [target-file]",
		Short: "Convert the config between HCL and JSON",
		Long: `Convert the active config to the other syntax (HCL to JSON or JSON to HCL).

The source defaults to the usual config context: an explicit --config file,
otherwise $AIPROXY_CONFIG, otherwise the default config path. The target
defaults to a file in the current working directory with the converted
extension. Use "-" as the target to print to stdout.

env("VAR") calls are resolved at conversion time, so the referenced
variables must be set. The converted output is validated before writing.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConvert(cmd, cfgPath, compact, force, args)
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to source config file (overrides $AIPROXY_CONFIG)")
	cmd.Flags().BoolVar(&compact, "compact", false, "write single-line JSON output (applies to JSON output only)")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite the target file if it exists")
	return cmd
}

func runConvert(cmd *cobra.Command, cfgPath string, compact, force bool, args []string) error {
	src, srcDesc, srcBase, err := readConvertSource(cmd, cfgPath)
	if err != nil {
		return err
	}
	out, from, to, err := config.Convert(src, srcDesc, compact)
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "-" {
		fmt.Fprint(cmd.OutOrStdout(), string(out))
		return nil
	}
	target := ""
	if len(args) > 0 {
		target = args[0]
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target = filepath.Join(cwd, defaultConvertedName(srcBase, to))
	}
	if _, err := os.Stat(target); err == nil && !force {
		return fmt.Errorf("target %s already exists (use --force to overwrite)", target)
	}
	if err := os.WriteFile(target, out, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "converted %s (%s) -> %s (%s)\n", srcDesc, from, target, to)
	return nil
}

func readConvertSource(cmd *cobra.Command, cfgPath string) ([]byte, string, string, error) {
	if !configFlagExplicit(cmd) {
		if src, ok := config.EnvConfigContent(); ok {
			return []byte(src), config.EnvConfigFilename, "", nil
		}
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, "", "", err
	}
	return raw, cfgPath, filepath.Base(cfgPath), nil
}

func defaultConvertedName(srcBase, to string) string {
	if srcBase == "" {
		return "config." + to
	}
	ext := strings.ToLower(filepath.Ext(srcBase))
	if ext == ".hcl" || ext == ".json" {
		return strings.TrimSuffix(srcBase, filepath.Ext(srcBase)) + "." + to
	}
	return srcBase + "." + to
}
